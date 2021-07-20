package k8s

import (
	"encoding/json"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	annotationKeyPodENIInfo = "vpc.amazonaws.com/pod-eni"
)

// PodInfo contains simplified pod information we cares about.
// We do so to minimize memory usage.
type PodInfo struct {
	Key types.NamespacedName
	UID types.UID

	ContainerPorts []corev1.ContainerPort
	ReadinessGates []corev1.PodReadinessGate
	Conditions     []corev1.PodCondition
	NodeName       string
	PodIP          string

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

// buildPodInfo will construct PodInfo for given pod.
func buildPodInfo(pod *corev1.Pod) PodInfo {
	podKey := NamespacedName(pod)

	var podENIInfos []PodENIInfo
	// we kept podENIInfo as nil if the eniInfo via annotation is malformed.
	if eniInfo, err := buildPodENIInfos(pod); err == nil {
		podENIInfos = eniInfo
	}

	var containerPorts []corev1.ContainerPort
	for _, podContainer := range pod.Spec.Containers {
		containerPorts = append(containerPorts, podContainer.Ports...)
	}
	return PodInfo{
		Key: podKey,
		UID: pod.UID,

		ContainerPorts: containerPorts,
		ReadinessGates: pod.Spec.ReadinessGates,
		Conditions:     pod.Status.Conditions,
		NodeName:       pod.Spec.NodeName,
		PodIP:          pod.Status.PodIP,

		ENIInfos: podENIInfos,
	}
}

// buildPodENIInfo will construct PodENIInfo for given pod if any.
func buildPodENIInfos(pod *corev1.Pod) ([]PodENIInfo, error) {
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
