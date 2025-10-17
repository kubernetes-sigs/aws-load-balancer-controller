package k8s

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"regexp"
	"strings"
)

var awsInstanceIDRegex = regexp.MustCompile("^i-[^/]*$")
var eksHybridIDRegex = regexp.MustCompile("^mi-[^/]*$")

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

	// Check if this is a hybrid node
	if strings.HasPrefix(providerID, "eks-hybrid://") {
		providerIDParts := strings.Split(providerID, "/")
		hybridID := providerIDParts[len(providerIDParts)-1]
		if !eksHybridIDRegex.MatchString(hybridID) {
			return "", errors.Errorf("providerID %s is invalid for EKS hybrid instances, node: %s", providerID, node.Name)
		}
		// Return a special prefix to identify hybrid nodes
		return "hybrid-" + hybridID, nil
	}

	// Handle EC2 instances (existing logic)
	providerIDParts := strings.Split(providerID, "/")
	instanceID := providerIDParts[len(providerIDParts)-1]
	if !awsInstanceIDRegex.MatchString(instanceID) {
		return "", errors.Errorf("providerID %s is invalid for EC2 instances, node: %s", providerID, node.Name)
	}
	return instanceID, nil
}
