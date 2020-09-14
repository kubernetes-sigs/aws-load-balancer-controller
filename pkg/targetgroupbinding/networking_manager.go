package targetgroupbinding

import (
	"context"
	elbv2api "sigs.k8s.io/aws-alb-ingress-controller/apis/elbv2/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
)

// NetworkingManager manages the networking for targetGroupBindings.
type NetworkingManager interface {
	ReconcileForPodEndpoints(ctx context.Context, tgb *elbv2api.TargetGroupBinding, endpoints []backend.PodEndpoint) error
	ReconcileForNodePortEndpoints(ctx context.Context, tgb *elbv2api.TargetGroupBinding, endpoints []backend.NodePortEndpoint) error
}

// default implementation for NetworkingManager.
type defaultNetworkingManager struct {
}

func (m *defaultNetworkingManager) ReconcileForPodEndpoints(ctx context.Context, tgb *elbv2api.TargetGroupBinding, endpoints []backend.PodEndpoint) error {
	return nil
}

func (m *defaultResourceManager) ReconcileForNodePortEndpoints(ctx context.Context, tgb *elbv2api.TargetGroupBinding, endpoints []backend.NodePortEndpoint) error {
	return nil
}

func (m *defaultResourceManager) reconcileForInstanceSGs(ctx context.Context, tgb *elbv2api.TargetGroupBinding, instanceSGs []string) error {
	return nil
}

func (m *defaultNetworkingManager) computeInstanceSGsForPodEndpoints(ctx context.Context, endpoints []backend.PodEndpoint) []string {
	return nil
}

func (m *defaultResourceManager) computeSInstanceSGsForNodePortEndpoints(ctx context.Context, endpoints []backend.NodePortEndpoint) []string {
	return nil
}
