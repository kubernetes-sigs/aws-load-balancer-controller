package globalaccelerator

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	agav1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

// createAGA creates a GlobalAccelerator resource with the specified configuration
func createAGA(name, namespace, acceleratorName string, ipAddressType agav1beta1.IPAddressType, listeners *[]agav1beta1.GlobalAcceleratorListener) *agav1beta1.GlobalAccelerator {
	return &agav1beta1.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: agav1beta1.GlobalAcceleratorSpec{
			Name:          &acceleratorName,
			IPAddressType: ipAddressType,
			Listeners:     listeners,
		},
	}
}

// createAGAWithIngressEndpoint creates a GlobalAccelerator with an Ingress endpoint
// Pass nil for listeners to use auto-discovery (omits Protocol, PortRanges, ClientAffinity, TrafficDialPercentage)
func createAGAWithIngressEndpoint(name, namespace, acceleratorName, endpointName string, ipAddressType agav1beta1.IPAddressType, listeners *[]agav1beta1.GlobalAcceleratorListener) *agav1beta1.GlobalAccelerator {
	if listeners == nil {
		listeners = &[]agav1beta1.GlobalAcceleratorListener{
			{
				EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{
					{
						Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{
							{
								Type: agav1beta1.GlobalAcceleratorEndpointTypeIngress,
								Name: awssdk.String(endpointName),
							},
						},
					},
				},
			},
		}
	}
	return createAGA(name, namespace, acceleratorName, ipAddressType, listeners)
}

// createAGAWithServiceEndpoint creates a GlobalAccelerator with a Service endpoint
// Pass nil for listeners to use auto-discovery (omits Protocol, PortRanges, ClientAffinity, TrafficDialPercentage)
func createAGAWithServiceEndpoint(name, namespace, acceleratorName, endpointName string, ipAddressType agav1beta1.IPAddressType, listeners *[]agav1beta1.GlobalAcceleratorListener) *agav1beta1.GlobalAccelerator {
	if listeners == nil {
		ep := agav1beta1.GlobalAcceleratorEndpoint{
			Type: agav1beta1.GlobalAcceleratorEndpointTypeService,
			Name: awssdk.String(endpointName),
		}
		if tf.Options.IPFamily == framework.IPv6 {
			ep.ClientIPPreservationEnabled = awssdk.Bool(false)
		}
		listeners = &[]agav1beta1.GlobalAcceleratorListener{
			{
				EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{
					{
						Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{ep},
					},
				},
			},
		}
	}
	return createAGA(name, namespace, acceleratorName, ipAddressType, listeners)
}

// createAGAWithGatewayEndpoint creates a GlobalAccelerator with a Gateway endpoint
// Pass nil for listeners to use auto-discovery (omits Protocol, PortRanges, ClientAffinity, TrafficDialPercentage)
func createAGAWithGatewayEndpoint(name, namespace, acceleratorName, endpointName string, ipAddressType agav1beta1.IPAddressType, listeners *[]agav1beta1.GlobalAcceleratorListener) *agav1beta1.GlobalAccelerator {
	if listeners == nil {
		listeners = &[]agav1beta1.GlobalAcceleratorListener{
			{
				EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{
					{
						Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{
							{
								Type: agav1beta1.GlobalAcceleratorEndpointTypeGateway,
								Name: awssdk.String(endpointName),
							},
						},
					},
				},
			},
		}
	}
	return createAGA(name, namespace, acceleratorName, ipAddressType, listeners)
}

// createAGAWithEndpointID creates a GlobalAccelerator with a direct endpoint ID (ARN)
// Pass nil for listeners to use auto-discovery (omits Protocol, PortRanges, ClientAffinity, TrafficDialPercentage)
func createAGAWithEndpointID(name, namespace, acceleratorName, endpointID string, ipAddressType agav1beta1.IPAddressType, listeners *[]agav1beta1.GlobalAcceleratorListener) *agav1beta1.GlobalAccelerator {
	if listeners == nil {
		ep := agav1beta1.GlobalAcceleratorEndpoint{
			Type:       agav1beta1.GlobalAcceleratorEndpointTypeEndpointID,
			EndpointID: awssdk.String(endpointID),
		}
		if tf.Options.IPFamily == framework.IPv6 {
			ep.ClientIPPreservationEnabled = awssdk.Bool(false)
		}
		listeners = &[]agav1beta1.GlobalAcceleratorListener{
			{
				EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{
					{
						Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{ep},
					},
				},
			},
		}
	}
	return createAGA(name, namespace, acceleratorName, ipAddressType, listeners)
}

// createDeployment creates a deployment for testing
func createDeployment(baseName, namespace string, labels map[string]string) *appsv1.Deployment {
	replicas := int32(2)
	dpImage := utils.GetDeploymentImage(tf.Options.TestImageRegistry, utils.HelloImage)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baseName + "-deployment",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
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
							Image:           dpImage,
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{ContainerPort: 80},
							},
						},
					},
				},
			},
		},
	}
}

func createNodePortService(svcName, namespace string, labels map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
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
}

func createALBIngress(ingName, namespace, serviceName string) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	annotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme": "internet-facing",
	}
	if tf.Options.IPFamily == framework.IPv6 {
		annotations["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
	}

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ingName,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: awssdk.String("alb"),
			Rules: []networkingv1.IngressRule{
				{
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: serviceName,
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func createLoadBalancerService(svcName string, labels map[string]string, annotations map[string]string) *corev1.Service {
	return createLoadBalancerServiceWithPorts(svcName, labels, annotations, 80)
}

func createLoadBalancerServiceWithPorts(svcName string, labels map[string]string, annotations map[string]string, ports ...int32) *corev1.Service {
	servicePorts := make([]corev1.ServicePort, len(ports))
	for i, port := range ports {
		portName := intstr.FromInt(int(port))
		servicePorts[i] = corev1.ServicePort{
			Name:       "port-" + portName.String(),
			Port:       port,
			TargetPort: intstr.FromInt(80),
			Protocol:   corev1.ProtocolTCP,
		}
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        svcName,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: labels,
			Ports:    servicePorts,
		},
	}
}

func createServiceAnnotations(lbType, scheme string, ipFamily string) map[string]string {
	annotations := map[string]string{
		"service.beta.kubernetes.io/aws-load-balancer-type":   lbType,
		"service.beta.kubernetes.io/aws-load-balancer-scheme": scheme,
	}
	if ipFamily == framework.IPv6 {
		annotations["service.beta.kubernetes.io/aws-load-balancer-ip-address-type"] = "dualstack"
	}
	return annotations
}

// createServiceEndpoint creates a Service endpoint with appropriate clientIPPreservationEnabled setting
func createServiceEndpoint(svcName string, weight int32) agav1beta1.GlobalAcceleratorEndpoint {
	ep := agav1beta1.GlobalAcceleratorEndpoint{
		Type:   agav1beta1.GlobalAcceleratorEndpointTypeService,
		Name:   awssdk.String(svcName),
		Weight: awssdk.Int32(weight),
	}
	if tf.Options.IPFamily == framework.IPv6 {
		ep.ClientIPPreservationEnabled = awssdk.Bool(false)
	}
	return ep
}

// createEndpointIDEndpoint creates an EndpointID endpoint with appropriate clientIPPreservationEnabled setting
func createEndpointIDEndpoint(endpointID string) agav1beta1.GlobalAcceleratorEndpoint {
	ep := agav1beta1.GlobalAcceleratorEndpoint{
		Type:       agav1beta1.GlobalAcceleratorEndpointTypeEndpointID,
		EndpointID: awssdk.String(endpointID),
	}
	if tf.Options.IPFamily == framework.IPv6 {
		ep.ClientIPPreservationEnabled = awssdk.Bool(false)
	}
	return ep
}
