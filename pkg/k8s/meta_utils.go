package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ToSliceOfMetaObject converts the input slice s to slice of metav1.Object
func ToSliceOfMetaObject[T metav1.ObjectMetaAccessor](s []T) []metav1.Object {
	result := make([]metav1.Object, len(s))
	for i, v := range s {
		result[i] = v.GetObjectMeta()
	}
	return result
}

func ToSliceOfNamespacedNames[T metav1.ObjectMetaAccessor](s []T) []types.NamespacedName {
	result := make([]types.NamespacedName, len(s))
	for i, v := range s {
		result[i] = NamespacedName(v.GetObjectMeta())
	}
	return result
}
