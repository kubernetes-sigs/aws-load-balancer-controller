package gateway

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	"strings"
)

func buildDeploymentSpec(testImageRegistry string) *appsv1.Deployment {
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

func buildServiceSpec() *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/name":     "multi-port",
		"app.kubernetes.io/instance": defaultName,
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
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
	return svc
}

func buildGatewayClassSpec(controllerName string) *gwv1.GatewayClass {
	lbType := strings.Split(controllerName, "/")[1]
	gwc := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultGatewayClassName + "-" + lbType,
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: gwv1.GatewayController(controllerName),
		},
	}
	return gwc
}

func buildLoadBalancerConfig(spec elbv2gw.LoadBalancerConfigurationSpec) *elbv2gw.LoadBalancerConfiguration {
	lbc := &elbv2gw.LoadBalancerConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultLbConfigName,
		},
		Spec: spec,
	}
	return lbc
}

func buildTargetGroupConfig(spec elbv2gw.TargetGroupConfigurationSpec, svc *corev1.Service) *elbv2gw.TargetGroupConfiguration {
	spec.TargetReference.Name = svc.Name
	tgc := &elbv2gw.TargetGroupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultTgConfigName,
		},
		Spec: spec,
	}
	return tgc
}

func buildBasicGatewaySpec(gwc *gwv1.GatewayClass, protocol gwv1.ProtocolType) *gwv1.Gateway {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: gwv1.ObjectName(gwc.Name),
			Listeners: []gwv1.Listener{
				{
					Name:     "test-listener",
					Port:     80,
					Protocol: protocol,
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

func buildTCPRoute() *gwalpha2.TCPRoute {
	port := gwalpha2.PortNumber(80)
	tcpr := &gwalpha2.TCPRoute{
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
			Rules: []gwalpha2.TCPRouteRule{
				{
					BackendRefs: []gwalpha2.BackendRef{
						{
							BackendObjectReference: gwalpha2.BackendObjectReference{
								Name: defaultName,
								Port: &port,
							},
						},
					},
				},
			},
		},
	}
	return tcpr
}

func buildHTTPRoute() *gwv1.HTTPRoute {
	port := gwalpha2.PortNumber(80)
	httpr := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: defaultName,
					},
				},
			},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: defaultName,
									Port: &port,
								},
							},
						},
					},
				},
			},
		},
	}
	return httpr
}
