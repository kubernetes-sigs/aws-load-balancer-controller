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

func newALBResourceStack(dp *appsv1.Deployment, svc *corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, tgc *elbv2gw.TargetGroupConfiguration, lrc *elbv2gw.ListenerRuleConfiguration, httpr []*gwv1.HTTPRoute, baseName string, enablePodReadinessGate bool) *albResourceStack {

	commonStack := newCommonResourceStack([]*appsv1.Deployment{dp}, []*corev1.Service{svc}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgc}, []*elbv2gw.ListenerRuleConfiguration{lrc}, baseName, enablePodReadinessGate)
	return &albResourceStack{
		httprs:      httpr,
		commonStack: commonStack,
	}
}

// resourceStack containing the deployment and service resources
type albResourceStack struct {
	commonStack *commonResourceStack
	httprs      []*gwv1.HTTPRoute
}

func (s *albResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {
	return s.commonStack.Deploy(ctx, f, func(ctx context.Context, f *framework.Framework, namespace string) error {
		for _, httpr := range s.httprs {
			httpr.Namespace = namespace
			if err := s.createHTTPRoute(ctx, f, httpr); err != nil {
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

func (s *albResourceStack) deleteHTTPRoute(ctx context.Context, f *framework.Framework, httpr *gwv1.HTTPRoute) error {
	return f.K8sClient.Delete(ctx, httpr)
}
