package k8s

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func NamespacedName(obj metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

func LocalObjectReference(obj metav1.Object) corev1.LocalObjectReference {
	return corev1.LocalObjectReference{
		Name: obj.GetName(),
	}
}

func ObjectReference(obj metav1.Object) corev1.ObjectReference {
	return corev1.ObjectReference{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}
