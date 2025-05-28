package gateway

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func newNLBResourceStack(dp *appsv1.Deployment, svc *corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, tcpr *gwalpha2.TCPRoute, baseName string, enablePodReadinessGate bool) *nlbResourceStack {

	commonStack := newCommonResourceStack(dp, svc, gwc, gw, lbc, baseName, enablePodReadinessGate)
	return &nlbResourceStack{
		tcpr:        tcpr,
		commonStack: commonStack,
	}
}

// resourceStack containing the deployment and service resources
type nlbResourceStack struct {
	commonStack *commonResourceStack
	tcpr        *gwalpha2.TCPRoute
}

func (s *nlbResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {
	return s.commonStack.Deploy(ctx, f, func(ctx context.Context, f *framework.Framework, namespace string) error {
		s.tcpr.Namespace = namespace
		return s.createTCPRoute(ctx, f)
	})
}

func (s *nlbResourceStack) ScaleDeployment(ctx context.Context, f *framework.Framework, numReplicas int32) error {
	return s.commonStack.ScaleDeployment(ctx, f, numReplicas)
}

func (s *nlbResourceStack) Cleanup(ctx context.Context, f *framework.Framework) {
	s.commonStack.Cleanup(ctx, f)
}

func (s *nlbResourceStack) GetLoadBalancerIngressHostname() string {
	return s.commonStack.GetLoadBalancerIngressHostname()
}

func (s *nlbResourceStack) GetStackName() string {
	return s.commonStack.GetStackName()
}

func (s *nlbResourceStack) getListenersPortMap() map[string]string {
	return s.commonStack.getListenersPortMap()
}

func (s *nlbResourceStack) getTargetGroupNodePortMap() map[string]string {
	return s.commonStack.getTargetGroupNodePortMap()
}

func (s *nlbResourceStack) getHealthCheckNodePort() string {
	return s.commonStack.getHealthCheckNodePort()
}

func (s *nlbResourceStack) waitUntilDeploymentReady(ctx context.Context, f *framework.Framework) error {
	return s.commonStack.waitUntilDeploymentReady(ctx, f)
}

func (s *nlbResourceStack) createTCPRoute(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("creating tcp route", "tcpr", k8s.NamespacedName(s.tcpr))
	return f.K8sClient.Create(ctx, s.tcpr)
}

func (s *nlbResourceStack) deleteTCPRoute(ctx context.Context, f *framework.Framework) error {
	return f.K8sClient.Delete(ctx, s.tcpr)
}
