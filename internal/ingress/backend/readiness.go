package backend

import (
	"fmt"

	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
)

// a special readiness condition type that will mark pod as ready if any targetGroup have the pod as healthy.
// This is added to temporarily support Ingress/Backend with long names
// Please don't use this if your pod will be registered in multiple targetGroups.
// See https://github.com/kubernetes-sigs/aws-alb-ingress-controller/pull/1253
const AnyLBTGReadyConditionType = api.PodConditionType("target-health.alb.ingress.k8s.aws/load-balancer-any-tg-ready")

// PodReadinessGateConditionType returns the PodConditionType that is associated with the given ingress and backend
func PodReadinessGateConditionType(ingress *extensions.Ingress, backend *extensions.IngressBackend) api.PodConditionType {
	return api.PodConditionType(fmt.Sprintf(
		"target-health.alb.ingress.k8s.aws/%s_%s_%s",
		ingress.Name,
		backend.ServiceName,
		backend.ServicePort.String(),
	))
}

// PodHasReadinessGate returns true if the given pod has a readinessGate with the given conditionType
func PodHasReadinessGate(pod *api.Pod, conditionType api.PodConditionType) bool {
	for _, rg := range pod.Spec.ReadinessGates {
		if rg.ConditionType == conditionType {
			return true
		}
	}
	return false
}

func PodConditionForReadinessGate(pod *api.Pod, conditionType api.PodConditionType) (int, *api.PodCondition) {
	for i, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			return i, &condition
		}
	}
	return -1, nil
}
