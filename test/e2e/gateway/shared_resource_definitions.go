package gateway

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func buildUDPDeploymentSpec() *appsv1.Deployment {
	numReplicas := int32(defaultNumReplicas)
	labels := map[string]string{
		"app.kubernetes.io/instance": udpDefaultName,
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: udpDefaultName,
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
							Image:           utils.UDPImage,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: udpContainerPort,
									Protocol:      corev1.ProtocolUDP,
									Name:          "udp8080",
								},
								{
									ContainerPort: udpContainerPort,
									Protocol:      corev1.ProtocolTCP,
									Name:          "tcp8080",
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

func buildUDPServiceSpec() *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/instance": udpDefaultName,
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: udpDefaultName,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "tcp8080",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "udp8080",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolUDP,
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

func buildTargetGroupConfig(name string, spec elbv2gw.TargetGroupConfigurationSpec, svc *corev1.Service) *elbv2gw.TargetGroupConfiguration {
	spec.TargetReference.Name = svc.Name
	tgc := &elbv2gw.TargetGroupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: *(spec.DeepCopy()),
	}
	return tgc
}

func buildBasicGatewaySpec(gwc *gwv1.GatewayClass, listeners []gwv1.Listener) *gwv1.Gateway {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: gwv1.ObjectName(gwc.Name),
			Listeners:        listeners,
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
						Name:        defaultName,
						SectionName: (*gwv1.SectionName)(awssdk.String("port80")),
					},
					{
						Name:        defaultName,
						SectionName: (*gwv1.SectionName)(awssdk.String("port443")),
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

func buildOtherNsRefTcpRoute(sectionName string, otherNs *corev1.Namespace) *gwalpha2.TCPRoute {
	port := gwalpha2.PortNumber(80)
	tcpr := &gwalpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName + "-otherns",
		},
		Spec: gwalpha2.TCPRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name:        defaultName,
						SectionName: (*gwv1.SectionName)(awssdk.String(sectionName)),
					},
				},
			},
			Rules: []gwalpha2.TCPRouteRule{
				{
					BackendRefs: []gwalpha2.BackendRef{
						{
							BackendObjectReference: gwalpha2.BackendObjectReference{
								Name:      defaultName,
								Namespace: (*gwv1.Namespace)(&otherNs.Name),
								Port:      &port,
							},
						},
					},
				},
			},
		},
	}
	return tcpr
}

func buildUDPRoute() *gwalpha2.UDPRoute {
	port := gwalpha2.PortNumber(8080)
	udpr := &gwalpha2.UDPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
		Spec: gwalpha2.UDPRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name:        defaultName,
						SectionName: (*gwv1.SectionName)(awssdk.String("port8080")),
					},
				},
			},
			Rules: []gwalpha2.UDPRouteRule{
				{
					BackendRefs: []gwalpha2.BackendRef{
						{
							BackendObjectReference: gwalpha2.BackendObjectReference{
								Name: udpDefaultName,
								Port: &port,
							},
						},
					},
				},
			},
		},
	}
	return udpr
}

/*
func buildTLSRoute() *gwalpha2.TLSRoute {
	port := gwalpha2.PortNumber(80)
	tlrs := &gwalpha2.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
		Spec: gwalpha2.TLSRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name:        defaultName,
						SectionName: (*gwv1.SectionName)(awssdk.String("port443")),
					},
				},
			},
			Rules: []gwalpha2.TLSRouteRule{
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
	return tlrs
}

*/

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

func allocateNamespace(ctx context.Context, f *framework.Framework, baseName string, enablePodReadinessGate bool) (*corev1.Namespace, error) {
	f.Logger.Info("allocating namespace")
	ns, err := f.NSManager.AllocateNamespace(ctx, baseName)
	if err != nil {
		return nil, err
	}
	f.Logger.Info("allocated namespace", "nsName", ns.Name)
	if enablePodReadinessGate {
		f.Logger.Info("label namespace for podReadinessGate injection", "nsName", ns.Name)
		oldNS := ns.DeepCopy()
		ns.Labels = algorithm.MergeStringMap(map[string]string{
			"elbv2.k8s.aws/pod-readiness-gate-inject": "enabled",
		}, ns.Labels)
		err := f.K8sClient.Patch(ctx, ns, client.MergeFrom(oldNS))
		if err != nil {
			return nil, err
		}
		f.Logger.Info("labeled namespace with podReadinessGate injection", "nsName", ns.Name)
	}
	return ns, nil
}
