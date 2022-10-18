package k8s

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ToSliceOfMetaObject converts the input slice s to slice of metav1.Object
func ToSliceOfMetaObject[T metav1.Object](s []T) []metav1.Object {
	result := make([]metav1.Object, len(s))
	for i, v := range s {
		result[i] = v
	}
	return result
}
