package gateway

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ALBTestStack struct {
	albResourceStack *albResourceStack
}

func (s *ALBTestStack) Deploy(ctx context.Context, auxiliaryStack *auxiliaryResourceStack, f *framework.Framework, gwListeners []gwv1.Listener, httprs []*gwv1.HTTPRoute, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec, readinessGateEnabled bool) error {
	if auxiliaryStack != nil {
		gwListeners = append(gwListeners, gwv1.Listener{
			Name:     "other-ns",
			Port:     5000,
			Protocol: gwv1.HTTPProtocolType,
		})

		httprs = append(httprs, buildOtherNsRefHttpRoute("other-ns", auxiliaryStack.ns))
	}

	dp := buildDeploymentSpec(f.Options.TestImageRegistry)
	svc := buildServiceSpec()
	gwc := buildGatewayClassSpec("gateway.k8s.aws/alb")
	gw := buildBasicGatewaySpec(gwc, gwListeners)
	lbc := buildLoadBalancerConfig(lbConfSpec)
	tgc := buildTargetGroupConfig(defaultTgConfigName, tgConfSpec, svc)

	s.albResourceStack = newALBResourceStack(dp, svc, gwc, gw, lbc, tgc, httprs, "alb-gateway-e2e", readinessGateEnabled)

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
