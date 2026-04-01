package nlb_tests

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/gateway/alb_tests"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/gateway/test_resources"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type NLBTestStack struct {
	Resources *NLBResourceStack
}

func (s *NLBTestStack) Deploy(ctx context.Context, f *framework.Framework, auxiliaryStack *test_resources.AuxiliaryResourceStack, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, hasTLS bool, tlsMode gwv1.TLSModeType, readinessGateEnabled bool) error {
	dpTCP := test_resources.BuildDeploymentSpec(f.Options.TestImageRegistry)
	svcTCP := test_resources.BuildServiceSpec(map[string]string{})

	dpUDP := test_resources.BuildUDPDeploymentSpec()
	svcUDP := test_resources.BuildUDPServiceSpec()
	gwc := test_resources.BuildGatewayClassSpec("gateway.k8s.aws/nlb")

	if f.Options.IPFamily == framework.IPv6 {
		v6 := elbv2gw.LoadBalancerIpAddressTypeDualstack
		lbConfSpec.IpAddressType = &v6
	}

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

	if hasTLS {
		listeners = append(listeners, gwv1.Listener{
			Name:     "port443",
			Port:     443,
			Protocol: gwv1.TLSProtocolType,
			TLS: &gwv1.ListenerTLSConfig{
				Mode: &tlsMode,
				CertificateRefs: []gwv1.SecretObjectReference{
					{
						Name: "tls-cert",
					},
				},
			},
		})
	}

	tcprs := []*gwalpha2.TCPRoute{test_resources.BuildTCPRoute([]gwv1.ParentReference{}, []gwv1.BackendRef{})}
	if auxiliaryStack != nil {
		listeners = append(listeners, gwv1.Listener{
			Name:     "other-ns",
			Port:     5000,
			Protocol: gwv1.TCPProtocolType,
		})

		tcprs = append(tcprs, test_resources.BuildOtherNsRefTcpRoute("other-ns", auxiliaryStack.Ns))
	}

	gw := test_resources.BuildBasicGatewaySpec(gwc, listeners)

	lbc := test_resources.BuildLoadBalancerConfig(lbConfSpec)
	tgcTCP := test_resources.BuildTargetGroupConfig(test_resources.DefaultTgConfigName, tgConfSpec, svcTCP)
	tgcUDP := test_resources.BuildTargetGroupConfig(test_resources.UDPDefaultTgConfigName, tgConfSpec, svcUDP)
	udpr := test_resources.BuildUDPRoute("port8080")

	s.Resources = newNLBResourceStack([]*appsv1.Deployment{dpTCP, dpUDP}, []*corev1.Service{svcTCP, svcUDP}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgcTCP, tgcUDP}, tcprs, []*gwalpha2.UDPRoute{udpr}, nil, "nlb-gateway-e2e", test_resources.GetNamespaceLabels(readinessGateEnabled))

	return s.Resources.Deploy(ctx, f)
}

func (s *NLBTestStack) DeployTCPWeightedStack(ctx context.Context, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, readinessGateEnabled bool) error {
	dpTCP1 := test_resources.BuildCustomizableResponseDeploymentSpec("dp1", "yellow", f.Options.TestImageRegistry)
	dpTCP2 := test_resources.BuildCustomizableResponseDeploymentSpec("dp2", "green", f.Options.TestImageRegistry)
	svcTCP1 := test_resources.BuildServiceSpec(dpTCP1.Spec.Selector.MatchLabels)
	svcTCP2 := test_resources.BuildServiceSpec(dpTCP2.Spec.Selector.MatchLabels)
	svcTCP2.Name = svcTCP2.Name + "-2"

	gwc := test_resources.BuildGatewayClassSpec("gateway.k8s.aws/nlb")

	if f.Options.IPFamily == framework.IPv6 {
		v6 := elbv2gw.LoadBalancerIpAddressTypeDualstack
		lbConfSpec.IpAddressType = &v6
	}

	listeners := []gwv1.Listener{
		{
			Name:     "port80",
			Port:     80,
			Protocol: gwv1.TCPProtocolType,
		},
	}

	if lbConfSpec.ListenerConfigurations != nil {
		for _, lsr := range *lbConfSpec.ListenerConfigurations {
			if lsr.ProtocolPort == "TLS:443" {
				tlsMode := gwv1.TLSModeTerminate
				listeners = append(listeners, gwv1.Listener{
					Name:     "port443",
					Port:     443,
					Protocol: gwv1.TLSProtocolType,
					TLS: &gwv1.ListenerTLSConfig{
						Mode: &tlsMode,
						CertificateRefs: []gwv1.SecretObjectReference{
							{
								Name: "tls-cert",
							},
						},
					},
				})
				break
			}
		}
	}

	tcprs := []*gwalpha2.TCPRoute{test_resources.BuildTCPRoute([]gwv1.ParentReference{
		{
			Name: test_resources.DefaultName,
		},
	}, []gwv1.BackendRef{
		{
			BackendObjectReference: gwalpha2.BackendObjectReference{
				Name: gwv1.ObjectName(svcTCP1.Name),
				Port: &svcTCP1.Spec.Ports[0].Port,
			},
		},
		{
			BackendObjectReference: gwalpha2.BackendObjectReference{
				Name: gwv1.ObjectName(svcTCP2.Name),
				Port: &svcTCP2.Spec.Ports[0].Port,
			},
		},
	})}

	gw := test_resources.BuildBasicGatewaySpec(gwc, listeners)

	lbc := test_resources.BuildLoadBalancerConfig(lbConfSpec)
	tgcTCP1 := test_resources.BuildTargetGroupConfig(svcTCP1.Name, tgConfSpec, svcTCP1)
	tgcTCP2 := test_resources.BuildTargetGroupConfig(svcTCP2.Name, tgConfSpec, svcTCP2)

	s.Resources = newNLBResourceStack([]*appsv1.Deployment{dpTCP1, dpTCP2}, []*corev1.Service{svcTCP1, svcTCP2}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgcTCP1, tgcTCP2}, tcprs, []*gwalpha2.UDPRoute{}, nil, "nlb-gateway-e2e", test_resources.GetNamespaceLabels(readinessGateEnabled))

	return s.Resources.Deploy(ctx, f)
}

func (s *NLBTestStack) DeployTCP_UDP(ctx context.Context, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, readinessGateEnabled bool) error {
	dpUDP := test_resources.BuildUDPDeploymentSpec()
	svcUDP := test_resources.BuildUDPServiceSpec()
	gwc := test_resources.BuildGatewayClassSpec("gateway.k8s.aws/nlb")

	if f.Options.IPFamily == framework.IPv6 {
		v6 := elbv2gw.LoadBalancerIpAddressTypeDualstack
		lbConfSpec.IpAddressType = &v6
	}

	listeners := []gwv1.Listener{
		{
			Name:     "port80tcp",
			Port:     80,
			Protocol: gwv1.TCPProtocolType,
		},
		{
			Name:     "port80udp",
			Port:     80,
			Protocol: gwv1.UDPProtocolType,
		},
	}

	tcprs := []*gwalpha2.TCPRoute{}

	gw := test_resources.BuildBasicGatewaySpec(gwc, listeners)

	lbc := test_resources.BuildLoadBalancerConfig(lbConfSpec)
	tgcUDP := test_resources.BuildTargetGroupConfig(test_resources.UDPDefaultTgConfigName, tgConfSpec, svcUDP)
	udpr := test_resources.BuildUDPRoute("port80udp")

	s.Resources = newNLBResourceStack([]*appsv1.Deployment{dpUDP}, []*corev1.Service{svcUDP}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgcUDP}, tcprs, []*gwalpha2.UDPRoute{udpr}, nil, "nlb-gateway-e2e", test_resources.GetNamespaceLabels(readinessGateEnabled))

	return s.Resources.Deploy(ctx, f)
}

func (s *NLBTestStack) DeployQUIC(ctx context.Context, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, namespaceLabels map[string]string) error {
	dpUDP := test_resources.BuildUDPDeploymentSpec()
	svcUDP := test_resources.BuildUDPServiceSpec()

	dpUDP.Spec.Template.Annotations = make(map[string]string)
	dpUDP.Spec.Template.Annotations["service.beta.kubernetes.io/aws-load-balancer-quic-enabled-containers"] = "app"
	gwc := test_resources.BuildGatewayClassSpec("gateway.k8s.aws/nlb")

	if f.Options.IPFamily == framework.IPv6 {
		v6 := elbv2gw.LoadBalancerIpAddressTypeDualstack
		lbConfSpec.IpAddressType = &v6
	}

	gw := test_resources.BuildBasicGatewaySpec(gwc, []gwv1.Listener{
		{
			Name:     "udp-listener",
			Port:     8080,
			Protocol: "UDP",
		},
	})

	lbc := test_resources.BuildLoadBalancerConfig(lbConfSpec)
	tgcUDP := test_resources.BuildTargetGroupConfig(svcUDP.Name, tgConfSpec, svcUDP)

	udpr := test_resources.BuildUDPRoute("udp-listener")
	udpr.Name = "udp-route-quic"

	s.Resources = newNLBResourceStack([]*appsv1.Deployment{dpUDP}, []*corev1.Service{svcUDP}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgcUDP}, []*gwalpha2.TCPRoute{}, []*gwalpha2.UDPRoute{udpr}, nil, "nlb-gateway-quic-e2e", namespaceLabels)

	return s.Resources.Deploy(ctx, f)
}

func (s *NLBTestStack) DeployTCP_QUIC(ctx context.Context, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, namespaceLabels map[string]string) error {
	dpUDP := test_resources.BuildUDPDeploymentSpec()
	svcUDP := test_resources.BuildUDPServiceSpec()

	dpUDP.Spec.Template.Annotations = make(map[string]string)
	dpUDP.Spec.Template.Annotations["service.beta.kubernetes.io/aws-load-balancer-quic-enabled-containers"] = "app"

	gwc := test_resources.BuildGatewayClassSpec("gateway.k8s.aws/nlb")

	gw := test_resources.BuildBasicGatewaySpec(gwc, []gwv1.Listener{
		{
			Name:     "udp-listener",
			Port:     8080,
			Protocol: "UDP",
		},
		{
			Name:     "tcp-listener",
			Port:     8080,
			Protocol: "TCP",
		},
	})

	lbc := test_resources.BuildLoadBalancerConfig(lbConfSpec)
	tgcUDP := test_resources.BuildTargetGroupConfig(svcUDP.Name, tgConfSpec, svcUDP)

	udpr := test_resources.BuildUDPRoute("udp-listener")
	udpr.Name = "udp-route-quic"

	s.Resources = newNLBResourceStack([]*appsv1.Deployment{dpUDP}, []*corev1.Service{svcUDP}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgcUDP}, []*gwalpha2.TCPRoute{}, []*gwalpha2.UDPRoute{udpr}, nil, "nlb-gateway-tcp-udp-quic-e2e", namespaceLabels)

	return s.Resources.Deploy(ctx, f)
}

// DeployWithDefaultTGC deploys an NLB stack with a gateway-level default TGC referenced by the LBC,
// plus two services: svc1 inherits defaults, svc2 has a service-level TGC override.
func (s *NLBTestStack) DeployWithDefaultTGC(ctx context.Context, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, defaultTGC *elbv2gw.TargetGroupConfiguration, svcTgSpec elbv2gw.TargetGroupConfigurationSpec, readinessGateEnabled bool) error {
	if f.Options.IPFamily == framework.IPv6 {
		v6 := elbv2gw.LoadBalancerIpAddressTypeDualstack
		lbConfSpec.IpAddressType = &v6
	}

	listeners := []gwv1.Listener{
		{
			Name:     "port80",
			Port:     80,
			Protocol: gwv1.TCPProtocolType,
		},
	}

	dpTCP := test_resources.BuildDeploymentSpec(f.Options.TestImageRegistry)
	svcTCP1 := test_resources.BuildServiceSpec(map[string]string{})
	svcTCP2 := test_resources.BuildServiceSpec(map[string]string{})
	svcTCP2.Name = "echoserver-v2"
	svcTgc := test_resources.BuildTargetGroupConfig("svc2-tgc", svcTgSpec, svcTCP2)

	port := gwalpha2.PortNumber(80)
	tcprs := []*gwalpha2.TCPRoute{test_resources.BuildTCPRoute([]gwv1.ParentReference{}, []gwalpha2.BackendRef{
		{
			BackendObjectReference: gwalpha2.BackendObjectReference{
				Name: gwalpha2.ObjectName(svcTCP1.Name),
				Port: &port,
			},
		},
		{
			BackendObjectReference: gwalpha2.BackendObjectReference{
				Name: gwalpha2.ObjectName(svcTCP2.Name),
				Port: &port,
			},
		},
	})}

	gwc := test_resources.BuildGatewayClassSpec("gateway.k8s.aws/nlb")
	gw := test_resources.BuildBasicGatewaySpec(gwc, listeners)
	lbc := test_resources.BuildLoadBalancerConfig(lbConfSpec)

	s.Resources = newNLBResourceStack(
		[]*appsv1.Deployment{dpTCP},
		[]*corev1.Service{svcTCP1, svcTCP2},
		gwc, gw, lbc,
		[]*elbv2gw.TargetGroupConfiguration{defaultTGC, svcTgc},
		tcprs, []*gwalpha2.UDPRoute{}, nil,
		"nlb-gateway-e2e", test_resources.GetNamespaceLabels(readinessGateEnabled),
	)
	return s.Resources.Deploy(ctx, f)
}

func (s *NLBTestStack) DeployFrontendNLB(ctx context.Context, albStack alb_tests.ALBTestStack, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, hasTLS bool, readinessGateEnabled bool) error {
	gwc := test_resources.BuildGatewayClassSpec("gateway.k8s.aws/nlb")

	if f.Options.IPFamily == framework.IPv6 {
		v6 := elbv2gw.LoadBalancerIpAddressTypeDualstack
		lbConfSpec.IpAddressType = &v6
	}

	listeners := []gwv1.Listener{
		{
			Name:     "port80",
			Port:     80,
			Protocol: gwv1.TCPProtocolType,
		},
	}

	tcprs := []*gwalpha2.TCPRoute{test_resources.BuildFENLBTCPRoute(albStack.Resources.CommonStack.Gw.Name, albStack.Resources.CommonStack.Gw.Namespace, gwalpha2.PortNumber(80))}

	if hasTLS {
		listeners = append(listeners, gwv1.Listener{
			Name:     "port443",
			Port:     443,
			Protocol: gwv1.TCPProtocolType,
		})
		tcpForHTTPS := test_resources.BuildFENLBTCPRoute(albStack.Resources.CommonStack.Gw.Name, albStack.Resources.CommonStack.Gw.Namespace, gwalpha2.PortNumber(443))
		tcprs = append(tcprs, tcpForHTTPS)

	}

	gw := test_resources.BuildBasicGatewaySpec(gwc, listeners)

	lbc := test_resources.BuildLoadBalancerConfig(lbConfSpec)

	s.Resources = newNLBResourceStack([]*appsv1.Deployment{}, []*corev1.Service{}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{}, tcprs, []*gwalpha2.UDPRoute{}, nil, "nlb-gateway-e2e", test_resources.GetNamespaceLabels(readinessGateEnabled))

	err := s.Resources.Deploy(ctx, f)
	if err != nil {
		return err
	}

	// The special TGC is just to support HTTPS and HTTP health check routes on the same underlying test_resources.
	if !hasTLS {
		return nil
	}

	// Hack to get TargetGroupConfiguration working correctly, as it needs the namespace which is allocated in the deploy step.

	http := elbv2gw.TargetGroupHealthCheckProtocolHTTP
	https := elbv2gw.TargetGroupHealthCheckProtocolHTTPS

	return test_resources.CreateTargetGroupConfigs(ctx, f, []*elbv2gw.TargetGroupConfiguration{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("alb-https-hc-config"),
				Namespace: albStack.Resources.CommonStack.Gw.Namespace,
			},
			Spec: elbv2gw.TargetGroupConfigurationSpec{
				TargetReference: &elbv2gw.Reference{
					Group: nil,
					Kind:  awssdk.String("Gateway"),
					Name:  albStack.Resources.CommonStack.Gw.Name,
				},
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{HealthCheckProtocol: &http},
				},
				RouteConfigurations: []elbv2gw.RouteConfiguration{
					{
						RouteIdentifier: elbv2gw.RouteIdentifier{
							RouteName:      tcprs[1].Name,
							RouteNamespace: tcprs[1].Namespace,
							RouteKind:      "TCPRoute",
						},
						TargetGroupProps: elbv2gw.TargetGroupProps{
							HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{HealthCheckProtocol: &https},
						},
					},
				},
			},
		},
	})
}

func (s *NLBTestStack) CreateFENLBReferenceGrant(ctx context.Context, f *framework.Framework, albNamespace *corev1.Namespace) (*gwbeta1.ReferenceGrant, error) {
	refGrant := &gwbeta1.ReferenceGrant{

		ObjectMeta: metav1.ObjectMeta{
			Name:      "refgrant-fe-nlb",
			Namespace: albNamespace.Name,
		},
		Spec: gwbeta1.ReferenceGrantSpec{
			From: []gwbeta1.ReferenceGrantFrom{
				{
					Group:     gwbeta1.Group(gwbeta1.GroupName),
					Kind:      gwbeta1.Kind("TCPRoute"),
					Namespace: gwbeta1.Namespace(s.Resources.CommonStack.Ns.Name),
				},
			},
			To: []gwbeta1.ReferenceGrantTo{
				{
					Kind:  "Gateway",
					Group: gwbeta1.Group(gwbeta1.GroupName),
				},
			},
		},
	}

	if err := test_resources.CreateReferenceGrants(ctx, f, []*gwbeta1.ReferenceGrant{refGrant}); err != nil {
		return nil, err
	}

	return refGrant, nil
}

func (s *NLBTestStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	if s.Resources == nil {
		return nil
	}
	return s.Resources.Cleanup(ctx, f)
}

func (s *NLBTestStack) GetLoadBalancerIngressHostName() string {
	return s.Resources.GetLoadBalancerIngressHostname()
}

func (s *NLBTestStack) GetNamespace() string {
	return s.Resources.GetNamespace()
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

func validateL4RouteStatusNotPermitted(tf *framework.Framework, stack NLBTestStack, hasTLS bool) {
	tcpRouteListenerInfo := []test_resources.ListenerValidationInfo{
		{
			ListenerName:       "port80",
			ParentKind:         "Gateway",
			ResolvedRefReason:  "ResolvedRefs",
			ResolvedRefsStatus: "True",
			AcceptedReason:     "Accepted",
			AcceptedStatus:     "True",
		},
	}

	if hasTLS {
		tcpRouteListenerInfo = append(tcpRouteListenerInfo, test_resources.ListenerValidationInfo{
			ListenerName:       "port443",
			ParentKind:         "Gateway",
			ResolvedRefReason:  "ResolvedRefs",
			ResolvedRefsStatus: "True",
			AcceptedReason:     "Accepted",
			AcceptedStatus:     "True",
		})
	} else {
		tcpRouteListenerInfo = append(tcpRouteListenerInfo, test_resources.ListenerValidationInfo{
			ListenerName:       "port443",
			ParentKind:         "Gateway",
			ResolvedRefReason:  "ResolvedRefs",
			ResolvedRefsStatus: "True",
			AcceptedReason:     "NoMatchingParent",
			AcceptedStatus:     "False",
		})
	}

	tcpValidationInfo := map[string]test_resources.RouteValidationInfo{
		k8s.NamespacedName(stack.Resources.Tcprs[0]).String(): {
			ParentGatewayName: stack.Resources.CommonStack.Gw.Name,
			ListenerInfo:      tcpRouteListenerInfo,
		},
		k8s.NamespacedName(stack.Resources.Tcprs[1]).String(): {
			ParentGatewayName: stack.Resources.CommonStack.Gw.Name,
			ListenerInfo: []test_resources.ListenerValidationInfo{
				{
					ListenerName:       "other-ns",
					ParentKind:         "Gateway",
					ResolvedRefReason:  "RefNotPermitted",
					ResolvedRefsStatus: "False",
					AcceptedReason:     "Accepted",
					AcceptedStatus:     "True",
				},
			},
		},
	}

	udpValidationInfo := map[string]test_resources.RouteValidationInfo{
		k8s.NamespacedName(stack.Resources.Udprs[0]).String(): {
			ParentGatewayName: stack.Resources.CommonStack.Gw.Name,
			ListenerInfo: []test_resources.ListenerValidationInfo{
				{
					ListenerName:       "port8080",
					ParentKind:         "Gateway",
					ResolvedRefReason:  "ResolvedRefs",
					ResolvedRefsStatus: "True",
					AcceptedReason:     "Accepted",
					AcceptedStatus:     "True",
				},
			},
		},
	}

	test_resources.ValidateRouteStatus(tf, stack.Resources.Tcprs, tcpRouteStatusConverter, tcpValidationInfo)
	test_resources.ValidateRouteStatus(tf, stack.Resources.Udprs, udpRouteStatusConverter, udpValidationInfo)
}

func validateL4RouteStatusPermitted(tf *framework.Framework, stack NLBTestStack, hasTLS bool) {
	tcpRouteListenerInfo := []test_resources.ListenerValidationInfo{
		{
			ListenerName:       "port80",
			ParentKind:         "Gateway",
			ResolvedRefReason:  "ResolvedRefs",
			ResolvedRefsStatus: "True",
			AcceptedReason:     "Accepted",
			AcceptedStatus:     "True",
		},
	}

	if hasTLS {
		tcpRouteListenerInfo = append(tcpRouteListenerInfo, test_resources.ListenerValidationInfo{
			ListenerName:       "port443",
			ParentKind:         "Gateway",
			ResolvedRefReason:  "ResolvedRefs",
			ResolvedRefsStatus: "True",
			AcceptedReason:     "Accepted",
			AcceptedStatus:     "True",
		})
	} else {
		tcpRouteListenerInfo = append(tcpRouteListenerInfo, test_resources.ListenerValidationInfo{
			ListenerName:       "port443",
			ParentKind:         "Gateway",
			ResolvedRefReason:  "ResolvedRefs",
			ResolvedRefsStatus: "True",
			AcceptedReason:     "NoMatchingParent",
			AcceptedStatus:     "False",
		})
	}

	tcpValidationInfo := map[string]test_resources.RouteValidationInfo{
		k8s.NamespacedName(stack.Resources.Tcprs[0]).String(): {
			ParentGatewayName: stack.Resources.CommonStack.Gw.Name,
			ListenerInfo:      tcpRouteListenerInfo,
		},
		k8s.NamespacedName(stack.Resources.Tcprs[1]).String(): {
			ParentGatewayName: stack.Resources.CommonStack.Gw.Name,
			ListenerInfo: []test_resources.ListenerValidationInfo{
				{
					ListenerName:       "other-ns",
					ParentKind:         "Gateway",
					ResolvedRefReason:  "ResolvedRefs",
					ResolvedRefsStatus: "True",
					AcceptedReason:     "Accepted",
					AcceptedStatus:     "True",
				},
			},
		},
	}

	udpValidationInfo := map[string]test_resources.RouteValidationInfo{
		k8s.NamespacedName(stack.Resources.Udprs[0]).String(): {
			ParentGatewayName: stack.Resources.CommonStack.Gw.Name,
			ListenerInfo: []test_resources.ListenerValidationInfo{
				{
					ListenerName:       "port8080",
					ParentKind:         "Gateway",
					ResolvedRefReason:  "ResolvedRefs",
					ResolvedRefsStatus: "True",
					AcceptedReason:     "Accepted",
					AcceptedStatus:     "True",
				},
			},
		},
	}
	test_resources.ValidateRouteStatus(tf, stack.Resources.Tcprs, tcpRouteStatusConverter, tcpValidationInfo)
	test_resources.ValidateRouteStatus(tf, stack.Resources.Udprs, udpRouteStatusConverter, udpValidationInfo)
}

func tcpRouteStatusConverter(tf *framework.Framework, i interface{}) (gwv1.RouteStatus, types.NamespacedName, error) {
	tcpR := i.(*gwalpha2.TCPRoute)
	retrievedRoute := gwalpha2.TCPRoute{}
	err := tf.K8sClient.Get(context.Background(), k8s.NamespacedName(tcpR), &retrievedRoute)
	if err != nil {
		return gwv1.RouteStatus{}, types.NamespacedName{}, err
	}
	return retrievedRoute.Status.RouteStatus, k8s.NamespacedName(&retrievedRoute), nil
}

func udpRouteStatusConverter(tf *framework.Framework, i interface{}) (gwv1.RouteStatus, types.NamespacedName, error) {
	udpR := i.(*gwalpha2.UDPRoute)
	retrievedRoute := gwalpha2.UDPRoute{}
	err := tf.K8sClient.Get(context.Background(), k8s.NamespacedName(udpR), &retrievedRoute)
	if err != nil {
		return gwv1.RouteStatus{}, types.NamespacedName{}, err
	}
	return retrievedRoute.Status.RouteStatus, k8s.NamespacedName(&retrievedRoute), nil
}

func weightedRequestValidation(tf *framework.Framework, url string) {
	bm := &test_resources.BodyMatcher{
		ResponseCount: map[string]int{},
	}
	for i := 0; i < 100; i++ {
		_ = tf.HTTPVerifier.VerifyURL(url, bm)
	}
	// We have configured the weighted listener to have two body types.
	// We aren't interested in verifying that the NLB is correctly splitting traffic.
	Expect(len(bm.ResponseCount)).To(Equal(2))
}

func (s *NLBTestStack) DeployListenerMismatch(ctx context.Context, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, readinessGateEnabled bool) error {
	dpTCP := test_resources.BuildDeploymentSpec(f.Options.TestImageRegistry)
	svcTCP := test_resources.BuildServiceSpec(map[string]string{})
	gwc := test_resources.BuildGatewayClassSpec("gateway.k8s.aws/nlb")

	if f.Options.IPFamily == framework.IPv6 {
		v6 := elbv2gw.LoadBalancerIpAddressTypeDualstack
		lbConfSpec.IpAddressType = &v6
	}

	listeners := []gwv1.Listener{
		{
			Name:     "listener-exists",
			Port:     80,
			Protocol: gwv1.TCPProtocolType,
		},
		{
			Name:     "listener-other",
			Port:     8080,
			Protocol: gwv1.TCPProtocolType,
		},
	}

	tcprs := []*gwalpha2.TCPRoute{test_resources.BuildTCPRouteWithMismatchedParentRefs()}
	gw := test_resources.BuildBasicGatewaySpec(gwc, listeners)
	lbc := test_resources.BuildLoadBalancerConfig(lbConfSpec)
	tgcTCP := test_resources.BuildTargetGroupConfig(test_resources.DefaultTgConfigName, tgConfSpec, svcTCP)

	s.Resources = newNLBResourceStack([]*appsv1.Deployment{dpTCP}, []*corev1.Service{svcTCP}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgcTCP}, tcprs, []*gwalpha2.UDPRoute{}, nil, "nlb-gateway-e2e", test_resources.GetNamespaceLabels(readinessGateEnabled))

	return s.Resources.Deploy(ctx, f)
}

func validateTCPRouteListenerMismatch(tf *framework.Framework, stack NLBTestStack) {
	validationInfo := map[string]test_resources.RouteValidationInfo{
		k8s.NamespacedName(stack.Resources.Tcprs[0]).String(): {
			ParentGatewayName: stack.Resources.CommonStack.Gw.Name,
			ListenerInfo: []test_resources.ListenerValidationInfo{
				{
					ListenerName:       "listener-exists",
					ParentKind:         "Gateway",
					ResolvedRefReason:  "ResolvedRefs",
					ResolvedRefsStatus: "True",
					AcceptedReason:     "Accepted",
					AcceptedStatus:     "True",
				},
				{
					ListenerName:       "listener-nonexist",
					ParentKind:         "Gateway",
					ResolvedRefReason:  "ResolvedRefs",
					ResolvedRefsStatus: "True",
					AcceptedReason:     "NoMatchingParent",
					AcceptedStatus:     "False",
				},
			},
		},
	}
	test_resources.ValidateRouteStatus(tf, stack.Resources.Tcprs, tcpRouteStatusConverter, validationInfo)

	test_resources.ValidateGatewayStatus(tf, stack.Resources.CommonStack.Gw, test_resources.GatewayValidationInfo{
		Conditions: []test_resources.GatewayConditionValidation{
			{
				ConditionType:   gwv1.GatewayConditionProgrammed,
				ConditionStatus: "True",
				ConditionReason: "Programmed",
			},
			{
				ConditionType:   gwv1.GatewayConditionAccepted,
				ConditionStatus: "True",
				ConditionReason: "Accepted",
			},
		},
		Listeners: []test_resources.GatewayListenerValidation{
			{
				ListenerName:   "listener-exists",
				AttachedRoutes: 1,
				Conditions: []test_resources.ListenerConditionValidation{
					{
						ConditionType:   gwv1.ListenerConditionAccepted,
						ConditionStatus: "True",
						ConditionReason: "Accepted",
					},
					{
						ConditionType:   gwv1.ListenerConditionProgrammed,
						ConditionStatus: "True",
						ConditionReason: "Programmed",
					},
				},
			},
			{
				ListenerName:   "listener-other",
				AttachedRoutes: 0,
				Conditions: []test_resources.ListenerConditionValidation{
					{
						ConditionType:   gwv1.ListenerConditionAccepted,
						ConditionStatus: "True",
						ConditionReason: "Accepted",
					},
					{
						ConditionType:   gwv1.ListenerConditionProgrammed,
						ConditionStatus: "True",
						ConditionReason: "Programmed",
					},
				},
			},
		},
	})
}
