package k8s

import corev1 "k8s.io/api/core/v1"

// IsNodeReady returns whether node is ready.
func IsNodeReady(node *corev1.Node) bool {
	nodeReadyCond := GetNodeCondition(node, corev1.NodeReady)
	return nodeReadyCond != nil && nodeReadyCond.Status == corev1.ConditionTrue
}

// GetNodeCondition will get pointer to Node's existing condition.
// returns nil if no matching condition found.
func GetNodeCondition(node *corev1.Node, conditionType corev1.NodeConditionType) *corev1.NodeCondition {
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == conditionType {
			return &node.Status.Conditions[i]
		}
	}
	return nil
}
