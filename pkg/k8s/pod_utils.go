package k8s

import corev1 "k8s.io/api/core/v1"

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
