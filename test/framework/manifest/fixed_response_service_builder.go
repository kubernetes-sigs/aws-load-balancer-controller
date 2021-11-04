package manifest

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NewFixedResponseServiceBuilder constructs a builder that capable to build manifest for an HTTP service with fixed response.
func NewFixedResponseServiceBuilder() *fixedResponseServiceBuilder {
	return &fixedResponseServiceBuilder{
		replicas:       1,
		httpBody:       "Hello World!",
		port:           80,
		targetPort:     8080,
		svcType:        corev1.ServiceTypeNodePort,
		svcAnnotations: nil,
	}
}

type fixedResponseServiceBuilder struct {
	replicas       int32
	httpBody       string
	port           int32
	targetPort     int32
	targetPortName string
	svcType        corev1.ServiceType
	svcAnnotations map[string]string
}

func (b *fixedResponseServiceBuilder) WithReplicas(replicas int32) *fixedResponseServiceBuilder {
	b.replicas = replicas
	return b
}

func (b *fixedResponseServiceBuilder) WithHTTPBody(httpBody string) *fixedResponseServiceBuilder {
	b.httpBody = httpBody
	return b
}

func (b *fixedResponseServiceBuilder) WithPort(port int32) *fixedResponseServiceBuilder {
	b.port = port
	return b
}

func (b *fixedResponseServiceBuilder) WithTargetPort(targetPort int32) *fixedResponseServiceBuilder {
	b.targetPort = targetPort
	return b
}

func (b *fixedResponseServiceBuilder) WithTargetPortName(targetPortName string) *fixedResponseServiceBuilder {
	b.targetPortName = targetPortName
	return b
}

func (b *fixedResponseServiceBuilder) WithServiceType(svcType corev1.ServiceType) *fixedResponseServiceBuilder {
	b.svcType = svcType
	return b
}

func (b *fixedResponseServiceBuilder) WithServiceAnnotations(svcAnnotations map[string]string) *fixedResponseServiceBuilder {
	b.svcAnnotations = svcAnnotations
	return b
}

func (b *fixedResponseServiceBuilder) Build(namespace string, name string) (*appsv1.Deployment, *corev1.Service) {
	dp := b.buildDeployment(namespace, name)
	svc := b.buildService(namespace, name)
	return dp, svc
}

// TODO: have a deployment builder that been called by this component :D.
func (b *fixedResponseServiceBuilder) buildDeployment(namespace string, name string) *appsv1.Deployment {
	podLabels := b.buildPodLabels(name)
	dp := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: podLabels,
			},
			Replicas: aws.Int32(b.replicas),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "970805265562.dkr.ecr.us-west-2.amazonaws.com/colorteller:latest",
							Ports: []corev1.ContainerPort{
								{
									Name:          b.targetPortName,
									ContainerPort: b.targetPort,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "SERVER_PORT",
									Value: fmt.Sprintf("%d", b.targetPort),
								},
								{
									Name:  "COLOR",
									Value: b.httpBody,
								},
							},
						},
					},
				},
			},
		},
	}
	return dp
}

func (b *fixedResponseServiceBuilder) buildService(namespace string, name string) *corev1.Service {
	podLabels := b.buildPodLabels(name)
	targetPort := intstr.FromInt(int(b.targetPort))
	if len(b.targetPortName) > 0 {
		targetPort = intstr.FromString(b.targetPortName)
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Annotations: b.svcAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     b.svcType,
			Selector: podLabels,
			Ports: []corev1.ServicePort{
				{
					Port:       b.port,
					TargetPort: targetPort,
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	return svc
}

func (b *fixedResponseServiceBuilder) buildPodLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": name,
	}
}
