package ingress

import (
	"context"
	"fmt"
	networking "k8s.io/api/networking/v1beta1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	groupFinalizerPrefix = "alb.ingress.k8s.aws"
)

// FinalizerManager manages finalizer for ingresses.
type FinalizerManager interface {
	// AddGroupFinalizer add Ingress group finalizer for specified Ingresses.
	// Ingresses will be in-place updated.
	AddGroupFinalizer(ctx context.Context, groupID GroupID, ingList ...*networking.Ingress) error

	// RemoveGroupFinalizer remove Ingress group finalizer for specified Ingresses.
	// Ingresses will be in-place updated.
	RemoveGroupFinalizer(ctx context.Context, groupID GroupID, ingList ...*networking.Ingress) error
}

// NewDefaultFinalizerManager constructs new defaultFinalizerManager
func NewDefaultFinalizerManager(client client.Client) *defaultFinalizerManager {
	return &defaultFinalizerManager{client: client}
}

var _ FinalizerManager = (*defaultFinalizerManager)(nil)

// default implementation of FinalizerManager
type defaultFinalizerManager struct {
	client client.Client
}

func (m *defaultFinalizerManager) AddGroupFinalizer(ctx context.Context, groupID GroupID, ingList ...*networking.Ingress) error {
	finalizer := buildGroupFinalizer(groupID)
	for _, ing := range ingList {
		if k8s.HasFinalizer(ing, finalizer) {
			continue
		}

		oldIng := ing.DeepCopy()
		controllerutil.AddFinalizer(ing, finalizer)
		if err := m.client.Patch(ctx, ing, client.MergeFrom(oldIng)); err != nil {
			return err
		}
	}
	return nil
}

func (m *defaultFinalizerManager) RemoveGroupFinalizer(ctx context.Context, groupID GroupID, ingList ...*networking.Ingress) error {
	finalizer := buildGroupFinalizer(groupID)
	for _, ing := range ingList {
		if !k8s.HasFinalizer(ing, finalizer) {
			continue
		}

		oldIng := ing.DeepCopy()
		controllerutil.RemoveFinalizer(ing, finalizer)
		if err := m.client.Patch(ctx, ing, client.MergeFrom(oldIng)); err != nil {
			return err
		}
	}
	return nil
}

// buildGroupFinalizer returns a finalizer for specified Ingress group
// for explicit group, the format is "alb.ingress.k8s.aws/awesome-group"
// for implicit group, the format is "alb.ingress.k8s.aws/namespace-name.ingress-name"
func buildGroupFinalizer(groupID GroupID) string {
	if groupID.IsExplicit() {
		return fmt.Sprintf("%s/%s", groupFinalizerPrefix, groupID.Name)
	}
	return fmt.Sprintf("%s/%s.%s", groupFinalizerPrefix, groupID.Namespace, groupID.Name)
}
