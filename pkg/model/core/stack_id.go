package core

import (
	"fmt"
	"k8s.io/apimachinery/pkg/types"
)

// stackID is the identifier of a stack, it must be compatible with Kubernetes namespaced name.
type StackID types.NamespacedName

// String returns the string representation of a StackID.
// It will be used as AWS resource tags for resources provisioned for this stack.
func (stackID StackID) String() string {
	if stackID.Namespace == "" {
		return stackID.Name
	}
	return fmt.Sprintf("%s/%s", stackID.Namespace, stackID.Name)
}
