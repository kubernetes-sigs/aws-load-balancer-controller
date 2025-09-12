package k8s

import (
	"encoding/json"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	annotationKeyPodENIInfo = "vpc.amazonaws.com/pod-eni"
)

// PodInfo contains simplified pod information we care about.
// We do so to minimize memory usage.
type PodInfo struct {
	Key types.NamespacedName
	UID types.UID

	ContainerPorts []corev1.ContainerPort
	QUICServerIDs  map[int32]string
	ReadinessGates []corev1.PodReadinessGate
	Conditions     []corev1.PodCondition
	NodeName       string
	PodIP          string
	CreationTime   v1.Time

	ENIInfos []PodENIInfo
}

// PodENIInfo is a json convertible structure that stores the Branch ENI details that can be
// used by the CNI plugin or the component consuming the resource
type PodENIInfo struct {
	// ENIID is the network interface id of the branch interface
	ENIID string `json:"eniId"`

	// PrivateIP is the primary IP of the branch Network interface
	PrivateIP string `json:"privateIp"`
}

// HasAnyOfReadinessGates returns whether podInfo has any of these readinessGates
func (i *PodInfo) HasAnyOfReadinessGates(conditionTypes []corev1.PodConditionType) bool {
	for _, rg := range i.ReadinessGates {
		for _, conditionType := range conditionTypes {
			if rg.ConditionType == conditionType {
				return true
			}
		}
	}
	return false
}

// IsContainersReady returns whether podInfo is ContainersReady.
func (i *PodInfo) IsContainersReady() bool {
	containersReadyCond, exists := i.GetPodCondition(corev1.ContainersReady)
	return exists && containersReadyCond.Status == corev1.ConditionTrue
}

// GetPodCondition will get Pod's condition.
func (i *PodInfo) GetPodCondition(conditionType corev1.PodConditionType) (corev1.PodCondition, bool) {
	for _, cond := range i.Conditions {
		if cond.Type == conditionType {
			return cond, true
		}
	}

	return corev1.PodCondition{}, false
}

// LookupContainerPort returns the numerical containerPort for specific port on Pod.
func (i *PodInfo) LookupContainerPort(port intstr.IntOrString) (int64, error) {
	switch port.Type {
	case intstr.String:
		for _, podPort := range i.ContainerPorts {
			if podPort.Name == port.StrVal {
				return int64(podPort.ContainerPort), nil
			}
		}
	case intstr.Int:
		return int64(port.IntVal), nil
	}
	return 0, errors.Errorf("unable to find port %s on pod %s", port.String(), i.Key)
}

type podInfoBuilder struct {
	quicServerIDVariableName string
}

func newPodInfoBuilder(quicServerIDVariableName string) *podInfoBuilder {
	return &podInfoBuilder{
		quicServerIDVariableName: quicServerIDVariableName,
	}
}

// podInfoConversionFunc computes the converted PodInfo per pod object.
func (podInfoBuilder *podInfoBuilder) podInfoConverter(obj interface{}) (interface{}, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, errors.New("expect pod object")
	}
	podInfo := podInfoBuilder.buildPodInfo(pod)
	return &podInfo, nil
}

// buildPodInfo will construct PodInfo for given pod.
func (podInfoBuilder *podInfoBuilder) buildPodInfo(pod *corev1.Pod) PodInfo {
	podKey := NamespacedName(pod)

	var podENIInfos []PodENIInfo
	// we kept podENIInfo as nil if the eniInfo via annotation is malformed.
	if eniInfo, err := podInfoBuilder.buildPodENIInfos(pod); err == nil {
		podENIInfos = eniInfo
	}

	/*
		var containerInfo []ContainerInformation
		for _, podContainer := range pod.Spec.Containers {
			for i := range podContainer.Ports {
				containerInfo = append(containerInfo, ContainerInformation{
					Port:         podContainer.Ports[i],
					QUICServerID: podInfoBuilder.extractQUICServerID(pod, podContainer),
				})
			}
		}
		// also support sidecar container (initContainer with restartPolicy=Always)
		for _, podContainer := range pod.Spec.InitContainers {
			if podContainer.RestartPolicy != nil && *podContainer.RestartPolicy == corev1.ContainerRestartPolicyAlways {
				for i := range podContainer.Ports {
					containerInfo = append(containerInfo, ContainerInformation{
						Port:         podContainer.Ports[i],
						QUICServerID: podInfoBuilder.extractQUICServerID(pod, podContainer),
					})
				}
			}
		}
	*/

	var containerPorts []corev1.ContainerPort
	var quicServerIDs map[int32]string
	for _, podContainer := range pod.Spec.Containers {
		containerPorts = append(containerPorts, podContainer.Ports...)
		extractedId := podInfoBuilder.extractQUICServerID(pod, podContainer)
		if extractedId != nil {
			if quicServerIDs == nil {
				quicServerIDs = make(map[int32]string)
			}
			for _, p := range podContainer.Ports {
				quicServerIDs[p.ContainerPort] = *extractedId
			}
		}
	}
	// also support sidecar container (initContainer with restartPolicy=Always)
	for _, podContainer := range pod.Spec.InitContainers {
		if podContainer.RestartPolicy != nil && *podContainer.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			containerPorts = append(containerPorts, podContainer.Ports...)
		}

		extractedId := podInfoBuilder.extractQUICServerID(pod, podContainer)
		if extractedId != nil {
			if quicServerIDs == nil {
				quicServerIDs = make(map[int32]string)
			}
			for _, p := range podContainer.Ports {
				quicServerIDs[p.ContainerPort] = *extractedId
			}
		}
	}

	return PodInfo{
		Key: podKey,
		UID: pod.UID,

		ContainerPorts: containerPorts,
		QUICServerIDs:  quicServerIDs,
		ReadinessGates: pod.Spec.ReadinessGates,
		Conditions:     pod.Status.Conditions,
		NodeName:       pod.Spec.NodeName,
		PodIP:          pod.Status.PodIP,
		CreationTime:   pod.CreationTimestamp,

		ENIInfos: podENIInfos,
	}
}

// buildPodENIInfo will construct PodENIInfo for given pod if any.
func (podInfoBuilder *podInfoBuilder) buildPodENIInfos(pod *corev1.Pod) ([]PodENIInfo, error) {
	rawAnnotation, ok := pod.Annotations[annotationKeyPodENIInfo]
	if !ok {
		return nil, nil
	}

	var podENIInfos []PodENIInfo
	if err := json.Unmarshal([]byte(rawAnnotation), &podENIInfos); err != nil {
		return nil, err
	}

	return podENIInfos, nil
}

func (podInfoBuilder *podInfoBuilder) extractQUICServerID(pod *corev1.Pod, container corev1.Container) *string {

	if pod.Annotations == nil {
		return nil
	}

	// TODO - Fix this
	_, ok := pod.Annotations["service.beta.kubernetes.io/aws-load-balancer-quic-enabled-containers"]

	if !ok {
		return nil
	}

	if container.Env == nil || len(container.Env) == 0 {
		return nil
	}

	for _, env := range container.Env {
		if env.Name == podInfoBuilder.quicServerIDVariableName {
			return &env.Value
		}
	}

	return nil
}
