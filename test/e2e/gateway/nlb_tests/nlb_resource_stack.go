package nlb_tests

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/gateway/test_resources"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func newNLBResourceStack(dps []*appsv1.Deployment, svcs []*corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, tgcs []*elbv2gw.TargetGroupConfiguration, tcpr []*gwalpha2.TCPRoute, udpr []*gwalpha2.UDPRoute, tlsr []*gwv1.TLSRoute, baseName string, namespaceLabels map[string]string) *NLBResourceStack {

	CommonStack := test_resources.NewCommonResourceStack(dps, svcs, gwc, gw, lbc, tgcs, nil, baseName, namespaceLabels)
	return &NLBResourceStack{
		Tcprs:       tcpr,
		Udprs:       udpr,
		Tlsrs:       tlsr,
		CommonStack: CommonStack,
	}
}

// NLBResourceStack containing the deployment and service resources
type NLBResourceStack struct {
	CommonStack *test_resources.CommonResourceStack
	Tcprs       []*gwalpha2.TCPRoute
	Udprs       []*gwalpha2.UDPRoute
	Tlsrs       []*gwv1.TLSRoute
}

func (s *NLBResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {
	return s.CommonStack.Deploy(ctx, f, func(ctx context.Context, f *framework.Framework, namespace string) error {

		for _, v := range s.Tcprs {
			v.Namespace = namespace
		}

		for _, v := range s.Udprs {
			v.Namespace = namespace
		}

		if s.Tlsrs != nil {
			for _, v := range s.Tlsrs {
				v.Namespace = namespace
			}
		}
		err := s.CreateTCPRoutes(ctx, f)
		if err != nil {
			return err
		}
		err = s.CreateUDPRoutes(ctx, f)
		if err != nil {
			return err
		}
		return s.CreateTLSRoutes(ctx, f)
	})
}

func (s *NLBResourceStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	if s == nil || s.CommonStack == nil {
		return nil
	}
	return s.CommonStack.Cleanup(ctx, f)
}

func (s *NLBResourceStack) GetLoadBalancerIngressHostname() string {
	return s.CommonStack.GetLoadBalancerIngressHostname()
}

func (s *NLBResourceStack) GetListenersPortMap() map[string]string {
	return s.CommonStack.GetListenersPortMap()
}

func (s *NLBResourceStack) waitUntilDeploymentReady(ctx context.Context, f *framework.Framework) error {
	return test_resources.WaitUntilDeploymentReady(ctx, f, s.CommonStack.Dps)
}

func (s *NLBResourceStack) GetNamespace() string {
	return s.CommonStack.Ns.Name
}

func (s *NLBResourceStack) CreateTCPRoutes(ctx context.Context, f *framework.Framework) error {
	for _, tcpr := range s.Tcprs {
		f.Logger.Info("creating tcp route", "tcpr", k8s.NamespacedName(tcpr))
		err := f.K8sClient.Create(ctx, tcpr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *NLBResourceStack) CreateUDPRoutes(ctx context.Context, f *framework.Framework) error {
	for _, udpr := range s.Udprs {
		f.Logger.Info("creating udp route", "udpr", k8s.NamespacedName(udpr))
		err := f.K8sClient.Create(ctx, udpr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *NLBResourceStack) CreateTLSRoutes(ctx context.Context, f *framework.Framework) error {
	if s.Tlsrs == nil {
		return nil
	}
	for _, tlsr := range s.Tlsrs {
		f.Logger.Info("creating tls route", "tlsr", k8s.NamespacedName(tlsr))
		err := f.K8sClient.Create(ctx, tlsr)
		if err != nil {
			return err
		}
	}
	return nil
}
