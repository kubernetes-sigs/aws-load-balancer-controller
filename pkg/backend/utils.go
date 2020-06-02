package backend

import corev1 "k8s.io/api/core/v1"

const (
	labelNodeRoleMaster               = "node-role.kubernetes.io/master"
	labelNodeRoleExcludeBalancer      = "node.kubernetes.io/exclude-from-external-load-balancers"
	labelAlphaNodeRoleExcludeBalancer = "alpha.service-controller.kubernetes.io/exclude-balancer"
	labelEKSComputeType               = "eks.amazonaws.com/compute-type"
)

// IsNodeSuitableAsTrafficProxy check whether node is suitable as a traffic proxy.
// mimic the logic of serviceController: https://github.com/kubernetes/kubernetes/blob/b6b494b4484b51df8dc6b692fab234573da30ab4/pkg/controller/service/controller.go#L605
func IsNodeSuitableAsTrafficProxy(node *corev1.Node) bool {
	if node.Spec.Unschedulable {
		return false
	}
	if s, ok := node.ObjectMeta.Labels[labelEKSComputeType]; ok && s == "fargate" {
		return false
	}
	for _, label := range []string{labelNodeRoleMaster, labelNodeRoleExcludeBalancer, labelAlphaNodeRoleExcludeBalancer} {
		if _, hasLabel := node.ObjectMeta.Labels[label]; hasLabel {
			return false
		}
	}
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
