package k8s

import (
	"context"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type FinalizerManager interface {
	AddFinalizers(ctx context.Context, object client.Object, finalizers ...string) error
	RemoveFinalizers(ctx context.Context, object client.Object, finalizers ...string) error
}

func NewDefaultFinalizerManager(k8sClient client.Client, log logr.Logger) FinalizerManager {
	return &defaultFinalizerManager{
		k8sClient: k8sClient,
		log:       log,
	}
}

type defaultFinalizerManager struct {
	k8sClient client.Client

	log logr.Logger
}

func (m *defaultFinalizerManager) AddFinalizers(ctx context.Context, obj client.Object, finalizers ...string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := m.k8sClient.Get(ctx, NamespacedName(obj), obj); err != nil {
			return err
		}

		oldObj := obj.DeepCopyObject().(client.Object)
		needsUpdate := false
		for _, finalizer := range finalizers {
			if !HasFinalizer(obj, finalizer) {
				controllerutil.AddFinalizer(obj, finalizer)
				needsUpdate = true
			}
		}
		if !needsUpdate {
			return nil
		}
		return m.k8sClient.Patch(ctx, obj, client.MergeFromWithOptions(oldObj, client.MergeFromWithOptimisticLock{}))
	})
}

func (m *defaultFinalizerManager) RemoveFinalizers(ctx context.Context, obj client.Object, finalizers ...string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := m.k8sClient.Get(ctx, NamespacedName(obj), obj); err != nil {
			return err
		}

		oldObj := obj.DeepCopyObject().(client.Object)
		needsUpdate := false
		for _, finalizer := range finalizers {
			if HasFinalizer(obj, finalizer) {
				controllerutil.RemoveFinalizer(obj, finalizer)
				needsUpdate = true
			}
		}
		if !needsUpdate {
			return nil
		}
		return m.k8sClient.Patch(ctx, obj, client.MergeFromWithOptions(oldObj, client.MergeFromWithOptimisticLock{}))
	})
}

// HasFinalizer tests whether k8s object has specified finalizer
func HasFinalizer(obj metav1.Object, finalizer string) bool {
	f := obj.GetFinalizers()
	for _, e := range f {
		if e == finalizer {
			return true
		}
	}
	return false
}
