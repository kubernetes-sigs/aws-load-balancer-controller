package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NamespacedName returns the namespaced name for k8s objects
func NamespacedName(obj metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

// ToSliceOfNamespacedNames gets the slice of types.NamespacedName from the input slice s
func ToSliceOfNamespacedNames[T metav1.ObjectMetaAccessor](s []T) []types.NamespacedName {
	result := make([]types.NamespacedName, len(s))
	for i, v := range s {
		result[i] = NamespacedName(v.GetObjectMeta())
	}
	return result
}
