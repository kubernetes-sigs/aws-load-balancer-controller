package gateway

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type NLBInstanceTestStack struct {
	nlbResourceStack *nlbResourceStack
}

func (s *NLBInstanceTestStack) Deploy(ctx context.Context, f *framework.Framework, spec elbv2gw.LoadBalancerConfigurationSpec) error {
	dp := buildDeploymentSpec(f.Options.TestImageRegistry)
	svc := buildServiceSpec()
	gwc := buildGatewayClassSpec("gateway.k8s.aws/nlb")
	gw := buildBasicGatewaySpec(gwc, gwv1.TCPProtocolType)
	lbc := buildLoadBalancerConfig(spec)
	tcpr := buildTCPRoute()
	s.nlbResourceStack = newNLBResourceStack(dp, svc, gwc, gw, lbc, tcpr, "service-instance-e2e", false)

	return s.nlbResourceStack.Deploy(ctx, f)
}

func (s *NLBInstanceTestStack) ScaleDeployment(ctx context.Context, f *framework.Framework, numReplicas int32) error {
	return s.nlbResourceStack.ScaleDeployment(ctx, f, numReplicas)
}

func (s *NLBInstanceTestStack) Cleanup(ctx context.Context, f *framework.Framework) {
	s.nlbResourceStack.Cleanup(ctx, f)
}

func (s *NLBInstanceTestStack) GetLoadBalancerIngressHostName() string {
	return s.nlbResourceStack.GetLoadBalancerIngressHostname()
}

func (s *NLBInstanceTestStack) GetWorkerNodes(ctx context.Context, f *framework.Framework) ([]corev1.Node, error) {
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

func (s *NLBInstanceTestStack) ApplyNodeLabels(ctx context.Context, f *framework.Framework, node *corev1.Node, labels map[string]string) error {
	f.Logger.Info("applying node labels", "node", k8s.NamespacedName(node))
	oldNode := node.DeepCopy()
	for key, value := range labels {
		node.Labels[key] = value
	}
	if err := f.K8sClient.Patch(ctx, node, client.MergeFrom(oldNode)); err != nil {
		f.Logger.Info("failed to update node", "node", k8s.NamespacedName(node))
		return err
	}
	return nil
}
