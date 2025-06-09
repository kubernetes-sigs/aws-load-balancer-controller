package gateway

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type NLBTestStack struct {
	nlbResourceStack *nlbResourceStack
}

func (s *NLBTestStack) Deploy(ctx context.Context, f *framework.Framework, lbConfSpec elbv2gw.LoadBalancerConfigurationSpec, tgConfSpec elbv2gw.TargetGroupConfigurationSpec) error {
	dp := buildDeploymentSpec(f.Options.TestImageRegistry)
	svc := buildServiceSpec()
	gwc := buildGatewayClassSpec("gateway.k8s.aws/nlb")
	gw := buildBasicGatewaySpec(gwc, gwv1.TCPProtocolType)
	lbc := buildLoadBalancerConfig(lbConfSpec)
	tgc := buildTargetGroupConfig(tgConfSpec, svc)
	tcpr := buildTCPRoute()
	s.nlbResourceStack = newNLBResourceStack(dp, svc, gwc, gw, lbc, tgc, tcpr, "nlb-gateway-e2e", false)

	return s.nlbResourceStack.Deploy(ctx, f)
}

func (s *NLBTestStack) ScaleDeployment(ctx context.Context, f *framework.Framework, numReplicas int32) error {
	return s.nlbResourceStack.ScaleDeployment(ctx, f, numReplicas)
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
