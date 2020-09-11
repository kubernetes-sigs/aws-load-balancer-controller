package k8s

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"strings"
)

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

func ExtractNodeInstanceID(node *corev1.Node) (string, error) {
	providerID := node.Spec.ProviderID
	if providerID == "" {
		return "", errors.Errorf("providerID is not specified for node: %s", node.Name)
	}

	p := strings.Split(providerID, "/")
	return p[len(p)-1], nil
}
