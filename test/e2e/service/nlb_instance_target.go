package service

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	appContainerPort   = 80
	defaultNumReplicas = 3
	defaultName        = "instance-e2e"
)

type NLBInstanceTestStack struct {
	resourceStack *resourceStack
}

func (s *NLBInstanceTestStack) Deploy(ctx context.Context, f *framework.Framework, svcAnnotations map[string]string) error {
	dp := s.buildDeploymentSpec(f.Options.TestImageRegistry)
	svc := s.buildServiceSpec(ctx, svcAnnotations)
	s.resourceStack = NewResourceStack(dp, svc, "service-instance-e2e", false)

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

func (s *NLBInstanceTestStack) buildServiceSpec(ctx context.Context, annotations map[string]string) *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/name":     "multi-port",
		"app.kubernetes.io/instance": defaultName,
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
			Annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
				"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
				"service.beta.kubernetes.io/aws-load-balancer-scheme":          "internet-facing",
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
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
	// Override annotations based on the argument
	for key, value := range annotations {
		svc.Annotations[key] = value
	}

	return svc
}
