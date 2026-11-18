package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsNamespaceTerminating checks whether the namespace of obj is being deleted.
// It uses the condition of NamespaceLifecycle admission, which rejects new objects in a namespace
// whose phase is Terminating. A namespace that is not found is treated as not terminating.
func IsNamespaceTerminating(ctx context.Context, k8sClient client.Client, obj client.Object) (bool, error) {
	ns := &corev1.Namespace{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: obj.GetNamespace()}, ns); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	if ns.Status.Phase != corev1.NamespaceTerminating {
		return false, nil
	}
	// The Namespace cache can lag behind the cache of obj. An object created after the cached
	// namespace started terminating belongs to a newer namespace with the same name. An object created
	// in the same second belongs to the terminating namespace: a deleted namespace is removed no sooner
	// than 5 s after its deletionTimestamp, so a namespace with the same name is always newer.
	createdAt := obj.GetCreationTimestamp()
	if ns.DeletionTimestamp != nil && createdAt.After(ns.DeletionTimestamp.Time) {
		return false, nil
	}
	return true, nil
}
