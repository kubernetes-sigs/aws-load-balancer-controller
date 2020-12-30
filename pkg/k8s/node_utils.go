package k8s

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"strings"
)

const (
	toBeDeletedByCATaint = "ToBeDeletedByClusterAutoscaler"
)

// IsNodeReady returns whether node is ready.
func IsNodeReady(node *corev1.Node) bool {
	nodeReadyCond := GetNodeCondition(node, corev1.NodeReady)
	return nodeReadyCond != nil && nodeReadyCond.Status == corev1.ConditionTrue
}

// IsNodeSuitableAsTrafficProxy check whether node is suitable as a traffic proxy.
// mimic the logic of serviceController: https://github.com/kubernetes/kubernetes/blob/b6b494b4484b51df8dc6b692fab234573da30ab4/pkg/controller/service/controller.go#L605
func IsNodeSuitableAsTrafficProxy(node *corev1.Node) bool {
	// ToBeDeletedByClusterAutoscaler taint is added by cluster autoscaler before removing node from cluster
	// Marking the node as unsuitable for traffic once the taint is observed on the node
	for _, taint := range node.Spec.Taints {
		if taint.Key == toBeDeletedByCATaint {
			return false
		}
	}

	return IsNodeReady(node)
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
