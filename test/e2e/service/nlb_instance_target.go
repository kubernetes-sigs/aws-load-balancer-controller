package service

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
)

const (
	defaultTestImage   = "kishorj/hello-multi:v1"
	appContainerPort   = 80
	defaultNumReplicas = 3
	defaultName        = "instance-e2e"
)

type NLBInstanceTestStack struct {
	resourceStack *resourceStack
}

func (s *NLBInstanceTestStack) Deploy(ctx context.Context, f *framework.Framework, svcAnnotations map[string]string) error {
	dp := s.buildDeploymentSpec()
	svc := s.buildServiceSpec(ctx, svcAnnotations)
	s.resourceStack = NewResourceStack(dp, svc, "service-instance-e2e", false)

	return s.resourceStack.Deploy(ctx, f)
}

func (s *NLBInstanceTestStack) UpdateServiceAnnotation(ctx context.Context, f *framework.Framework, svcAnnotations map[string]string) error {
	return s.resourceStack.UpdateServiceAnnotations(ctx, f, svcAnnotations)
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

func (s *NLBInstanceTestStack) buildDeploymentSpec() *appsv1.Deployment {
	numReplicas := int32(defaultNumReplicas)
	labels := map[string]string{
		"app.kubernetes.io/name":     "multi-port",
		"app.kubernetes.io/instance": defaultName,
	}
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
							Image:           defaultTestImage,
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
