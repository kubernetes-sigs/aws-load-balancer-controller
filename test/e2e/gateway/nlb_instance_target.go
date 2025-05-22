package gateway

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	appContainerPort        = 80
	defaultNumReplicas      = 3
	defaultName             = "instance-e2e"
	defaultGatewayClassName = "gwclass-e2e"
	defaultLbConfigName     = "lbconfig-e2e"
)

type NLBInstanceTestStack struct {
	resourceStack *resourceStack
}

func (s *NLBInstanceTestStack) Deploy(ctx context.Context, f *framework.Framework, spec elbv2gw.LoadBalancerConfigurationSpec) error {
	dp := s.buildDeploymentSpec(f.Options.TestImageRegistry)
	svc := s.buildServiceSpec()
	gwc := s.buildGatewayClassSpec()
	gw := s.buildGatewaySpec()
	lbc := s.buildLoadBalancerConfig(spec)
	tcpr := s.buildTCPRoute()
	s.resourceStack = NewResourceStack(dp, svc, gwc, gw, lbc, tcpr, "service-instance-e2e", false)

	return s.resourceStack.Deploy(ctx, f)
}

func (s *NLBInstanceTestStack) UpdateServiceAnnotations(ctx context.Context, f *framework.Framework, svcAnnotations map[string]string) error {
	return s.resourceStack.UpdateServiceAnnotations(ctx, f, svcAnnotations)
}

func (s *NLBInstanceTestStack) DeleteServiceAnnotations(ctx context.Context, f *framework.Framework, annotationKeys []string) error {
	return s.resourceStack.DeleteServiceAnnotations(ctx, f, annotationKeys)
}

func (s *NLBInstanceTestStack) UpdateServiceTrafficPolicy(ctx context.Context, f *framework.Framework, trafficPolicy corev1.ServiceExternalTrafficPolicyType) error {
	return s.resourceStack.UpdateServiceTrafficPolicy(ctx, f, trafficPolicy)
}

func (s *NLBInstanceTestStack) ScaleDeployment(ctx context.Context, f *framework.Framework, numReplicas int32) error {
	return s.resourceStack.ScaleDeployment(ctx, f, numReplicas)
}

func (s *NLBInstanceTestStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	return s.resourceStack.Cleanup(ctx, f)
}

func (s *NLBInstanceTestStack) GetLoadBalancerIngressHostName() string {
	return s.resourceStack.GetLoadBalancerIngressHostname()
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

func (s *NLBInstanceTestStack) buildDeploymentSpec(testImageRegistry string) *appsv1.Deployment {
	numReplicas := int32(defaultNumReplicas)
	labels := map[string]string{
		"app.kubernetes.io/name":     "multi-port",
		"app.kubernetes.io/instance": defaultName,
	}
	dpImage := utils.GetDeploymentImage(testImageRegistry, utils.HelloImage)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "app",
							ImagePullPolicy: corev1.PullAlways,
							Image:           dpImage,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: appContainerPort,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (s *NLBInstanceTestStack) buildServiceSpec() *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/name":     "multi-port",
		"app.kubernetes.io/instance": defaultName,
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	return svc
}

func (s *NLBInstanceTestStack) buildGatewayClassSpec() *gwv1.GatewayClass {
	gwc := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultGatewayClassName,
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: "gateway.k8s.aws/nlb",
		},
	}
	return gwc
}

func (s *NLBInstanceTestStack) buildLoadBalancerConfig(spec elbv2gw.LoadBalancerConfigurationSpec) *elbv2gw.LoadBalancerConfiguration {
	lbc := &elbv2gw.LoadBalancerConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultLbConfigName,
		},
		Spec: spec,
	}
	return lbc
}

func (s *NLBInstanceTestStack) buildGatewaySpec() *gwv1.Gateway {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: defaultGatewayClassName,
			Listeners: []gwv1.Listener{
				{
					Port:     80,
					Protocol: gwv1.TCPProtocolType,
				},
			},
			Infrastructure: &gwv1.GatewayInfrastructure{
				ParametersRef: &gwv1.LocalParametersReference{
					Group: "gateway.k8s.aws",
					Kind:  "LoadBalancerConfiguration",
					Name:  defaultLbConfigName,
				},
			},
		},
	}
	return gw
}

func (s *NLBInstanceTestStack) buildTCPRoute() *gwalpha2.TCPRoute {
	gw := &gwalpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
		Spec: gwalpha2.TCPRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: defaultName,
					},
				},
			},
		},
	}
	return gw
}
