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

func newALBResourceStack(dp *appsv1.Deployment, svc *corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, tgc *elbv2gw.TargetGroupConfiguration, httpr *gwv1.HTTPRoute, baseName string, enablePodReadinessGate bool) *albResourceStack {

	commonStack := newCommonResourceStack(dp, svc, gwc, gw, lbc, tgc, baseName, enablePodReadinessGate)
	return &albResourceStack{
		httpr:       httpr,
		commonStack: commonStack,
	}
}

// resourceStack containing the deployment and service resources
type albResourceStack struct {
	commonStack *commonResourceStack
	httpr       *gwv1.HTTPRoute
}

func (s *albResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {
	return s.commonStack.Deploy(ctx, f, func(ctx context.Context, f *framework.Framework, namespace string) error {
		s.httpr.Namespace = namespace
		return s.createHTTPRoute(ctx, f)
	})
}

func (s *albResourceStack) ScaleDeployment(ctx context.Context, f *framework.Framework, numReplicas int32) error {
	return s.commonStack.ScaleDeployment(ctx, f, numReplicas)
}

func (s *albResourceStack) Cleanup(ctx context.Context, f *framework.Framework) {
	s.commonStack.Cleanup(ctx, f)
}

func (s *albResourceStack) GetLoadBalancerIngressHostname() string {
	return s.commonStack.GetLoadBalancerIngressHostname()
}

func (s *albResourceStack) GetStackName() string {
	return s.commonStack.GetStackName()
}

func (s *albResourceStack) getListenersPortMap() map[string]string {
	return s.commonStack.getListenersPortMap()
}

func (s *albResourceStack) getTargetGroupNodePortMap() map[string]string {
	res := s.commonStack.getTargetGroupNodePortMap()

	for p := range res {
		// TODO - kinda a hack to get HTTP to work.
		if res[p] == string(corev1.ProtocolTCP) {
			res[p] = "HTTP"
		}
	}

	return res
}

func (s *albResourceStack) getHealthCheckNodePort() string {
	return s.commonStack.getHealthCheckNodePort()
}

func (s *albResourceStack) waitUntilDeploymentReady(ctx context.Context, f *framework.Framework) error {
	return s.commonStack.waitUntilDeploymentReady(ctx, f)
}

func (s *albResourceStack) createHTTPRoute(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("creating http route", "httpr", k8s.NamespacedName(s.httpr))
	return f.K8sClient.Create(ctx, s.httpr)
}

func (s *albResourceStack) deleteHTTPRoute(ctx context.Context, f *framework.Framework) error {
	return f.K8sClient.Delete(ctx, s.httpr)
}
