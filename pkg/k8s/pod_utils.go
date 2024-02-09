package k8s

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func IsPodHasReadinessGate(pod *corev1.Pod, conditionType corev1.PodConditionType) bool {
	for _, rg := range pod.Spec.ReadinessGates {
		if rg.ConditionType == conditionType {
			return true
		}
	}
	return false
}

// IsPodContainersReady returns whether pod is containersReady.
func IsPodContainersReady(pod *corev1.Pod) bool {
	containersReadyCond := GetPodCondition(pod, corev1.ContainersReady)
	return containersReadyCond != nil && containersReadyCond.Status == corev1.ConditionTrue
}

// GetPodCondition will get pointer to Pod's existing condition.
// returns nil if no matching condition found.
func GetPodCondition(pod *corev1.Pod, conditionType corev1.PodConditionType) *corev1.PodCondition {
	for i := range pod.Status.Conditions {
		if pod.Status.Conditions[i].Type == conditionType {
			return &pod.Status.Conditions[i]
		}
	}
	return nil
}

// UpdatePodCondition will update Pod to contain specified condition.
func UpdatePodCondition(pod *corev1.Pod, condition corev1.PodCondition) {
	existingCond := GetPodCondition(pod, condition.Type)
	if existingCond != nil {
		*existingCond = condition
		return
	}
	pod.Status.Conditions = append(pod.Status.Conditions, condition)
}

// LookupContainerPort returns the numerical containerPort for specific port on Pod.
func LookupContainerPort(pod *corev1.Pod, port intstr.IntOrString) (int64, error) {
	switch port.Type {
	case intstr.String:
		for _, podContainer := range pod.Spec.Containers {
			for _, podPort := range podContainer.Ports {
				if podPort.Name == port.StrVal {
					return int64(podPort.ContainerPort), nil
				}
			}
		}
	case intstr.Int:
		return int64(port.IntVal), nil
	}
	return 0, errors.Errorf("unable to find port %s on pod %s", port.String(), NamespacedName(pod))
}
