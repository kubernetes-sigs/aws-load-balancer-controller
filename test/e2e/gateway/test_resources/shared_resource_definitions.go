package test_resources

import (
	"context"
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/http"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func BuildDeploymentSpec(testImageRegistry string) *appsv1.Deployment {
	numReplicas := int32(DefaultNumReplicas)
	labels := map[string]string{
		"app.kubernetes.io/name":     "multi-port",
		"app.kubernetes.io/instance": DefaultName,
	}
	dpImage := utils.GetDeploymentImage(testImageRegistry, utils.HelloImage)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultName,
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
									ContainerPort: AppContainerPort,
								},
							},
						},
					},
				},
			},
		},
	}
}

func BuildCustomizableResponseDeploymentSpec(dpName, fixedResponseContent, testImageRegistry string) *appsv1.Deployment {
	numReplicas := int32(DefaultNumReplicas)
	labels := map[string]string{
		"app.kubernetes.io/instance": dpName,
	}
	dpImage := utils.GetDeploymentImage(testImageRegistry, utils.ColortellerImage)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: dpName,
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
									ContainerPort: AppContainerPort,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "SERVER_PORT",
									Value: fmt.Sprintf("%d", AppContainerPort),
								},
								{
									Name:  "COLOR",
									Value: fixedResponseContent,
								},
							},
						},
					},
				},
			},
		},
	}
}

func BuildUDPDeploymentSpec() *appsv1.Deployment {
	numReplicas := int32(DefaultNumReplicas)
	labels := map[string]string{
		"app.kubernetes.io/instance": UDPDefaultName,
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: UDPDefaultName,
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
									ContainerPort: UDPContainerPort,
									Protocol:      corev1.ProtocolUDP,
									Name:          "udp8080",
								},
								{
									ContainerPort: UDPContainerPort,
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

func BuildGRPCDeploymentSpec(name string, fixedResponseMessage string, labels map[string]string) *appsv1.Deployment {
	numReplicas := int32(DefaultNumReplicas)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
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
							Image:           utils.GRPCImage,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: GRPCContainerPort,
									Protocol:      corev1.ProtocolTCP,
									Name:          "tcp50051",
								},
							},
							Args: []string{
								fixedResponseMessage,
							},
						},
					},
				},
			},
		},
	}
}

func BuildServiceSpec(labels map[string]string) *corev1.Service {
	if len(labels) == 0 {
		labels["app.kubernetes.io/instance"] = DefaultName
		labels["app.kubernetes.io/name"] = "multi-port"
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultName,
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

func BuildUDPServiceSpec() *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/instance": UDPDefaultName,
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: UDPDefaultName,
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

func BuildGRPCServiceSpec(name string, labels map[string]string) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "tcp50051",
					Port:       50051,
					TargetPort: intstr.FromInt(50051),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	return svc
}

func BuildGatewayClassSpec(controllerName string) *gwv1.GatewayClass {
	lbType := strings.Split(controllerName, "/")[1]
	gwc := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-%s", DefaultGatewayClassName, lbType, utils.RandomDNS1123Label(6)),
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: gwv1.GatewayController(controllerName),
		},
	}
	return gwc
}

func BuildLoadBalancerConfig(spec elbv2gw.LoadBalancerConfigurationSpec) *elbv2gw.LoadBalancerConfiguration {
	lbc := &elbv2gw.LoadBalancerConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultLbConfigName,
		},
		Spec: spec,
	}
	return lbc
}

func BuildTargetGroupConfig(name string, spec elbv2gw.TargetGroupConfigurationSpec, svc *corev1.Service) *elbv2gw.TargetGroupConfiguration {
	if spec.TargetReference == nil {
		spec.TargetReference = &elbv2gw.Reference{}
	}
	spec.TargetReference.Name = svc.Name
	tgc := &elbv2gw.TargetGroupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: *(spec.DeepCopy()),
	}
	return tgc
}

// BuildDefaultTargetGroupConfig creates a TGC without targetReference, used as a gateway-level default via LBC.
func BuildDefaultTargetGroupConfig(name string, props elbv2gw.TargetGroupProps) *elbv2gw.TargetGroupConfiguration {
	return &elbv2gw.TargetGroupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: elbv2gw.TargetGroupConfigurationSpec{
			DefaultConfiguration: props,
		},
	}
}

func BuildListenerRuleConfig(name string, spec elbv2gw.ListenerRuleConfigurationSpec) *elbv2gw.ListenerRuleConfiguration {
	lrc := &elbv2gw.ListenerRuleConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: *(spec.DeepCopy()),
	}
	return lrc
}

func BuildBasicGatewaySpec(gwc *gwv1.GatewayClass, listeners []gwv1.Listener) *gwv1.Gateway {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultName,
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: gwv1.ObjectName(gwc.Name),
			Listeners:        listeners,
			Infrastructure: &gwv1.GatewayInfrastructure{
				ParametersRef: &gwv1.LocalParametersReference{
					Group: "gateway.k8s.aws",
					Kind:  "LoadBalancerConfiguration",
					Name:  DefaultLbConfigName,
				},
			},
		},
	}
	return gw
}

func BuildTCPRoute(parentRefs []gwv1.ParentReference, backendRefs []gwalpha2.BackendRef) *gwalpha2.TCPRoute {

	if len(backendRefs) == 0 {
		port := gwalpha2.PortNumber(80)
		backendRefs = []gwalpha2.BackendRef{
			{
				BackendObjectReference: gwalpha2.BackendObjectReference{
					Name: DefaultName,
					Port: new(port),
				},
			},
		}
	}

	if len(parentRefs) == 0 {
		parentRefs = []gwalpha2.ParentReference{
			{
				Name:        DefaultName,
				SectionName: (*gwv1.SectionName)(awssdk.String("port80")),
			},
			{
				Name:        DefaultName,
				SectionName: (*gwv1.SectionName)(awssdk.String("port443")),
			},
		}
	}
	tcpr := &gwalpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultName,
		},
		Spec: gwalpha2.TCPRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: parentRefs,
			},
			Rules: []gwalpha2.TCPRouteRule{
				{
					BackendRefs: backendRefs,
				},
			},
		},
	}
	return tcpr
}

func BuildFENLBTCPRoute(albGatewayName, albNamespace string, port gwalpha2.PortNumber) *gwalpha2.TCPRoute {
	tcpr := &gwalpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("fenlb-tcp-route-%d", port),
		},
		Spec: gwalpha2.TCPRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: DefaultName,
						Port: &port,
					},
				},
			},
			Rules: []gwalpha2.TCPRouteRule{
				{
					BackendRefs: []gwalpha2.BackendRef{
						{
							BackendObjectReference: gwalpha2.BackendObjectReference{
								Name:      gwv1.ObjectName(albGatewayName),
								Kind:      (*gwv1.Kind)(awssdk.String("Gateway")),
								Namespace: (*gwv1.Namespace)(&albNamespace),
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

func BuildUDPRoute(sectionName string) *gwalpha2.UDPRoute {
	port := gwalpha2.PortNumber(8080)
	udpr := &gwalpha2.UDPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultName,
		},
		Spec: gwalpha2.UDPRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name:        DefaultName,
						SectionName: (*gwv1.SectionName)(awssdk.String(sectionName)),
					},
				},
			},
			Rules: []gwalpha2.UDPRouteRule{
				{
					BackendRefs: []gwalpha2.BackendRef{
						{
							BackendObjectReference: gwalpha2.BackendObjectReference{
								Name: UDPDefaultName,
								Port: new(port),
							},
						},
					},
				},
			},
		},
	}
	return udpr
}

func BuildHTTPRoute(hostnames []string, rules []gwv1.HTTPRouteRule, sectionName *gwv1.SectionName) *gwv1.HTTPRoute {
	routeName := fmt.Sprintf("%v-%v", DefaultName, utils.RandomDNS1123Label(6))
	httpr := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: routeName,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name:        DefaultName,
						SectionName: sectionName,
					},
				},
			},
		},
	}
	routeHostnames := make([]gwv1.Hostname, 0, len(hostnames))
	for _, hostname := range hostnames {
		routeHostnames = append(routeHostnames, gwv1.Hostname(hostname))
	}
	httpr.Spec.Hostnames = routeHostnames
	if len(rules) > 0 {
		httpr.Spec.Rules = rules
	} else {
		httpr.Spec.Rules = []gwv1.HTTPRouteRule{
			{
				BackendRefs: DefaultHttpRouteRuleBackendRefs,
			},
		}
	}
	return httpr
}

func BuildGRPCRoute(hostnames []string, rules []gwv1.GRPCRouteRule, sectionName *gwv1.SectionName) *gwv1.GRPCRoute {
	routeName := fmt.Sprintf("%v-%v", DefaultName, utils.RandomDNS1123Label(6))
	grcpr := &gwv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: routeName,
		},
		Spec: gwv1.GRPCRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name:        DefaultName,
						SectionName: sectionName,
					},
				},
			},
		},
	}
	routeHostnames := make([]gwv1.Hostname, 0, len(hostnames))
	for _, hostname := range hostnames {
		routeHostnames = append(routeHostnames, gwv1.Hostname(hostname))
	}
	grcpr.Spec.Hostnames = routeHostnames
	if len(rules) > 0 {
		grcpr.Spec.Rules = rules
	} else {
		grcpr.Spec.Rules = []gwv1.GRPCRouteRule{
			{
				BackendRefs: DefaultGrpcRouteRuleBackendRefs,
			},
		}
	}
	return grcpr
}

func BuildOtherNsRefTcpRoute(sectionName string, otherNs *corev1.Namespace) *gwalpha2.TCPRoute {
	port := gwalpha2.PortNumber(80)
	tcpr := &gwalpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultName + "-otherns",
		},
		Spec: gwalpha2.TCPRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name:        DefaultName,
						SectionName: (*gwv1.SectionName)(awssdk.String(sectionName)),
					},
				},
			},
			Rules: []gwalpha2.TCPRouteRule{
				{
					BackendRefs: []gwalpha2.BackendRef{
						{
							BackendObjectReference: gwalpha2.BackendObjectReference{
								Name:      DefaultName,
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

func BuildOtherNsRefHttpRoute(sectionName string, otherNs *corev1.Namespace) *gwv1.HTTPRoute {
	port := gwalpha2.PortNumber(80)
	httpr := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultName + "-otherns",
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name:        DefaultName,
						SectionName: (*gwv1.SectionName)(awssdk.String(sectionName)),
					},
				},
			},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      DefaultName,
									Namespace: (*gwv1.Namespace)(&otherNs.Name),
									Port:      new(port),
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

func AllocateNamespace(ctx context.Context, f *framework.Framework, baseName string, namespaceLabels map[string]string) (*corev1.Namespace, error) {
	f.Logger.Info("allocating namespace")
	ns, err := f.NSManager.AllocateNamespace(ctx, baseName)
	if err != nil {
		return nil, err
	}
	f.Logger.Info("allocated namespace", "nsName", ns.Name)
	if namespaceLabels != nil && len(namespaceLabels) > 0 {
		f.Logger.Info("label namespace", "nsName", ns.Name, "labels", namespaceLabels)
		oldNS := ns.DeepCopy()
		ns.Labels = algorithm.MergeStringMap(namespaceLabels, ns.Labels)
		err := f.K8sClient.Patch(ctx, ns, client.MergeFrom(oldNS))
		if err != nil {
			return nil, err
		}
	}
	return ns, nil
}

type BodyMatcher struct {
	ResponseCount map[string]int
}

func (b *BodyMatcher) Matches(resp http.Response) error {
	if resp.ResponseCode >= 300 {
		return nil
	}
	bodyString := string(resp.Body)
	_, ok := b.ResponseCount[bodyString]
	if !ok {
		b.ResponseCount[bodyString] = 0
	}
	b.ResponseCount[bodyString]++
	return nil
}

func BuildTCPRouteWithMismatchedParentRefs() *gwalpha2.TCPRoute {
	port := gwalpha2.PortNumber(80)
	tcpr := &gwalpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultName,
		},
		Spec: gwalpha2.TCPRouteSpec{
			CommonRouteSpec: gwalpha2.CommonRouteSpec{
				ParentRefs: []gwalpha2.ParentReference{
					{
						Name:        DefaultName,
						SectionName: (*gwv1.SectionName)(awssdk.String("listener-exists")),
					},
					{
						Name:        DefaultName,
						SectionName: (*gwv1.SectionName)(awssdk.String("listener-nonexist")),
					},
				},
			},
			Rules: []gwalpha2.TCPRouteRule{
				{
					BackendRefs: []gwalpha2.BackendRef{
						{
							BackendObjectReference: gwalpha2.BackendObjectReference{
								Name: DefaultName,
								Port: new(port),
							},
						},
					},
				},
			},
		},
	}
	return tcpr
}
