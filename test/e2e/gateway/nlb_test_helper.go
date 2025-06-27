package gateway

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type NLBTestStack struct {
	nlbResourceStack *nlbResourceStack
}

func (s *NLBTestStack) Deploy(ctx context.Context, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec) error {
	dpTCP := buildDeploymentSpec(f.Options.TestImageRegistry)
	svcTCP := buildServiceSpec()

	dpUDP := buildUDPDeploymentSpec()
	svcUDP := buildUDPServiceSpec()
	gwc := buildGatewayClassSpec("gateway.k8s.aws/nlb")

	listeners := []gwv1.Listener{
		{
			Name:     "port80",
			Port:     80,
			Protocol: gwv1.TCPProtocolType,
		},
		{
			Name:     "port8080",
			Port:     8080,
			Protocol: gwv1.UDPProtocolType,
		},
	}

	var tlsr *gwalpha2.TLSRoute
	if lbConfSpec.ListenerConfigurations != nil {
		for _, lsr := range *lbConfSpec.ListenerConfigurations {
			if lsr.ProtocolPort == "TLS:443" {
				listeners = append(listeners, gwv1.Listener{
					Name:     "port443",
					Port:     443,
					Protocol: gwv1.TLSProtocolType,
				})
				//tlsr = buildTLSRoute()
				break
			}
		}
	}
	gw := buildBasicGatewaySpec(gwc, listeners)

	lbc := buildLoadBalancerConfig(lbConfSpec)
	tgcTCP := buildTargetGroupConfig(defaultTgConfigName, tgConfSpec, svcTCP)
	tgcUDP := buildTargetGroupConfig(udpDefaultTgConfigName, tgConfSpec, svcUDP)
	tcpr := buildTCPRoute()
	udpr := buildUDPRoute()
	s.nlbResourceStack = newNLBResourceStack([]*appsv1.Deployment{dpTCP, dpUDP}, []*corev1.Service{svcTCP, svcUDP}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgcTCP, tgcUDP}, tcpr, udpr, tlsr, "nlb-gateway-e2e", false)

	return s.nlbResourceStack.Deploy(ctx, f)
}

func (s *NLBTestStack) Cleanup(ctx context.Context, f *framework.Framework) {
	s.nlbResourceStack.Cleanup(ctx, f)
}

func (s *NLBTestStack) GetLoadBalancerIngressHostName() string {
	return s.nlbResourceStack.GetLoadBalancerIngressHostname()
}

func (s *NLBTestStack) GetWorkerNodes(ctx context.Context, f *framework.Framework) ([]corev1.Node, error) {
	allNodes := &corev1.NodeList{}
	err := f.K8sClient.List(ctx, allNodes)
	if err != nil {
		return nil, err
	}
	nodeList := []corev1.Node{}
	for _, node := range allNodes.Items {
		if _, notarget := node.Labels["node.kubernetes.io/exclude-from-external-load-balancers"]; !notarget {
			nodeList = append(nodeList, node)
		}
	}
	return nodeList, nil
}
