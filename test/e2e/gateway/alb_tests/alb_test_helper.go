package alb_tests

import (
	"context"
	"crypto/tls"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/gateway/test_resources"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/gateway/test_resources/grpc/echo"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ALBTestStack struct {
	Resources *ALBResourceStack
}

func (s *ALBTestStack) DeployHTTP(ctx context.Context, auxiliaryStack *test_resources.AuxiliaryResourceStack, f *framework.Framework, gwListeners []gwv1.Listener, httprs []*gwv1.HTTPRoute, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, lrConfSpec elbv2gw.ListenerRuleConfigurationSpec, secret *testOIDCSecret, readinessGateEnabled bool) error {
	if auxiliaryStack != nil {
		gwListeners = append(gwListeners, gwv1.Listener{
			Name:     "other-ns",
			Port:     5000,
			Protocol: gwv1.HTTPProtocolType,
		})

		httprs = append(httprs, test_resources.BuildOtherNsRefHttpRoute("other-ns", auxiliaryStack.Ns))
	}

	if f.Options.IPFamily == framework.IPv6 {
		v6 := elbv2gw.LoadBalancerIpAddressTypeDualstack
		lbConfSpec.IpAddressType = &v6
	}

	svc := test_resources.BuildServiceSpec(map[string]string{})
	tgc := test_resources.BuildTargetGroupConfig(test_resources.DefaultTgConfigName, tgConfSpec, svc)
	return s.deploy(ctx, f, gwListeners, httprs, []*gwv1.GRPCRoute{}, []*appsv1.Deployment{test_resources.BuildDeploymentSpec(f.Options.TestImageRegistry)}, []*corev1.Service{svc}, lbConfSpec, []*elbv2gw.TargetGroupConfiguration{tgc}, lrConfSpec, secret, readinessGateEnabled)
}

func (s *ALBTestStack) DeployGRPC(ctx context.Context, f *framework.Framework, gwListeners []gwv1.Listener, grpcrs []*gwv1.GRPCRoute, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, lrConfSpec elbv2gw.ListenerRuleConfigurationSpec, readinessGateEnabled bool) error {
	labels := map[string]string{
		"app.kubernetes.io/instance": test_resources.GRPCDefaultName,
	}

	otherLabels := map[string]string{
		"app.kubernetes.io/instance": "other",
	}

	svc := test_resources.BuildGRPCServiceSpec(test_resources.GRPCDefaultName, labels)
	dp := test_resources.BuildGRPCDeploymentSpec(test_resources.GRPCDefaultName, "Hello World", labels)
	tgc := test_resources.BuildTargetGroupConfig(test_resources.DefaultTgConfigName, tgConfSpec, svc)

	svcOther := test_resources.BuildGRPCServiceSpec(test_resources.GRPCDefaultName+"-other", otherLabels)
	dpOther := test_resources.BuildGRPCDeploymentSpec(test_resources.GRPCDefaultName+"-other", "Hello World - Other", otherLabels)
	tgcOther := test_resources.BuildTargetGroupConfig(test_resources.DefaultTgConfigName+"-other", tgConfSpec, svcOther)

	return s.deploy(ctx, f, gwListeners, []*gwv1.HTTPRoute{}, grpcrs, []*appsv1.Deployment{dp, dpOther}, []*corev1.Service{svc, svcOther}, lbConfSpec, []*elbv2gw.TargetGroupConfiguration{tgc, tgcOther}, lrConfSpec, nil, readinessGateEnabled)
}

// DeployHTTPWithDefaultTGC deploys an ALB stack with a gateway-level default TGC referenced by the LBC,
// plus two services: svc1 inherits defaults, svc2 has a service-level TGC override.
func (s *ALBTestStack) DeployHTTPWithDefaultTGC(ctx context.Context, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, defaultTGC *elbv2gw.TargetGroupConfiguration, svcTgSpec elbv2gw.TargetGroupConfigurationSpec, readinessGateEnabled bool) error {
	if f.Options.IPFamily == framework.IPv6 {
		v6 := elbv2gw.LoadBalancerIpAddressTypeDualstack
		lbConfSpec.IpAddressType = &v6
	}

	gwListeners := []gwv1.Listener{
		{
			Name:     "http80",
			Port:     80,
			Protocol: gwv1.HTTPProtocolType,
		},
	}

	svc1 := test_resources.BuildServiceSpec(map[string]string{})
	svc2 := test_resources.BuildServiceSpec(map[string]string{})
	svc2.Name = "echoserver-v2"
	svcTgc := test_resources.BuildTargetGroupConfig("svc2-tgc", svcTgSpec, svc2)

	port := gwv1.PortNumber(80)
	httpr := test_resources.BuildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{
		{
			BackendRefs: []gwv1.HTTPBackendRef{
				{
					BackendRef: gwv1.BackendRef{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name: gwv1.ObjectName(svc1.Name),
							Port: &port,
						},
					},
				},
				{
					BackendRef: gwv1.BackendRef{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name: gwv1.ObjectName(svc2.Name),
							Port: &port,
						},
					},
				},
			},
		},
	}, nil)
	lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{}

	return s.deploy(ctx, f, gwListeners, []*gwv1.HTTPRoute{httpr}, []*gwv1.GRPCRoute{},
		[]*appsv1.Deployment{test_resources.BuildDeploymentSpec(f.Options.TestImageRegistry)},
		[]*corev1.Service{svc1, svc2},
		lbConfSpec,
		[]*elbv2gw.TargetGroupConfiguration{defaultTGC, svcTgc},
		lrcSpec, nil, readinessGateEnabled)
}

func (s *ALBTestStack) deploy(ctx context.Context, f *framework.Framework, gwListeners []gwv1.Listener, httprs []*gwv1.HTTPRoute, grpcrs []*gwv1.GRPCRoute, dps []*appsv1.Deployment, svcs []*corev1.Service, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgcs []*elbv2gw.TargetGroupConfiguration, lrConfSpec elbv2gw.ListenerRuleConfigurationSpec, secret *testOIDCSecret, readinessGateEnabled bool) error {
	gwc := test_resources.BuildGatewayClassSpec("test_resources.k8s.aws/alb")
	gw := test_resources.BuildBasicGatewaySpec(gwc, gwListeners)
	lbc := test_resources.BuildLoadBalancerConfig(lbConfSpec)
	lrc := test_resources.BuildListenerRuleConfig(test_resources.DefaultLRConfigName, lrConfSpec)

	namespaceLabels := test_resources.GetNamespaceLabels(readinessGateEnabled)

	s.Resources = newALBResourceStack(dps, svcs, gwc, gw, lbc, tgcs, lrc, httprs, grpcrs, secret, "alb-gateway-e2e", namespaceLabels)

	return s.Resources.Deploy(ctx, f)
}

func (s *ALBTestStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	if s.Resources == nil {
		return nil
	}
	return s.Resources.Cleanup(ctx, f)
}

func (s *ALBTestStack) GetLoadBalancerIngressHostName() string {
	return s.Resources.GetLoadBalancerIngressHostname()
}

func (s *ALBTestStack) GetNamespace() string {
	return s.Resources.GetNamespace()
}

func (s *ALBTestStack) GetWorkerNodes(ctx context.Context, f *framework.Framework) ([]corev1.Node, error) {
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

func generateGRPCClient(dnsName string) (echo.EchoServiceClient, error) {
	target := fmt.Sprintf("%s:443", dnsName)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // This skips all certificate verification, including expiry.
	}

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		return nil, err
	}
	return echo.NewEchoServiceClient(conn), nil
}

func validateHTTPRouteStatusNotPermitted(tf *framework.Framework, stack ALBTestStack) {
	validationInfo := map[string]test_resources.RouteValidationInfo{
		k8s.NamespacedName(stack.Resources.Httprs[0]).String(): {
			ParentGatewayName: stack.Resources.CommonStack.Gw.Name,
			ListenerInfo: []test_resources.ListenerValidationInfo{
				{
					ListenerName:       "test-listener",
					ParentKind:         "Gateway",
					ResolvedRefReason:  "ResolvedRefs",
					ResolvedRefsStatus: "True",
					AcceptedReason:     "Accepted",
					AcceptedStatus:     "True",
				},
			},
		},
		k8s.NamespacedName(stack.Resources.Httprs[1]).String(): {
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
	test_resources.ValidateRouteStatus(tf, stack.Resources.Httprs, httpRouteStatusConverter, validationInfo)
}

func validateHTTPRouteStatusPermitted(tf *framework.Framework, stack ALBTestStack) {
	validationInfo := map[string]test_resources.RouteValidationInfo{
		k8s.NamespacedName(stack.Resources.Httprs[0]).String(): {
			ParentGatewayName: stack.Resources.CommonStack.Gw.Name,
			ListenerInfo: []test_resources.ListenerValidationInfo{
				{
					ListenerName:       "test-listener",
					ParentKind:         "Gateway",
					ResolvedRefReason:  "ResolvedRefs",
					ResolvedRefsStatus: "True",
					AcceptedReason:     "Accepted",
					AcceptedStatus:     "True",
				},
			},
		},
		k8s.NamespacedName(stack.Resources.Httprs[1]).String(): {
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
	test_resources.ValidateRouteStatus(tf, stack.Resources.Httprs, httpRouteStatusConverter, validationInfo)
}

func validateGRPCRouteStatus(tf *framework.Framework, stack ALBTestStack) {
	validationInfo := map[string]test_resources.RouteValidationInfo{
		k8s.NamespacedName(stack.Resources.Grpcrs[0]).String(): {
			ParentGatewayName: stack.Resources.CommonStack.Gw.Name,
			ListenerInfo: []test_resources.ListenerValidationInfo{
				{
					ListenerName:       "test-listener",
					ParentKind:         "Gateway",
					ResolvedRefReason:  "ResolvedRefs",
					ResolvedRefsStatus: "True",
					AcceptedReason:     "Accepted",
					AcceptedStatus:     "True",
				},
			},
		},
	}
	test_resources.ValidateRouteStatus(tf, stack.Resources.Grpcrs, grpcRouteStatusConverter, validationInfo)
}

func httpRouteStatusConverter(tf *framework.Framework, i interface{}) (gwv1.RouteStatus, types.NamespacedName, error) {
	httpR := i.(*gwv1.HTTPRoute)
	retrievedRoute := gwv1.HTTPRoute{}
	err := tf.K8sClient.Get(context.Background(), k8s.NamespacedName(httpR), &retrievedRoute)
	if err != nil {
		return gwv1.RouteStatus{}, types.NamespacedName{}, err
	}
	return retrievedRoute.Status.RouteStatus, k8s.NamespacedName(&retrievedRoute), nil
}

func grpcRouteStatusConverter(tf *framework.Framework, i interface{}) (gwv1.RouteStatus, types.NamespacedName, error) {
	grpcR := i.(*gwv1.GRPCRoute)
	retrievedRoute := gwv1.GRPCRoute{}
	err := tf.K8sClient.Get(context.Background(), k8s.NamespacedName(grpcR), &retrievedRoute)
	if err != nil {
		return gwv1.RouteStatus{}, types.NamespacedName{}, err
	}
	return retrievedRoute.Status.RouteStatus, k8s.NamespacedName(&retrievedRoute), nil
}

func validateHTTPRouteHostnameMismatchRouteAndGatewayStatus(tf *framework.Framework, stack ALBTestStack) {
	validationInfo := map[string]test_resources.RouteValidationInfo{
		k8s.NamespacedName(stack.Resources.Httprs[0]).String(): {
			ParentGatewayName: stack.Resources.CommonStack.Gw.Name,
			ListenerInfo: []test_resources.ListenerValidationInfo{
				{
					ListenerName:       "listener-no-hostname",
					ParentKind:         "Gateway",
					ResolvedRefReason:  "ResolvedRefs",
					ResolvedRefsStatus: "True",
					AcceptedReason:     "Accepted",
					AcceptedStatus:     "True",
				},
			},
		},
	}
	test_resources.ValidateRouteStatus(tf, stack.Resources.Httprs, httpRouteStatusConverter, validationInfo)

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
				ListenerName:   "listener-no-hostname",
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
				ListenerName:   "listener-with-hostname",
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
