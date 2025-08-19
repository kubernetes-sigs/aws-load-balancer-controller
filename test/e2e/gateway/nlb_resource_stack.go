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

func newNLBResourceStack(dps []*appsv1.Deployment, svcs []*corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, tgcs []*elbv2gw.TargetGroupConfiguration, tcpr []*gwalpha2.TCPRoute, udpr []*gwalpha2.UDPRoute, tlsr []*gwalpha2.TLSRoute, baseName string, enablePodReadinessGate bool) *nlbResourceStack {

	commonStack := newCommonResourceStack(dps, svcs, gwc, gw, lbc, tgcs, nil, baseName, enablePodReadinessGate)
	return &nlbResourceStack{
		tcprs:       tcpr,
		udprs:       udpr,
		tlsrs:       tlsr,
		commonStack: commonStack,
	}
}

// resourceStack containing the deployment and service resources
type nlbResourceStack struct {
	commonStack *commonResourceStack
	tcprs       []*gwalpha2.TCPRoute
	udprs       []*gwalpha2.UDPRoute
	tlsrs       []*gwalpha2.TLSRoute
}

func (s *nlbResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {
	return s.commonStack.Deploy(ctx, f, func(ctx context.Context, f *framework.Framework, namespace string) error {

		for _, v := range s.tcprs {
			v.Namespace = namespace
		}

		for _, v := range s.udprs {
			v.Namespace = namespace
		}

		if s.tlsrs != nil {
			for _, v := range s.tlsrs {
				v.Namespace = namespace
			}
		}
		err := s.createTCPRoutes(ctx, f)
		if err != nil {
			return err
		}
		err = s.createUDPRoutes(ctx, f)
		if err != nil {
			return err
		}
		return s.createTLSRoutes(ctx, f)
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
	return waitUntilDeploymentReady(ctx, f, s.commonStack.dps)
}

func (s *nlbResourceStack) createTCPRoutes(ctx context.Context, f *framework.Framework) error {
	for _, tcpr := range s.tcprs {
		f.Logger.Info("creating tcp route", "tcpr", k8s.NamespacedName(tcpr))
		err := f.K8sClient.Create(ctx, tcpr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *nlbResourceStack) createUDPRoutes(ctx context.Context, f *framework.Framework) error {
	for _, udpr := range s.udprs {
		f.Logger.Info("creating udp route", "udpr", k8s.NamespacedName(udpr))
		err := f.K8sClient.Create(ctx, udpr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *nlbResourceStack) createTLSRoutes(ctx context.Context, f *framework.Framework) error {
	if s.tlsrs == nil {
		return nil
	}
	for _, tlsr := range s.tlsrs {
		f.Logger.Info("creating tls route", "tlsr", k8s.NamespacedName(tlsr))
		err := f.K8sClient.Create(ctx, tlsr)
		if err != nil {
			return err
		}
	}
	return nil
}
