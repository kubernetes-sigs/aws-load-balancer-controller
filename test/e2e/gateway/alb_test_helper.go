package gateway

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
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/gateway/grpc/echo"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ALBTestStack struct {
	albResourceStack *albResourceStack
}

func (s *ALBTestStack) DeployHTTP(ctx context.Context, auxiliaryStack *auxiliaryResourceStack, f *framework.Framework, gwListeners []gwv1.Listener, httprs []*gwv1.HTTPRoute, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, lrConfSpec elbv2gw.ListenerRuleConfigurationSpec, secret *testOIDCSecret, readinessGateEnabled bool) error {
	if auxiliaryStack != nil {
		gwListeners = append(gwListeners, gwv1.Listener{
			Name:     "other-ns",
			Port:     5000,
			Protocol: gwv1.HTTPProtocolType,
		})

		httprs = append(httprs, buildOtherNsRefHttpRoute("other-ns", auxiliaryStack.ns))
	}

	if f.Options.IPFamily == framework.IPv6 {
		v6 := elbv2gw.LoadBalancerIpAddressTypeDualstack
		lbConfSpec.IpAddressType = &v6
	}

	svc := buildServiceSpec()
	tgc := buildTargetGroupConfig(defaultTgConfigName, tgConfSpec, svc)
	return s.deploy(ctx, f, gwListeners, httprs, []*gwv1.GRPCRoute{}, []*appsv1.Deployment{buildDeploymentSpec(f.Options.TestImageRegistry)}, []*corev1.Service{svc}, lbConfSpec, []*elbv2gw.TargetGroupConfiguration{tgc}, lrConfSpec, secret, readinessGateEnabled)
}

func (s *ALBTestStack) DeployGRPC(ctx context.Context, f *framework.Framework, gwListeners []gwv1.Listener, grpcrs []*gwv1.GRPCRoute, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, lrConfSpec elbv2gw.ListenerRuleConfigurationSpec, readinessGateEnabled bool) error {
	labels := map[string]string{
		"app.kubernetes.io/instance": grpcDefaultName,
	}

	otherLabels := map[string]string{
		"app.kubernetes.io/instance": "other",
	}

	svc := buildGRPCServiceSpec(grpcDefaultName, labels)
	dp := buildGRPCDeploymentSpec(grpcDefaultName, "Hello World", labels)
	tgc := buildTargetGroupConfig(defaultTgConfigName, tgConfSpec, svc)

	svcOther := buildGRPCServiceSpec(grpcDefaultName+"-other", otherLabels)
	dpOther := buildGRPCDeploymentSpec(grpcDefaultName+"-other", "Hello World - Other", otherLabels)
	tgcOther := buildTargetGroupConfig(defaultTgConfigName+"-other", tgConfSpec, svcOther)

	return s.deploy(ctx, f, gwListeners, []*gwv1.HTTPRoute{}, grpcrs, []*appsv1.Deployment{dp, dpOther}, []*corev1.Service{svc, svcOther}, lbConfSpec, []*elbv2gw.TargetGroupConfiguration{tgc, tgcOther}, lrConfSpec, nil, readinessGateEnabled)
}

func (s *ALBTestStack) deploy(ctx context.Context, f *framework.Framework, gwListeners []gwv1.Listener, httprs []*gwv1.HTTPRoute, grpcrs []*gwv1.GRPCRoute, dps []*appsv1.Deployment, svcs []*corev1.Service, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgcs []*elbv2gw.TargetGroupConfiguration, lrConfSpec elbv2gw.ListenerRuleConfigurationSpec, secret *testOIDCSecret, readinessGateEnabled bool) error {
	gwc := buildGatewayClassSpec("gateway.k8s.aws/alb")
	gw := buildBasicGatewaySpec(gwc, gwListeners)
	lbc := buildLoadBalancerConfig(lbConfSpec)
	lrc := buildListenerRuleConfig(defaultLRConfigName, lrConfSpec)

	s.albResourceStack = newALBResourceStack(dps, svcs, gwc, gw, lbc, tgcs, lrc, httprs, grpcrs, secret, "alb-gateway-e2e", readinessGateEnabled)

	return s.albResourceStack.Deploy(ctx, f)
}

func (s *ALBTestStack) Cleanup(ctx context.Context, f *framework.Framework) {
	s.albResourceStack.Cleanup(ctx, f)
}

func (s *ALBTestStack) GetLoadBalancerIngressHostName() string {
	return s.albResourceStack.GetLoadBalancerIngressHostname()
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
