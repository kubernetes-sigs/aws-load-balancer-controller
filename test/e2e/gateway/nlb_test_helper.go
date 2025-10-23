package gateway

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type NLBTestStack struct {
	nlbResourceStack *nlbResourceStack
}

func (s *NLBTestStack) Deploy(ctx context.Context, f *framework.Framework, auxiliaryStack *auxiliaryResourceStack, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, readinessGateEnabled bool) error {
	dpTCP := buildDeploymentSpec(f.Options.TestImageRegistry)
	svcTCP := buildServiceSpec()

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
				listeners = append(listeners, gwv1.Listener{
					Name:     "port443",
					Port:     443,
					Protocol: gwv1.TLSProtocolType,
				})
				break
			}
		}
	}

	tcprs := []*gwalpha2.TCPRoute{buildTCPRoute()}
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
	udpr := buildUDPRoute()

	s.nlbResourceStack = newNLBResourceStack([]*appsv1.Deployment{dpTCP, dpUDP}, []*corev1.Service{svcTCP, svcUDP}, gwc, gw, lbc, []*elbv2gw.TargetGroupConfiguration{tgcTCP, tgcUDP}, tcprs, []*gwalpha2.UDPRoute{udpr}, nil, "nlb-gateway-e2e", readinessGateEnabled)

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
					acceptedReason:     "RefNotPermitted",
					acceptedStatus:     "False",
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
