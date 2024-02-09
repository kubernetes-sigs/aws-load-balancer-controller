package ingress

import (
	"context"
	"fmt"
	networking "k8s.io/api/networking/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
)

const (
	explicitGroupFinalizerPrefix = "group.ingress.k8s.aws/"
	implicitGroupFinalizer       = "ingress.k8s.aws/resources"
)

// FinalizerManager manages finalizer for ingresses.
type FinalizerManager interface {
	// AddGroupFinalizer add Ingress group finalizer for active member Ingresses.
	// Ingresses will be in-place updated.
	AddGroupFinalizer(ctx context.Context, groupID GroupID, members []ClassifiedIngress) error

	// RemoveGroupFinalizer remove Ingress group finalizer from inactive member Ingresses.
	// Ingresses will be in-place updated.
	RemoveGroupFinalizer(ctx context.Context, groupID GroupID, inactiveMembers []*networking.Ingress) error
}

// NewDefaultFinalizerManager constructs new defaultFinalizerManager
func NewDefaultFinalizerManager(k8sFinalizerManager k8s.FinalizerManager) *defaultFinalizerManager {
	return &defaultFinalizerManager{
		k8sFinalizerManager: k8sFinalizerManager,
	}
}

var _ FinalizerManager = (*defaultFinalizerManager)(nil)

// default implementation of FinalizerManager
type defaultFinalizerManager struct {
	k8sFinalizerManager k8s.FinalizerManager
}

func (m *defaultFinalizerManager) AddGroupFinalizer(ctx context.Context, groupID GroupID, members []ClassifiedIngress) error {
	finalizer := buildGroupFinalizer(groupID)
	for _, member := range members {
		if err := m.k8sFinalizerManager.AddFinalizers(ctx, member.Ing, finalizer); err != nil {
			return err
		}
	}
	return nil
}

func (m *defaultFinalizerManager) RemoveGroupFinalizer(ctx context.Context, groupID GroupID, inactiveMembers []*networking.Ingress) error {
	finalizer := buildGroupFinalizer(groupID)
	for _, ing := range inactiveMembers {
		if err := m.k8sFinalizerManager.RemoveFinalizers(ctx, ing, finalizer); err != nil {
			return err
		}
	}
	return nil
}

// buildGroupFinalizer returns a finalizer for specified Ingress group
// for explicit group, the format is "group.ingress.k8s.aws/awesome-group"
// for implicit group, the format is "ingress.k8s.aws/resources"
func buildGroupFinalizer(groupID GroupID) string {
	if groupID.IsExplicit() {
		return fmt.Sprintf("%s%s", explicitGroupFinalizerPrefix, groupID.Name)
	}
	return implicitGroupFinalizer
}
