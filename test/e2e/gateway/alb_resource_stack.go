package gateway

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func newALBResourceStack(dps []*appsv1.Deployment, svcs []*corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, tgcs []*elbv2gw.TargetGroupConfiguration, lrc *elbv2gw.ListenerRuleConfiguration, httpr []*gwv1.HTTPRoute, grpcrs []*gwv1.GRPCRoute, baseName string, enablePodReadinessGate bool) *albResourceStack {

	commonStack := newCommonResourceStack(dps, svcs, gwc, gw, lbc, tgcs, []*elbv2gw.ListenerRuleConfiguration{lrc}, baseName, enablePodReadinessGate)
	return &albResourceStack{
		httprs:      httpr,
		grpcrs:      grpcrs,
		commonStack: commonStack,
	}
}

// resourceStack containing the deployment and service resources
type albResourceStack struct {
	commonStack *commonResourceStack
	httprs      []*gwv1.HTTPRoute
	grpcrs      []*gwv1.GRPCRoute
}

func (s *albResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {
	return s.commonStack.Deploy(ctx, f, func(ctx context.Context, f *framework.Framework, namespace string) error {
		for i := range s.httprs {
			s.httprs[i].Namespace = namespace
			if err := s.createHTTPRoute(ctx, f, s.httprs[i]); err != nil {
				return err
			}
		}

		for i := range s.grpcrs {
			s.grpcrs[i].Namespace = namespace
			if err := s.createGRPCRoute(ctx, f, s.grpcrs[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *albResourceStack) Cleanup(ctx context.Context, f *framework.Framework) {
	s.commonStack.Cleanup(ctx, f)
}

func (s *albResourceStack) GetLoadBalancerIngressHostname() string {
	return s.commonStack.GetLoadBalancerIngressHostname()
}

func (s *albResourceStack) getListenersPortMap() map[string]string {
	return s.commonStack.getListenersPortMap()
}

func (s *albResourceStack) waitUntilDeploymentReady(ctx context.Context, f *framework.Framework) error {
	return waitUntilDeploymentReady(ctx, f, s.commonStack.dps)
}

func (s *albResourceStack) createHTTPRoute(ctx context.Context, f *framework.Framework, httpr *gwv1.HTTPRoute) error {
	f.Logger.Info("creating http route", "httpr", k8s.NamespacedName(httpr))
	return f.K8sClient.Create(ctx, httpr)
}

func (s *albResourceStack) createGRPCRoute(ctx context.Context, f *framework.Framework, grpcr *gwv1.GRPCRoute) error {
	f.Logger.Info("creating grpc route", "grpc", k8s.NamespacedName(grpcr))
	return f.K8sClient.Create(ctx, grpcr)
}

func (s *albResourceStack) updateGRPCRoute(ctx context.Context, f *framework.Framework, grpcr *gwv1.GRPCRoute) error {
	f.Logger.Info("updating grpc route", "grpc", k8s.NamespacedName(grpcr))
	return f.K8sClient.Update(ctx, grpcr)
}

func (s *albResourceStack) deleteHTTPRoute(ctx context.Context, f *framework.Framework, httpr *gwv1.HTTPRoute) error {
	return f.K8sClient.Delete(ctx, httpr)
}
