package gateway

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
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type NLBTestStack struct {
	nlbResourceStack *nlbResourceStack
}

func (s *NLBTestStack) Deploy(ctx context.Context, f *framework.Framework, auxiliaryStack *auxiliaryResourceStack, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, readinessGateEnabled bool) error {
	dpTCP := buildDeploymentSpec(f.Options.TestImageRegistry)
	svcTCP := buildServiceSpec(map[string]string{})

	dpUDP := buildUDPDeploymentSpec()
	svcUDP := buildUDPServiceSpec()
	gwc := buildGatewayClassSpec("gateway.k8s.aws/nlb")

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

	if lbConfSpec.ListenerConfigurations != nil {
		for _, lsr := range *lbConfSpec.ListenerConfigurations {
			if lsr.ProtocolPort == "TLS:443" {
				tlsMode := gwv1.TLSModeTerminate
				listeners = append(listeners, gwv1.Listener{
					Name:     "port443",
					Port:     443,
					Protocol: gwv1.TLSProtocolType,
					TLS: &gwv1.GatewayTLSConfig{
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

	tcprs := []*gwalpha2.TCPRoute{buildTCPRoute([]gwv1.ParentReference{}, []gwv1.BackendRef{})}
	if auxiliaryStack != nil {
		listeners = append(listeners, gwv1.Listener{
			Name:     "other-ns",
			Port:     5000,
			Protocol: gwv1.TCPProtocolType,
		})

		tcprs = append(tcprs, buildOtherNsRefTcpRoute("other-ns", auxiliaryStack.ns))
	}

	gw := buildBasicGatewaySpec(gwc, listeners)

	lbc := buildLoadBalancerConfig(lbConfSpec)
	tgcTCP := buildTargetGroupConfig(defaultTgConfigName, tgConfSpec, svcTCP)
	tgcUDP := buildTargetGroupConfig(udpDefaultTgConfigName, tgConfSpec, svcUDP)
	udpr := buildUDPRoute("port8080")

	s.nlbResourceStack = newNLBResourceStack([]*appsv1.Deployment{dpTCP, dpUDP}, []*corev1.Service{svcTCP, svcUDP}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgcTCP, tgcUDP}, tcprs, []*gwalpha2.UDPRoute{udpr}, nil, "nlb-gateway-e2e", readinessGateEnabled)

	return s.nlbResourceStack.Deploy(ctx, f)
}

func (s *NLBTestStack) DeployTCPWeightedStack(ctx context.Context, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, readinessGateEnabled bool) error {
	dpTCP1 := buildCustomizableResponseDeploymentSpec("dp1", "yellow", f.Options.TestImageRegistry)
	dpTCP2 := buildCustomizableResponseDeploymentSpec("dp2", "green", f.Options.TestImageRegistry)
	svcTCP1 := buildServiceSpec(dpTCP1.Spec.Selector.MatchLabels)
	svcTCP2 := buildServiceSpec(dpTCP2.Spec.Selector.MatchLabels)
	svcTCP2.Name = svcTCP2.Name + "-2"

	gwc := buildGatewayClassSpec("gateway.k8s.aws/nlb")

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
					TLS: &gwv1.GatewayTLSConfig{
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

	tcprs := []*gwalpha2.TCPRoute{buildTCPRoute([]gwv1.ParentReference{
		{
			Name: defaultName,
		},
	}, []gwv1.BackendRef{
		{
			BackendObjectReference: gwalpha2.BackendObjectReference{
				Name: gwv1.ObjectName(svcTCP1.Name),
				Port: (*gwv1.PortNumber)(&svcTCP1.Spec.Ports[0].Port),
			},
		},
		{
			BackendObjectReference: gwalpha2.BackendObjectReference{
				Name: gwv1.ObjectName(svcTCP2.Name),
				Port: (*gwv1.PortNumber)(&svcTCP2.Spec.Ports[0].Port),
			},
		},
	})}

	gw := buildBasicGatewaySpec(gwc, listeners)

	lbc := buildLoadBalancerConfig(lbConfSpec)
	tgcTCP1 := buildTargetGroupConfig(svcTCP1.Name, tgConfSpec, svcTCP1)
	tgcTCP2 := buildTargetGroupConfig(svcTCP2.Name, tgConfSpec, svcTCP2)

	s.nlbResourceStack = newNLBResourceStack([]*appsv1.Deployment{dpTCP1, dpTCP2}, []*corev1.Service{svcTCP1, svcTCP2}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgcTCP1, tgcTCP2}, tcprs, []*gwalpha2.UDPRoute{}, nil, "nlb-gateway-e2e", readinessGateEnabled)

	return s.nlbResourceStack.Deploy(ctx, f)
}

func (s *NLBTestStack) DeployTCP_UDP(ctx context.Context, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, readinessGateEnabled bool) error {
	dpUDP := buildUDPDeploymentSpec()
	svcUDP := buildUDPServiceSpec()
	gwc := buildGatewayClassSpec("gateway.k8s.aws/nlb")

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

	gw := buildBasicGatewaySpec(gwc, listeners)

	lbc := buildLoadBalancerConfig(lbConfSpec)
	tgcUDP := buildTargetGroupConfig(udpDefaultTgConfigName, tgConfSpec, svcUDP)
	udpr := buildUDPRoute("port80udp")

	s.nlbResourceStack = newNLBResourceStack([]*appsv1.Deployment{dpUDP}, []*corev1.Service{svcUDP}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgcUDP}, tcprs, []*gwalpha2.UDPRoute{udpr}, nil, "nlb-gateway-e2e", readinessGateEnabled)

	return s.nlbResourceStack.Deploy(ctx, f)
}

func (s *NLBTestStack) DeployFrontendNLB(ctx context.Context, albStack ALBTestStack, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, hasTLS bool, readinessGateEnabled bool) error {
	gwc := buildGatewayClassSpec("gateway.k8s.aws/nlb")

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

	tcprs := []*gwalpha2.TCPRoute{buildFENLBTCPRoute(albStack.albResourceStack.commonStack.gw.Name, albStack.albResourceStack.commonStack.gw.Namespace, gwalpha2.PortNumber(80))}

	if hasTLS {
		listeners = append(listeners, gwv1.Listener{
			Name:     "port443",
			Port:     443,
			Protocol: gwv1.TCPProtocolType,
		})
		tcpForHTTPS := buildFENLBTCPRoute(albStack.albResourceStack.commonStack.gw.Name, albStack.albResourceStack.commonStack.gw.Namespace, gwalpha2.PortNumber(443))
		tcprs = append(tcprs, tcpForHTTPS)

	}

	gw := buildBasicGatewaySpec(gwc, listeners)

	lbc := buildLoadBalancerConfig(lbConfSpec)

	s.nlbResourceStack = newNLBResourceStack([]*appsv1.Deployment{}, []*corev1.Service{}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{}, tcprs, []*gwalpha2.UDPRoute{}, nil, "nlb-gateway-e2e", readinessGateEnabled)

	err := s.nlbResourceStack.Deploy(ctx, f)
	if err != nil {
		return err
	}

	// The special TGC is just to support HTTPS and HTTP health check routes on the same underlying gateway.
	if !hasTLS {
		return nil
	}

	// Hack to get TargetGroupConfiguration working correctly, as it needs the namespace which is allocated in the deploy step.

	http := elbv2gw.TargetGroupHealthCheckProtocolHTTP
	https := elbv2gw.TargetGroupHealthCheckProtocolHTTPS

	return createTargetGroupConfigs(ctx, f, []*elbv2gw.TargetGroupConfiguration{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("alb-https-hc-config"),
				Namespace: albStack.albResourceStack.commonStack.gw.Namespace,
			},
			Spec: elbv2gw.TargetGroupConfigurationSpec{
				TargetReference: elbv2gw.Reference{
					Group: nil,
					Kind:  awssdk.String("Gateway"),
					Name:  albStack.albResourceStack.commonStack.gw.Name,
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
					Namespace: gwbeta1.Namespace(s.nlbResourceStack.commonStack.ns.Name),
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

	if err := createReferenceGrants(ctx, f, []*gwbeta1.ReferenceGrant{refGrant}); err != nil {
		return nil, err
	}

	return refGrant, nil
}

func (s *NLBTestStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	return s.nlbResourceStack.Cleanup(ctx, f)
}

func (s *NLBTestStack) GetLoadBalancerIngressHostName() string {
	return s.nlbResourceStack.GetLoadBalancerIngressHostname()
}

func (s *NLBTestStack) GetNamespace() string {
	return s.nlbResourceStack.GetNamespace()
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
	tcpRouteListenerInfo := []listenerValidationInfo{
		{
			listenerName:       "port80",
			parentKind:         "Gateway",
			resolvedRefReason:  "ResolvedRefs",
			resolvedRefsStatus: "True",
			acceptedReason:     "Accepted",
			acceptedStatus:     "True",
		},
	}

	if hasTLS {
		tcpRouteListenerInfo = append(tcpRouteListenerInfo, listenerValidationInfo{
			listenerName:       "port443",
			parentKind:         "Gateway",
			resolvedRefReason:  "ResolvedRefs",
			resolvedRefsStatus: "True",
			acceptedReason:     "Accepted",
			acceptedStatus:     "True",
		})
	} else {
		tcpRouteListenerInfo = append(tcpRouteListenerInfo, listenerValidationInfo{
			listenerName:       "port443",
			parentKind:         "Gateway",
			resolvedRefReason:  "ResolvedRefs",
			resolvedRefsStatus: "True",
			acceptedReason:     "NoMatchingParent",
			acceptedStatus:     "False",
		})
	}

	tcpValidationInfo := map[string]routeValidationInfo{
		k8s.NamespacedName(stack.nlbResourceStack.tcprs[0]).String(): {
			parentGatewayName: stack.nlbResourceStack.commonStack.gw.Name,
			listenerInfo:      tcpRouteListenerInfo,
		},
		k8s.NamespacedName(stack.nlbResourceStack.tcprs[1]).String(): {
			parentGatewayName: stack.nlbResourceStack.commonStack.gw.Name,
			listenerInfo: []listenerValidationInfo{
				{
					listenerName:       "other-ns",
					parentKind:         "Gateway",
					resolvedRefReason:  "RefNotPermitted",
					resolvedRefsStatus: "False",
					acceptedReason:     "Accepted",
					acceptedStatus:     "True",
				},
			},
		},
	}

	udpValidationInfo := map[string]routeValidationInfo{
		k8s.NamespacedName(stack.nlbResourceStack.udprs[0]).String(): {
			parentGatewayName: stack.nlbResourceStack.commonStack.gw.Name,
			listenerInfo: []listenerValidationInfo{
				{
					listenerName:       "port8080",
					parentKind:         "Gateway",
					resolvedRefReason:  "ResolvedRefs",
					resolvedRefsStatus: "True",
					acceptedReason:     "Accepted",
					acceptedStatus:     "True",
				},
			},
		},
	}

	validateRouteStatus(tf, stack.nlbResourceStack.tcprs, tcpRouteStatusConverter, tcpValidationInfo)
	validateRouteStatus(tf, stack.nlbResourceStack.udprs, udpRouteStatusConverter, udpValidationInfo)
}

func validateL4RouteStatusPermitted(tf *framework.Framework, stack NLBTestStack, hasTLS bool) {
	tcpRouteListenerInfo := []listenerValidationInfo{
		{
			listenerName:       "port80",
			parentKind:         "Gateway",
			resolvedRefReason:  "ResolvedRefs",
			resolvedRefsStatus: "True",
			acceptedReason:     "Accepted",
			acceptedStatus:     "True",
		},
	}

	if hasTLS {
		tcpRouteListenerInfo = append(tcpRouteListenerInfo, listenerValidationInfo{
			listenerName:       "port443",
			parentKind:         "Gateway",
			resolvedRefReason:  "ResolvedRefs",
			resolvedRefsStatus: "True",
			acceptedReason:     "Accepted",
			acceptedStatus:     "True",
		})
	} else {
		tcpRouteListenerInfo = append(tcpRouteListenerInfo, listenerValidationInfo{
			listenerName:       "port443",
			parentKind:         "Gateway",
			resolvedRefReason:  "ResolvedRefs",
			resolvedRefsStatus: "True",
			acceptedReason:     "NoMatchingParent",
			acceptedStatus:     "False",
		})
	}

	tcpValidationInfo := map[string]routeValidationInfo{
		k8s.NamespacedName(stack.nlbResourceStack.tcprs[0]).String(): {
			parentGatewayName: stack.nlbResourceStack.commonStack.gw.Name,
			listenerInfo:      tcpRouteListenerInfo,
		},
		k8s.NamespacedName(stack.nlbResourceStack.tcprs[1]).String(): {
			parentGatewayName: stack.nlbResourceStack.commonStack.gw.Name,
			listenerInfo: []listenerValidationInfo{
				{
					listenerName:       "other-ns",
					parentKind:         "Gateway",
					resolvedRefReason:  "ResolvedRefs",
					resolvedRefsStatus: "True",
					acceptedReason:     "Accepted",
					acceptedStatus:     "True",
				},
			},
		},
	}

	udpValidationInfo := map[string]routeValidationInfo{
		k8s.NamespacedName(stack.nlbResourceStack.udprs[0]).String(): {
			parentGatewayName: stack.nlbResourceStack.commonStack.gw.Name,
			listenerInfo: []listenerValidationInfo{
				{
					listenerName:       "port8080",
					parentKind:         "Gateway",
					resolvedRefReason:  "ResolvedRefs",
					resolvedRefsStatus: "True",
					acceptedReason:     "Accepted",
					acceptedStatus:     "True",
				},
			},
		},
	}
	validateRouteStatus(tf, stack.nlbResourceStack.tcprs, tcpRouteStatusConverter, tcpValidationInfo)
	validateRouteStatus(tf, stack.nlbResourceStack.udprs, udpRouteStatusConverter, udpValidationInfo)
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
	bm := &bodyMatcher{
		responseCount: map[string]int{},
	}
	for i := 0; i < 100; i++ {
		_ = tf.HTTPVerifier.VerifyURL(url, bm)
	}
	// We have configured the weighted listener to have two body types.
	// We aren't interested in verifying that the NLB is correctly splitting traffic.
	Expect(len(bm.responseCount)).To(Equal(2))
}

func (s *NLBTestStack) DeployListenerMismatch(ctx context.Context, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, readinessGateEnabled bool) error {
	dpTCP := buildDeploymentSpec(f.Options.TestImageRegistry)
	svcTCP := buildServiceSpec(map[string]string{})
	gwc := buildGatewayClassSpec("gateway.k8s.aws/nlb")

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

	tcprs := []*gwalpha2.TCPRoute{buildTCPRouteWithMismatchedParentRefs()}
	gw := buildBasicGatewaySpec(gwc, listeners)
	lbc := buildLoadBalancerConfig(lbConfSpec)
	tgcTCP := buildTargetGroupConfig(defaultTgConfigName, tgConfSpec, svcTCP)

	s.nlbResourceStack = newNLBResourceStack([]*appsv1.Deployment{dpTCP}, []*corev1.Service{svcTCP}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgcTCP}, tcprs, []*gwalpha2.UDPRoute{}, nil, "nlb-gateway-e2e", readinessGateEnabled)

	return s.nlbResourceStack.Deploy(ctx, f)
}

func validateTCPRouteListenerMismatch(tf *framework.Framework, stack NLBTestStack) {
	validationInfo := map[string]routeValidationInfo{
		k8s.NamespacedName(stack.nlbResourceStack.tcprs[0]).String(): {
			parentGatewayName: stack.nlbResourceStack.commonStack.gw.Name,
			listenerInfo: []listenerValidationInfo{
				{
					listenerName:       "listener-exists",
					parentKind:         "Gateway",
					resolvedRefReason:  "ResolvedRefs",
					resolvedRefsStatus: "True",
					acceptedReason:     "Accepted",
					acceptedStatus:     "True",
				},
				{
					listenerName:       "listener-nonexist",
					parentKind:         "Gateway",
					resolvedRefReason:  "ResolvedRefs",
					resolvedRefsStatus: "True",
					acceptedReason:     "NoMatchingParent",
					acceptedStatus:     "False",
				},
			},
		},
	}
	validateRouteStatus(tf, stack.nlbResourceStack.tcprs, tcpRouteStatusConverter, validationInfo)

	validateGatewayStatus(tf, stack.nlbResourceStack.commonStack.gw, gatewayValidationInfo{
		conditions: []gatewayConditionValidation{
			{
				conditionType:   gwv1.GatewayConditionProgrammed,
				conditionStatus: "True",
				conditionReason: "Programmed",
			},
			{
				conditionType:   gwv1.GatewayConditionAccepted,
				conditionStatus: "True",
				conditionReason: "Accepted",
			},
		},
		listeners: []gatewayListenerValidation{
			{
				listenerName:   "listener-exists",
				attachedRoutes: 1,
				conditions: []listenerConditionValidation{
					{
						conditionType:   gwv1.ListenerConditionAccepted,
						conditionStatus: "True",
						conditionReason: "Accepted",
					},
					{
						conditionType:   gwv1.ListenerConditionProgrammed,
						conditionStatus: "True",
						conditionReason: "Programmed",
					},
				},
			},
			{
				listenerName:   "listener-other",
				attachedRoutes: 0,
				conditions: []listenerConditionValidation{
					{
						conditionType:   gwv1.ListenerConditionAccepted,
						conditionStatus: "True",
						conditionReason: "Accepted",
					},
					{
						conditionType:   gwv1.ListenerConditionProgrammed,
						conditionStatus: "True",
						conditionReason: "Programmed",
					},
				},
			},
		},
	})
}
