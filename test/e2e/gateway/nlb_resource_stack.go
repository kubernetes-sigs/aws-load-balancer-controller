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

func newNLBResourceStack(dps []*appsv1.Deployment, svcs []*corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, tgcs []*elbv2gw.TargetGroupConfiguration, tcpr *gwalpha2.TCPRoute, udpr *gwalpha2.UDPRoute, tlsr *gwalpha2.TLSRoute, baseName string, enablePodReadinessGate bool) *nlbResourceStack {

	commonStack := newCommonResourceStack(dps, svcs, gwc, gw, lbc, tgcs, baseName, enablePodReadinessGate)
	return &nlbResourceStack{
		tcpr:        tcpr,
		udpr:        udpr,
		tlsr:        tlsr,
		commonStack: commonStack,
	}
}

// resourceStack containing the deployment and service resources
type nlbResourceStack struct {
	commonStack *commonResourceStack
	tcpr        *gwalpha2.TCPRoute
	udpr        *gwalpha2.UDPRoute
	tlsr        *gwalpha2.TLSRoute
}

func (s *nlbResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {
	return s.commonStack.Deploy(ctx, f, func(ctx context.Context, f *framework.Framework, namespace string) error {
		s.tcpr.Namespace = namespace
		s.udpr.Namespace = namespace
		if s.tlsr != nil {
			s.tlsr.Namespace = namespace
		}
		err := s.createTCPRoute(ctx, f)
		if err != nil {
			return err
		}
		err = s.createUDPRoute(ctx, f)
		if err != nil {
			return err
		}
		return s.createTLSRoute(ctx, f)
	})
}

func (s *nlbResourceStack) Cleanup(ctx context.Context, f *framework.Framework) {
	s.commonStack.Cleanup(ctx, f)
}

func (s *nlbResourceStack) GetLoadBalancerIngressHostname() string {
	return s.commonStack.GetLoadBalancerIngressHostname()
}

func (s *nlbResourceStack) getListenersPortMap() map[string]string {
	return s.commonStack.getListenersPortMap()
}

func (s *nlbResourceStack) waitUntilDeploymentReady(ctx context.Context, f *framework.Framework) error {
	return s.commonStack.waitUntilDeploymentReady(ctx, f)
}

func (s *nlbResourceStack) createTCPRoute(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("creating tcp route", "tcpr", k8s.NamespacedName(s.tcpr))
	return f.K8sClient.Create(ctx, s.tcpr)
}

func (s *nlbResourceStack) createUDPRoute(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("creating udp route", "udpr", k8s.NamespacedName(s.udpr))
	return f.K8sClient.Create(ctx, s.udpr)
}

func (s *nlbResourceStack) createTLSRoute(ctx context.Context, f *framework.Framework) error {
	if s.tlsr == nil {
		return nil
	}
	f.Logger.Info("creating tls route", "tlsr", k8s.NamespacedName(s.tlsr))
	return f.K8sClient.Create(ctx, s.tlsr)
}
