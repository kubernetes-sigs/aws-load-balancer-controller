package model

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2modelk8s "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

func Test_buildTargetGroupSpec(t *testing.T) {
	http1 := elbv2model.ProtocolVersionHTTP1
	testCases := []struct {
		name                     string
		tags                     map[string]string
		lbType                   elbv2model.LoadBalancerType
		disableRestrictedSGRules bool
		defaultTargetType        string
		gateway                  *gwv1.Gateway
		route                    *routeutils.MockRoute
		backend                  routeutils.ServiceBackendConfig
		tagErr                   error
		expectErr                bool
		expectedTgSpec           elbv2model.TargetGroupSpec
	}{
		{
			name:                     "no tg config - instance - nlb",
			tags:                     make(map[string]string),
			lbType:                   elbv2model.LoadBalancerTypeNetwork,
			disableRestrictedSGRules: false,
			defaultTargetType:        string(elbv2model.TargetTypeInstance),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "my-gw-ns",
					Name:      "my-gw",
				},
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "my-route",
				Namespace: "my-route-ns",
			},
			backend: routeutils.ServiceBackendConfig{
				Service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				ServicePort: &corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			},
			expectedTgSpec: elbv2model.TargetGroupSpec{
				Name:          "k8s-myrouten-myroute-8d8111f6ac",
				TargetType:    elbv2model.TargetTypeInstance,
				Port:          awssdk.Int32(8080),
				Protocol:      elbv2model.ProtocolTCP,
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr.IntOrString{
						StrVal: shared_constants.HealthCheckPortTrafficPort,
						Type:   intstr.String,
					},
					Protocol:                elbv2model.ProtocolTCP,
					IntervalSeconds:         awssdk.Int32(15),
					TimeoutSeconds:          awssdk.Int32(5),
					HealthyThresholdCount:   awssdk.Int32(3),
					UnhealthyThresholdCount: awssdk.Int32(3),
				},
				TargetGroupAttributes: make([]elbv2model.TargetGroupAttribute, 0),
				Tags:                  make(map[string]string),
			},
		},
		{
			name:                     "no tg config - instance - alb",
			tags:                     make(map[string]string),
			lbType:                   elbv2model.LoadBalancerTypeApplication,
			disableRestrictedSGRules: false,
			defaultTargetType:        string(elbv2model.TargetTypeInstance),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "my-gw-ns",
					Name:      "my-gw",
				},
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "my-route",
				Namespace: "my-route-ns",
			},
			backend: routeutils.ServiceBackendConfig{
				Service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				ServicePort: &corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			},
			expectedTgSpec: elbv2model.TargetGroupSpec{
				Name:            "k8s-myrouten-myroute-224f4b6ea6",
				TargetType:      elbv2model.TargetTypeInstance,
				Port:            awssdk.Int32(8080),
				Protocol:        elbv2model.ProtocolHTTP,
				ProtocolVersion: &http1,
				IPAddressType:   elbv2model.TargetGroupIPAddressTypeIPv4,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr.IntOrString{
						StrVal: shared_constants.HealthCheckPortTrafficPort,
						Type:   intstr.String,
					},
					Path: awssdk.String("/"),
					Matcher: &elbv2model.HealthCheckMatcher{
						HTTPCode: awssdk.String("200-399"),
					},
					Protocol:                elbv2model.ProtocolHTTP,
					IntervalSeconds:         awssdk.Int32(15),
					TimeoutSeconds:          awssdk.Int32(5),
					HealthyThresholdCount:   awssdk.Int32(3),
					UnhealthyThresholdCount: awssdk.Int32(3),
				},
				TargetGroupAttributes: make([]elbv2model.TargetGroupAttribute, 0),
				Tags:                  make(map[string]string),
			},
		},
		{
			name:                     "no tg config - ip - nlb",
			tags:                     make(map[string]string),
			lbType:                   elbv2model.LoadBalancerTypeNetwork,
			disableRestrictedSGRules: false,
			defaultTargetType:        string(elbv2model.TargetTypeIP),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "my-gw-ns",
					Name:      "my-gw",
				},
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "my-route",
				Namespace: "my-route-ns",
			},
			backend: routeutils.ServiceBackendConfig{
				Service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				ServicePort: &corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			},
			expectedTgSpec: elbv2model.TargetGroupSpec{
				Name:          "k8s-myrouten-myroute-3bce8b0f70",
				TargetType:    elbv2model.TargetTypeIP,
				Port:          awssdk.Int32(80),
				Protocol:      elbv2model.ProtocolTCP,
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr.IntOrString{
						StrVal: shared_constants.HealthCheckPortTrafficPort,
						Type:   intstr.String,
					},
					Protocol:                elbv2model.ProtocolTCP,
					IntervalSeconds:         awssdk.Int32(15),
					TimeoutSeconds:          awssdk.Int32(5),
					HealthyThresholdCount:   awssdk.Int32(3),
					UnhealthyThresholdCount: awssdk.Int32(3),
				},
				TargetGroupAttributes: make([]elbv2model.TargetGroupAttribute, 0),
				Tags:                  make(map[string]string),
			},
		},
		{
			name:                     "no tg config - ip - alb",
			tags:                     make(map[string]string),
			lbType:                   elbv2model.LoadBalancerTypeApplication,
			disableRestrictedSGRules: false,
			defaultTargetType:        string(elbv2model.TargetTypeIP),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "my-gw-ns",
					Name:      "my-gw",
				},
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "my-route",
				Namespace: "my-route-ns",
			},
			backend: routeutils.ServiceBackendConfig{
				Service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				ServicePort: &corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			},
			expectedTgSpec: elbv2model.TargetGroupSpec{
				Name:            "k8s-myrouten-myroute-a44a20bcbf",
				TargetType:      elbv2model.TargetTypeIP,
				Port:            awssdk.Int32(80),
				Protocol:        elbv2model.ProtocolHTTP,
				ProtocolVersion: &http1,
				IPAddressType:   elbv2model.TargetGroupIPAddressTypeIPv4,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr.IntOrString{
						StrVal: shared_constants.HealthCheckPortTrafficPort,
						Type:   intstr.String,
					},
					Path: awssdk.String("/"),
					Matcher: &elbv2model.HealthCheckMatcher{
						HTTPCode: awssdk.String("200-399"),
					},
					Protocol:                elbv2model.ProtocolHTTP,
					IntervalSeconds:         awssdk.Int32(15),
					TimeoutSeconds:          awssdk.Int32(5),
					HealthyThresholdCount:   awssdk.Int32(3),
					UnhealthyThresholdCount: awssdk.Int32(3),
				},
				TargetGroupAttributes: make([]elbv2model.TargetGroupAttribute, 0),
				Tags:                  make(map[string]string),
			},
		},
		{
			name:                     "wrong svc type for instance should produce an error",
			tags:                     make(map[string]string),
			lbType:                   elbv2model.LoadBalancerTypeNetwork,
			disableRestrictedSGRules: false,
			defaultTargetType:        string(elbv2model.TargetTypeInstance),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "my-gw-ns",
					Name:      "my-gw",
				},
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "my-route",
				Namespace: "my-route-ns",
			},
			backend: routeutils.ServiceBackendConfig{
				Service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				ServicePort: &corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			tagger := &mockTagHelper{
				tags: tc.tags,
				err:  tc.tagErr,
			}

			builder := newTargetGroupBuilder("my-cluster", "vpc-xxx", tagger, tc.lbType, gateway.NewTargetGroupConfigConstructor(), tc.disableRestrictedSGRules, tc.defaultTargetType, nil)

			out, err := builder.(*targetGroupBuilderImpl).buildTargetGroupSpec(tc.gateway, tc.route, elbv2gw.LoadBalancerConfiguration{}, elbv2model.IPAddressTypeIPV4, tc.backend, nil)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedTgSpec, out)
		})
	}
}

func Test_buildTargetGroupBindingSpec(t *testing.T) {
	instanceType := elbv2api.TargetType(elbv2model.TargetTypeInstance)
	ipType := elbv2api.TargetType(elbv2model.TargetTypeIP)
	http1 := elbv2model.ProtocolVersionHTTP1
	tcpProtocol := elbv2model.ProtocolTCP
	httpProtocol := elbv2model.ProtocolHTTP
	testCases := []struct {
		name                     string
		tags                     map[string]string
		lbType                   elbv2model.LoadBalancerType
		disableRestrictedSGRules bool
		defaultTargetType        string
		gateway                  *gwv1.Gateway
		route                    *routeutils.MockRoute
		backend                  routeutils.ServiceBackendConfig
		tagErr                   error
		expectErr                bool
		expectedTgSpec           elbv2model.TargetGroupSpec
		expectedTgBindingSpec    elbv2modelk8s.TargetGroupBindingResourceSpec
	}{
		{
			name:                     "no tg config - instance - nlb",
			tags:                     make(map[string]string),
			lbType:                   elbv2model.LoadBalancerTypeNetwork,
			disableRestrictedSGRules: false,
			defaultTargetType:        string(elbv2model.TargetTypeInstance),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "my-gw-ns",
					Name:      "my-gw",
				},
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "my-route",
				Namespace: "my-route-ns",
			},
			backend: routeutils.ServiceBackendConfig{
				Service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				ServicePort: &corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			},
			expectedTgSpec: elbv2model.TargetGroupSpec{
				Name:          "k8s-myrouten-myroute-d02da2803b",
				TargetType:    elbv2model.TargetTypeInstance,
				Port:          awssdk.Int32(8080),
				Protocol:      elbv2model.ProtocolTCP,
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr.IntOrString{
						StrVal: shared_constants.HealthCheckPortTrafficPort,
						Type:   intstr.String,
					},
					Protocol:                elbv2model.ProtocolTCP,
					IntervalSeconds:         awssdk.Int32(15),
					TimeoutSeconds:          awssdk.Int32(5),
					HealthyThresholdCount:   awssdk.Int32(3),
					UnhealthyThresholdCount: awssdk.Int32(3),
				},
				TargetGroupAttributes: make([]elbv2model.TargetGroupAttribute, 0),
				Tags:                  make(map[string]string),
			},
			expectedTgBindingSpec: elbv2modelk8s.TargetGroupBindingResourceSpec{
				Template: elbv2modelk8s.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "my-svc-ns",
						Name:        "k8s-myrouten-myroute-d02da2803b",
						Annotations: make(map[string]string),
						Labels:      make(map[string]string),
					},
					Spec: elbv2modelk8s.TargetGroupBindingSpec{
						TargetType: &instanceType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromInt32(80), // TODO - Figure out why this port is added and not the node port.
						},
						IPAddressType:       elbv2api.TargetGroupIPAddressType(elbv2model.IPAddressTypeIPV4),
						VpcID:               "vpc-xxx",
						TargetGroupProtocol: &tcpProtocol,
					},
				},
			},
		},
		{
			name:                     "no tg config - instance - alb",
			tags:                     make(map[string]string),
			lbType:                   elbv2model.LoadBalancerTypeApplication,
			disableRestrictedSGRules: false,
			defaultTargetType:        string(elbv2model.TargetTypeInstance),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "my-gw-ns",
					Name:      "my-gw",
				},
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "my-route",
				Namespace: "my-route-ns",
			},
			backend: routeutils.ServiceBackendConfig{
				Service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				ServicePort: &corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			},
			expectedTgSpec: elbv2model.TargetGroupSpec{
				Name:            "k8s-myrouten-myroute-224f4b6ea6",
				TargetType:      elbv2model.TargetTypeInstance,
				Port:            awssdk.Int32(8080),
				Protocol:        elbv2model.ProtocolHTTP,
				ProtocolVersion: &http1,
				IPAddressType:   elbv2model.TargetGroupIPAddressTypeIPv4,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr.IntOrString{
						StrVal: shared_constants.HealthCheckPortTrafficPort,
						Type:   intstr.String,
					},
					Path: awssdk.String("/"),
					Matcher: &elbv2model.HealthCheckMatcher{
						HTTPCode: awssdk.String("200-399"),
					},
					Protocol:                elbv2model.ProtocolHTTP,
					IntervalSeconds:         awssdk.Int32(15),
					TimeoutSeconds:          awssdk.Int32(5),
					HealthyThresholdCount:   awssdk.Int32(3),
					UnhealthyThresholdCount: awssdk.Int32(3),
				},
				TargetGroupAttributes: make([]elbv2model.TargetGroupAttribute, 0),
				Tags:                  make(map[string]string),
			},
			expectedTgBindingSpec: elbv2modelk8s.TargetGroupBindingResourceSpec{
				Template: elbv2modelk8s.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "my-svc-ns",
						Name:        "k8s-myrouten-myroute-224f4b6ea6",
						Annotations: make(map[string]string),
						Labels:      make(map[string]string),
					},
					Spec: elbv2modelk8s.TargetGroupBindingSpec{
						TargetType: &instanceType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromInt32(80), // TODO - Figure out why this port is added and not the node port.
						},
						IPAddressType:       elbv2api.TargetGroupIPAddressType(elbv2model.IPAddressTypeIPV4),
						VpcID:               "vpc-xxx",
						TargetGroupProtocol: &httpProtocol,
					},
				},
			},
		},
		{
			name:                     "no tg config - ip - nlb",
			tags:                     make(map[string]string),
			lbType:                   elbv2model.LoadBalancerTypeNetwork,
			disableRestrictedSGRules: false,
			defaultTargetType:        string(elbv2model.TargetTypeIP),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "my-gw-ns",
					Name:      "my-gw",
				},
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "my-route",
				Namespace: "my-route-ns",
			},
			backend: routeutils.ServiceBackendConfig{
				Service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				ServicePort: &corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			},
			expectedTgSpec: elbv2model.TargetGroupSpec{
				Name:          "k8s-myrouten-myroute-3bce8b0f70",
				TargetType:    elbv2model.TargetTypeIP,
				Port:          awssdk.Int32(80),
				Protocol:      elbv2model.ProtocolTCP,
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr.IntOrString{
						StrVal: shared_constants.HealthCheckPortTrafficPort,
						Type:   intstr.String,
					},
					Protocol:                elbv2model.ProtocolTCP,
					IntervalSeconds:         awssdk.Int32(15),
					TimeoutSeconds:          awssdk.Int32(5),
					HealthyThresholdCount:   awssdk.Int32(3),
					UnhealthyThresholdCount: awssdk.Int32(3),
				},
				TargetGroupAttributes: make([]elbv2model.TargetGroupAttribute, 0),
				Tags:                  make(map[string]string),
			},
			expectedTgBindingSpec: elbv2modelk8s.TargetGroupBindingResourceSpec{
				Template: elbv2modelk8s.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "my-svc-ns",
						Name:        "k8s-myrouten-myroute-3bce8b0f70",
						Annotations: make(map[string]string),
						Labels:      make(map[string]string),
					},
					Spec: elbv2modelk8s.TargetGroupBindingSpec{
						TargetType: &ipType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromInt32(80),
						},
						IPAddressType:       elbv2api.TargetGroupIPAddressType(elbv2model.IPAddressTypeIPV4),
						VpcID:               "vpc-xxx",
						TargetGroupProtocol: &tcpProtocol,
					},
				},
			},
		},
		{
			name:                     "no tg config - ip - alb",
			tags:                     make(map[string]string),
			lbType:                   elbv2model.LoadBalancerTypeApplication,
			disableRestrictedSGRules: false,
			defaultTargetType:        string(elbv2model.TargetTypeIP),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "my-gw-ns",
					Name:      "my-gw",
				},
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "my-route",
				Namespace: "my-route-ns",
			},
			backend: routeutils.ServiceBackendConfig{
				Service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				ServicePort: &corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			},
			expectedTgSpec: elbv2model.TargetGroupSpec{
				Name:            "k8s-myrouten-myroute-a44a20bcbf",
				TargetType:      elbv2model.TargetTypeIP,
				Port:            awssdk.Int32(80),
				Protocol:        elbv2model.ProtocolHTTP,
				ProtocolVersion: &http1,
				IPAddressType:   elbv2model.TargetGroupIPAddressTypeIPv4,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr.IntOrString{
						StrVal: shared_constants.HealthCheckPortTrafficPort,
						Type:   intstr.String,
					},
					Path: awssdk.String("/"),
					Matcher: &elbv2model.HealthCheckMatcher{
						HTTPCode: awssdk.String("200-399"),
					},
					Protocol:                elbv2model.ProtocolHTTP,
					IntervalSeconds:         awssdk.Int32(15),
					TimeoutSeconds:          awssdk.Int32(5),
					HealthyThresholdCount:   awssdk.Int32(3),
					UnhealthyThresholdCount: awssdk.Int32(3),
				},
				TargetGroupAttributes: make([]elbv2model.TargetGroupAttribute, 0),
				Tags:                  make(map[string]string),
			},
			expectedTgBindingSpec: elbv2modelk8s.TargetGroupBindingResourceSpec{
				Template: elbv2modelk8s.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "my-svc-ns",
						Name:        "k8s-myrouten-myroute-a44a20bcbf",
						Annotations: make(map[string]string),
						Labels:      make(map[string]string),
					},
					Spec: elbv2modelk8s.TargetGroupBindingSpec{
						TargetType: &ipType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromInt32(80), // TODO - Figure out why this port is added and not the node port.
						},
						IPAddressType:       elbv2api.TargetGroupIPAddressType(elbv2model.IPAddressTypeIPV4),
						VpcID:               "vpc-xxx",
						TargetGroupProtocol: &httpProtocol,
					},
				},
			},
		},
		{
			name:                     "no tg config - ip - alb - add infra annotations / labels",
			tags:                     make(map[string]string),
			lbType:                   elbv2model.LoadBalancerTypeApplication,
			disableRestrictedSGRules: false,
			defaultTargetType:        string(elbv2model.TargetTypeIP),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "my-gw-ns",
					Name:      "my-gw",
				},
				Spec: gwv1.GatewaySpec{
					Infrastructure: &gwv1.GatewayInfrastructure{
						Annotations: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
							"foo": "bar",
						},
						Labels: map[gwv1.LabelKey]gwv1.LabelValue{
							"labelfoo": "labelbar",
						},
					},
				},
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "my-route",
				Namespace: "my-route-ns",
			},
			backend: routeutils.ServiceBackendConfig{
				Service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				ServicePort: &corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			},
			expectedTgSpec: elbv2model.TargetGroupSpec{
				Name:            "k8s-myrouten-myroute-a44a20bcbf",
				TargetType:      elbv2model.TargetTypeIP,
				Port:            awssdk.Int32(80),
				Protocol:        elbv2model.ProtocolHTTP,
				ProtocolVersion: &http1,
				IPAddressType:   elbv2model.TargetGroupIPAddressTypeIPv4,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr.IntOrString{
						StrVal: shared_constants.HealthCheckPortTrafficPort,
						Type:   intstr.String,
					},
					Path: awssdk.String("/"),
					Matcher: &elbv2model.HealthCheckMatcher{
						HTTPCode: awssdk.String("200-399"),
					},
					Protocol:                elbv2model.ProtocolHTTP,
					IntervalSeconds:         awssdk.Int32(15),
					TimeoutSeconds:          awssdk.Int32(5),
					HealthyThresholdCount:   awssdk.Int32(3),
					UnhealthyThresholdCount: awssdk.Int32(3),
				},
				TargetGroupAttributes: make([]elbv2model.TargetGroupAttribute, 0),
				Tags:                  make(map[string]string),
			},
			expectedTgBindingSpec: elbv2modelk8s.TargetGroupBindingResourceSpec{
				Template: elbv2modelk8s.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "k8s-myrouten-myroute-a44a20bcbf",
						Annotations: map[string]string{
							"foo": "bar",
						},
						Labels: map[string]string{
							"labelfoo": "labelbar",
						},
					},
					Spec: elbv2modelk8s.TargetGroupBindingSpec{
						TargetType: &ipType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromInt32(80), // TODO - Figure out why this port is added and not the node port.
						},
						IPAddressType:       elbv2api.TargetGroupIPAddressType(elbv2model.IPAddressTypeIPV4),
						VpcID:               "vpc-xxx",
						TargetGroupProtocol: &httpProtocol,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			tagger := &mockTagHelper{
				tags: tc.tags,
				err:  tc.tagErr,
			}

			builder := newTargetGroupBuilder("my-cluster", "vpc-xxx", tagger, tc.lbType, gateway.NewTargetGroupConfigConstructor(), tc.disableRestrictedSGRules, tc.defaultTargetType, nil)

			out := builder.(*targetGroupBuilderImpl).buildTargetGroupBindingSpec(tc.gateway, nil, tc.expectedTgSpec, nil, tc.backend, nil)

			assert.Equal(t, tc.expectedTgBindingSpec, out)
		})
	}
}

func Test_buildTargetGroupBindingNetworking(t *testing.T) {
	protocolTCP := elbv2api.NetworkingProtocolTCP
	protocolUDP := elbv2api.NetworkingProtocolUDP

	intstr80 := intstr.FromInt32(80)
	intstr85 := intstr.FromInt32(85)
	intstrTrafficPort := intstr.FromString(shared_constants.HealthCheckPortTrafficPort)

	testCases := []struct {
		name                     string
		disableRestrictedSGRules bool

		targetPort       intstr.IntOrString
		healthCheckPort  intstr.IntOrString
		tgProtocol       elbv2model.Protocol
		backendSGIDToken core.StringToken

		expected *elbv2modelk8s.TargetGroupBindingNetworking
	}{
		{
			name:                     "disable restricted sg rules",
			disableRestrictedSGRules: true,
			backendSGIDToken:         core.LiteralStringToken("foo"),
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     nil,
							},
						},
					},
				},
			},
		},
		{
			name:                     "disable restricted sg rules - with udp",
			disableRestrictedSGRules: true,
			backendSGIDToken:         core.LiteralStringToken("foo"),
			tgProtocol:               elbv2model.ProtocolUDP,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     nil,
							},
							{
								Protocol: &protocolUDP,
								Port:     nil,
							},
						},
					},
				},
			},
		},
		{
			name:             "use restricted sg rules - int hc port",
			backendSGIDToken: core.LiteralStringToken("foo"),
			tgProtocol:       elbv2model.ProtocolTCP,
			targetPort:       intstr80,
			healthCheckPort:  intstr80,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
				},
			},
		},
		{
			name:             "use restricted sg rules - int hc port - udp traffic",
			backendSGIDToken: core.LiteralStringToken("foo"),
			tgProtocol:       elbv2model.ProtocolUDP,
			targetPort:       intstr80,
			healthCheckPort:  intstr80,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolUDP,
								Port:     &intstr80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
				},
			},
		},
		{
			name:             "use restricted sg rules - str hc port",
			backendSGIDToken: core.LiteralStringToken("foo"),
			tgProtocol:       elbv2model.ProtocolHTTP,
			targetPort:       intstr80,
			healthCheckPort:  intstrTrafficPort,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
				},
			},
		},
		{
			name:             "use restricted sg rules - str hc port - udp",
			backendSGIDToken: core.LiteralStringToken("foo"),
			tgProtocol:       elbv2model.ProtocolUDP,
			targetPort:       intstr80,
			healthCheckPort:  intstrTrafficPort,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolUDP,
								Port:     &intstr80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
				},
			},
		},
		{
			name:             "use restricted sg rules - diff hc port",
			backendSGIDToken: core.LiteralStringToken("foo"),
			tgProtocol:       elbv2model.ProtocolHTTP,
			targetPort:       intstr80,
			healthCheckPort:  intstr85,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr85,
							},
						},
					},
				},
			},
		},
		{
			name:             "use restricted sg rules - str hc port - udp",
			backendSGIDToken: core.LiteralStringToken("foo"),
			tgProtocol:       elbv2model.ProtocolUDP,
			targetPort:       intstr80,
			healthCheckPort:  intstr85,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolUDP,
								Port:     &intstr80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr85,
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := &targetGroupBuilderImpl{
				disableRestrictedSGRules: tc.disableRestrictedSGRules,
			}

			result := builder.buildTargetGroupBindingNetworking(tc.targetPort, tc.healthCheckPort, tc.tgProtocol, tc.backendSGIDToken)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_buildTargetGroupName(t *testing.T) {
	http2 := elbv2model.ProtocolVersionHTTP2
	clusterName := "foo"
	gwKey := types.NamespacedName{
		Namespace: "my-ns",
		Name:      "my-gw",
	}
	routeKey := types.NamespacedName{
		Namespace: "my-ns",
		Name:      "my-route",
	}
	svcKey := types.NamespacedName{
		Namespace: "my-ns",
		Name:      "my-svc",
	}
	testCases := []struct {
		name             string
		targetGroupProps *elbv2gw.TargetGroupProps
		protocolVersion  *elbv2model.ProtocolVersion
		expected         string
	}{
		{
			name:             "name override",
			targetGroupProps: &elbv2gw.TargetGroupProps{TargetGroupName: awssdk.String("foobaz")},
			expected:         "foobaz",
		},
		{
			name:             "no name in props",
			targetGroupProps: &elbv2gw.TargetGroupProps{},
			expected:         "k8s-myns-myroute-27d98b9190",
		},
		{
			name:     "no props",
			expected: "k8s-myns-myroute-27d98b9190",
		},
		{
			name:            "protocol specified props",
			protocolVersion: &http2,
			expected:        "k8s-myns-myroute-d2bd5deaa7",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{
				clusterName: clusterName,
			}

			result := builder.buildTargetGroupName(tc.targetGroupProps, gwKey, routeKey, routeutils.HTTPRouteKind, svcKey, 80, elbv2model.TargetTypeIP, elbv2model.ProtocolTCP, tc.protocolVersion)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_buildTargetGroupTargetType(t *testing.T) {
	builder := targetGroupBuilderImpl{
		defaultTargetType: elbv2model.TargetTypeIP,
	}

	res := builder.buildTargetGroupTargetType(nil)
	assert.Equal(t, elbv2model.TargetTypeIP, res)

	res = builder.buildTargetGroupTargetType(&elbv2gw.TargetGroupProps{})
	assert.Equal(t, elbv2model.TargetTypeIP, res)

	inst := elbv2gw.TargetTypeInstance
	res = builder.buildTargetGroupTargetType(&elbv2gw.TargetGroupProps{
		TargetType: &inst,
	})
	assert.Equal(t, elbv2model.TargetTypeInstance, res)
}

func Test_buildTargetGroupIPAddressType(t *testing.T) {
	testCases := []struct {
		name                      string
		svc                       *corev1.Service
		loadBalancerIPAddressType elbv2model.IPAddressType
		expectErr                 bool
		expected                  elbv2model.TargetGroupIPAddressType
	}{
		{
			name:                      "no ip families",
			svc:                       &corev1.Service{},
			loadBalancerIPAddressType: elbv2model.IPAddressTypeIPV4,
			expected:                  elbv2model.TargetGroupIPAddressTypeIPv4,
		},
		{
			name: "ipv4 family",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					IPFamilies: []corev1.IPFamily{
						corev1.IPv4Protocol,
					},
				},
			},
			loadBalancerIPAddressType: elbv2model.IPAddressTypeIPV4,
			expected:                  elbv2model.TargetGroupIPAddressTypeIPv4,
		},
		{
			name: "ipv6 family",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					IPFamilies: []corev1.IPFamily{
						corev1.IPv6Protocol,
					},
				},
			},
			loadBalancerIPAddressType: elbv2model.IPAddressTypeDualStack,
			expected:                  elbv2model.TargetGroupIPAddressTypeIPv6,
		},
		{
			name: "ipv6 family - dual stack no ipv4",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					IPFamilies: []corev1.IPFamily{
						corev1.IPv6Protocol,
					},
				},
			},
			loadBalancerIPAddressType: elbv2model.IPAddressTypeDualStackWithoutPublicIPV4,
			expected:                  elbv2model.TargetGroupIPAddressTypeIPv6,
		},
		{
			name: "ipv6 family - bad lb type",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					IPFamilies: []corev1.IPFamily{
						corev1.IPv6Protocol,
					},
				},
			},
			loadBalancerIPAddressType: elbv2model.IPAddressTypeIPV4,
			expectErr:                 true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{}
			res, err := builder.buildTargetGroupIPAddressType(tc.svc, tc.loadBalancerIPAddressType)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, res)

		})
	}
}

func Test_buildTargetGroupPort(t *testing.T) {
	testCases := []struct {
		name       string
		targetType elbv2model.TargetType
		svcPort    corev1.ServicePort
		expected   int32
	}{
		{
			name: "instance",
			svcPort: corev1.ServicePort{
				NodePort: 8080,
			},
			targetType: elbv2model.TargetTypeInstance,
			expected:   8080,
		},
		{
			name:       "instance - no node port",
			svcPort:    corev1.ServicePort{},
			targetType: elbv2model.TargetTypeInstance,
			expected:   0,
		},
		{
			name: "ip",
			svcPort: corev1.ServicePort{
				NodePort:   8080,
				TargetPort: intstr.FromInt32(80),
			},
			targetType: elbv2model.TargetTypeIP,
			expected:   80,
		},
		{
			name: "ip - str port",
			svcPort: corev1.ServicePort{
				NodePort:   8080,
				TargetPort: intstr.FromString("foo"),
			},
			targetType: elbv2model.TargetTypeIP,
			expected:   1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{}
			res := builder.buildTargetGroupPort(tc.targetType, tc.svcPort)
			assert.Equal(t, tc.expected, res)

		})
	}
}

func Test_buildTargetGroupProtocol(t *testing.T) {
	testCases := []struct {
		name             string
		lbType           elbv2model.LoadBalancerType
		targetGroupProps *elbv2gw.TargetGroupProps
		route            routeutils.RouteDescriptor
		expected         elbv2model.Protocol
		expectErr        bool
	}{
		{
			name:   "alb - auto detect - http",
			lbType: elbv2model.LoadBalancerTypeApplication,
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolHTTP,
		},
		{
			name:   "alb - auto detect - grpc",
			lbType: elbv2model.LoadBalancerTypeApplication,
			route: &routeutils.MockRoute{
				Kind:      routeutils.GRPCRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolHTTP,
		},
		{
			name:   "alb - auto detect - tls",
			lbType: elbv2model.LoadBalancerTypeApplication,
			route: &routeutils.MockRoute{
				Kind:      routeutils.TLSRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolHTTPS,
		},
		{
			name:   "nlb - auto detect - tcp",
			lbType: elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTCP,
		},
		{
			name:   "alb - auto detect - udp",
			lbType: elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.UDPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolUDP,
		},
		{
			name:   "nlb - auto detect - tls",
			lbType: elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.TLSRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTLS,
		},
		{
			name:   "alb - specified - http",
			lbType: elbv2model.LoadBalancerTypeApplication,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				Protocol: protocolPtr(elbv2gw.ProtocolHTTP),
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolHTTP,
		},
		{
			name:   "alb - specified - https",
			lbType: elbv2model.LoadBalancerTypeApplication,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				Protocol: protocolPtr(elbv2gw.ProtocolHTTPS),
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolHTTPS,
		},
		{
			name:   "alb - specified - invalid protocol",
			lbType: elbv2model.LoadBalancerTypeApplication,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				Protocol: protocolPtr(elbv2gw.ProtocolTCP),
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expectErr: true,
		},
		{
			name:   "nlb - auto detect - tcp",
			lbType: elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTCP,
		},
		{
			name:   "alb - auto detect - udp",
			lbType: elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.UDPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolUDP,
		},
		{
			name:   "nlb - auto detect - tls",
			lbType: elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.TLSRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTLS,
		},
		{
			name:   "nlb - specified - tcp protocol",
			lbType: elbv2model.LoadBalancerTypeNetwork,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				Protocol: protocolPtr(elbv2gw.ProtocolTCP),
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTCP,
		},
		{
			name:   "nlb - specified - udp protocol",
			lbType: elbv2model.LoadBalancerTypeNetwork,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				Protocol: protocolPtr(elbv2gw.ProtocolUDP),
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolUDP,
		},
		{
			name:   "nlb - specified - tcpudp protocol",
			lbType: elbv2model.LoadBalancerTypeNetwork,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				Protocol: protocolPtr(elbv2gw.ProtocolTCP_UDP),
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTCP_UDP,
		},
		{
			name:   "nlb - specified - tls protocol",
			lbType: elbv2model.LoadBalancerTypeNetwork,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				Protocol: protocolPtr(elbv2gw.ProtocolTLS),
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTLS,
		},
		{
			name:   "nlb - specified - invalid protocol",
			lbType: elbv2model.LoadBalancerTypeNetwork,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				Protocol: protocolPtr(elbv2gw.ProtocolHTTPS),
			},
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{
				loadBalancerType: tc.lbType,
			}
			res, err := builder.buildTargetGroupProtocol(tc.targetGroupProps, tc.route)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func Test_buildTargetGroupProtocolVersion(t *testing.T) {
	http2Gw := elbv2gw.ProtocolVersionHTTP2
	http2Elb := elbv2model.ProtocolVersionHTTP2
	http1Elb := elbv2model.ProtocolVersionHTTP1
	grpcElb := elbv2model.ProtocolVersionGRPC
	testCases := []struct {
		name             string
		loadBalancerType elbv2model.LoadBalancerType
		route            routeutils.RouteDescriptor
		targetGroupProps *elbv2gw.TargetGroupProps
		expected         *elbv2model.ProtocolVersion
	}{
		{
			name:             "nlb - no props",
			loadBalancerType: elbv2model.LoadBalancerTypeNetwork,
			route:            &routeutils.MockRoute{Kind: routeutils.TCPRouteKind},
		},
		{
			name:             "nlb - with props",
			loadBalancerType: elbv2model.LoadBalancerTypeNetwork,
			route:            &routeutils.MockRoute{Kind: routeutils.TCPRouteKind},
			targetGroupProps: &elbv2gw.TargetGroupProps{
				ProtocolVersion: &http2Gw,
			},
		},
		{
			name:             "alb - no props",
			route:            &routeutils.MockRoute{Kind: routeutils.HTTPRouteKind},
			loadBalancerType: elbv2model.LoadBalancerTypeApplication,
			expected:         &http1Elb,
		},
		{
			name:             "alb - no props - grpc",
			route:            &routeutils.MockRoute{Kind: routeutils.GRPCRouteKind},
			loadBalancerType: elbv2model.LoadBalancerTypeApplication,
			expected:         &grpcElb,
		},
		{
			name:             "alb - with props",
			route:            &routeutils.MockRoute{Kind: routeutils.HTTPRouteKind},
			loadBalancerType: elbv2model.LoadBalancerTypeApplication,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				ProtocolVersion: &http2Gw,
			},
			expected: &http2Elb,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{
				loadBalancerType: tc.loadBalancerType,
			}
			res := builder.buildTargetGroupProtocolVersion(tc.targetGroupProps, tc.route)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func Test_buildTargetGroupHealthCheckPort(t *testing.T) {
	testCases := []struct {
		name                                    string
		isServiceExternalTrafficPolicyTypeLocal bool
		targetGroupProps                        *elbv2gw.TargetGroupProps
		targetType                              elbv2model.TargetType
		svc                                     *corev1.Service
		expected                                intstr.IntOrString
		expectErr                               bool
	}{
		{
			name:                                    "nil props",
			isServiceExternalTrafficPolicyTypeLocal: false,
			expected:                                intstr.FromString(shared_constants.HealthCheckPortTrafficPort),
		},
		{
			name:                                    "nil hc props",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetGroupProps:                        &elbv2gw.TargetGroupProps{},
			expected:                                intstr.FromString(shared_constants.HealthCheckPortTrafficPort),
		},
		{
			name:                                    "nil hc port",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{},
			},
			expected: intstr.FromString(shared_constants.HealthCheckPortTrafficPort),
		},
		{
			name:                                    "explicit is use traffic port hc port",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String(shared_constants.HealthCheckPortTrafficPort),
				},
			},
			expected: intstr.FromString(shared_constants.HealthCheckPortTrafficPort),
		},
		{
			name:                                    "explicit port",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("80"),
				},
			},
			expected: intstr.FromInt32(80),
		},
		{
			name:                                    "resolve str port",
			isServiceExternalTrafficPolicyTypeLocal: false,
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "foo",
							TargetPort: intstr.FromInt32(80),
						},
					},
				},
			},
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("foo"),
				},
			},
			expected: intstr.FromInt32(80),
		},
		{
			name:                                    "resolve str port - instance",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetType:                              elbv2model.TargetTypeInstance,
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "foo",
							TargetPort: intstr.FromInt32(80),
							NodePort:   1000,
						},
					},
				},
			},
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("foo"),
				},
			},
			expected: intstr.FromInt32(1000),
		},
		{
			name:                                    "resolve str port - resolves to other str port (error)",
			isServiceExternalTrafficPolicyTypeLocal: false,
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "foo",
							TargetPort: intstr.FromString("bar"),
							NodePort:   1000,
						},
					},
				},
			},
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("foo"),
				},
			},
			expectErr: true,
		},
		{
			name:                                    "resolve str port - resolves to other str port but instance mode",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetType:                              elbv2model.TargetTypeInstance,
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "foo",
							TargetPort: intstr.FromString("bar"),
							NodePort:   1000,
						},
					},
				},
			},
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("foo"),
				},
			},
			expected: intstr.FromInt32(1000),
		},
		{
			name:                                    "resolve str port - cant find configured port",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetType:                              elbv2model.TargetTypeInstance,
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "baz",
							TargetPort: intstr.FromString("bar"),
							NodePort:   1000,
						},
					},
				},
			},
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("foo"),
				},
			},
			expectErr: true,
		},
		{
			name:                                    "with ExternalTrafficPolicyTypeLocal and HealthCheckNodePort specified",
			isServiceExternalTrafficPolicyTypeLocal: true,
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					HealthCheckNodePort:   32000,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
				},
			},
			expected: intstr.FromInt32(32000),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{}
			res, err := builder.buildTargetGroupHealthCheckPort(tc.targetGroupProps, tc.targetType, tc.svc, tc.isServiceExternalTrafficPolicyTypeLocal)
			if tc.expectErr {
				assert.Error(t, err, res)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func Test_buildTargetGroupHealthCheckProtocol(t *testing.T) {
	testCases := []struct {
		name             string
		lbType           elbv2model.LoadBalancerType
		targetGroupProps *elbv2gw.TargetGroupProps
		tgProtocol       elbv2model.Protocol
		expected         elbv2model.Protocol
	}{
		{
			name:       "nlb - default",
			lbType:     elbv2model.LoadBalancerTypeNetwork,
			tgProtocol: elbv2model.ProtocolUDP,
			expected:   elbv2model.ProtocolTCP,
		},
		{
			name:       "alb - default",
			lbType:     elbv2model.LoadBalancerTypeApplication,
			tgProtocol: elbv2model.ProtocolHTTP,
			expected:   elbv2model.ProtocolHTTP,
		},
		{
			name:   "specified http",
			lbType: elbv2model.LoadBalancerTypeApplication,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckProtocol: (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.ProtocolHTTP))),
				},
			},
			tgProtocol: elbv2model.ProtocolHTTP,
			expected:   elbv2model.ProtocolHTTP,
		},
		{
			name:   "specified https",
			lbType: elbv2model.LoadBalancerTypeApplication,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckProtocol: (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.ProtocolHTTPS))),
				},
			},
			tgProtocol: elbv2model.ProtocolHTTP,
			expected:   elbv2model.ProtocolHTTPS,
		},
		{
			name:   "specified tcp",
			lbType: elbv2model.LoadBalancerTypeApplication,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckProtocol: (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.ProtocolTCP))),
				},
			},
			tgProtocol: elbv2model.ProtocolTCP,
			expected:   elbv2model.ProtocolTCP,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{
				loadBalancerType: tc.lbType,
			}

			res := builder.buildTargetGroupHealthCheckProtocol(tc.targetGroupProps, tc.tgProtocol, false)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func Test_buildTargetGroupHealthCheckPath(t *testing.T) {
	httpDefaultPath := "httpDefault"
	grpcDefaultPath := "grpcDefault"
	testCases := []struct {
		name              string
		targetGroupProps  *elbv2gw.TargetGroupProps
		tgProtocolVersion *elbv2model.ProtocolVersion
		hcProtocol        elbv2model.Protocol
		expected          *string
	}{
		{
			name: "path specified",
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPath: awssdk.String("foo"),
				},
			},
			expected: awssdk.String("foo"),
		},
		{
			name:       "default - tcp",
			hcProtocol: elbv2model.ProtocolTCP,
		},
		{
			name:             "default - tcp - with cfg",
			hcProtocol:       elbv2model.ProtocolTCP,
			targetGroupProps: &elbv2gw.TargetGroupProps{},
		},
		{
			name:       "default - http",
			hcProtocol: elbv2model.ProtocolHTTP,
			expected:   &httpDefaultPath,
		},
		{
			name:             "default - http - with cfg",
			hcProtocol:       elbv2model.ProtocolHTTP,
			expected:         &httpDefaultPath,
			targetGroupProps: &elbv2gw.TargetGroupProps{},
		},
		{
			name:              "default - grpc",
			hcProtocol:        elbv2model.ProtocolHTTP,
			tgProtocolVersion: (*elbv2model.ProtocolVersion)(awssdk.String(string(elbv2model.ProtocolVersionGRPC))),
			expected:          &grpcDefaultPath,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{
				defaultHealthCheckPathHTTP: httpDefaultPath,
				defaultHealthCheckPathGRPC: grpcDefaultPath,
			}

			res := builder.buildTargetGroupHealthCheckPath(tc.targetGroupProps, tc.tgProtocolVersion, tc.hcProtocol, false)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func Test_buildTargetGroupHealthCheckMatcher(t *testing.T) {
	httpDefaultMatcher := "httpMatcher"
	grpcDefaultMatcher := "grpcMatcher"
	testCases := []struct {
		name              string
		targetGroupProps  *elbv2gw.TargetGroupProps
		tgProtocolVersion *elbv2model.ProtocolVersion
		hcProtocol        elbv2model.Protocol
		expected          *elbv2model.HealthCheckMatcher
	}{
		{
			name:       "default - tcp",
			hcProtocol: elbv2model.ProtocolTCP,
		},
		{
			name: "specified - grpc",
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					Matcher: &elbv2gw.HealthCheckMatcher{
						GRPCCode: awssdk.String("foo"),
					},
				},
			},
			hcProtocol:        elbv2model.ProtocolHTTP,
			tgProtocolVersion: (*elbv2model.ProtocolVersion)(awssdk.String(string(elbv2model.ProtocolVersionGRPC))),
			expected: &elbv2model.HealthCheckMatcher{
				GRPCCode: awssdk.String("foo"),
			},
		},
		{
			name: "specified - http",
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					Matcher: &elbv2gw.HealthCheckMatcher{
						HTTPCode: awssdk.String("foo"),
					},
				},
			},
			hcProtocol: elbv2model.ProtocolHTTP,
			expected: &elbv2model.HealthCheckMatcher{
				HTTPCode: awssdk.String("foo"),
			},
		},
		{
			name:              "default - grpc",
			hcProtocol:        elbv2model.ProtocolHTTP,
			tgProtocolVersion: (*elbv2model.ProtocolVersion)(awssdk.String(string(elbv2model.ProtocolVersionGRPC))),
			expected: &elbv2model.HealthCheckMatcher{
				GRPCCode: &grpcDefaultMatcher,
			},
		},
		{
			name:              "default - http1",
			hcProtocol:        elbv2model.ProtocolHTTP,
			tgProtocolVersion: (*elbv2model.ProtocolVersion)(awssdk.String(string(elbv2model.ProtocolVersionHTTP1))),
			expected: &elbv2model.HealthCheckMatcher{
				HTTPCode: &httpDefaultMatcher,
			},
		},
		{
			name:              "default - no protocol version",
			hcProtocol:        elbv2model.ProtocolHTTP,
			tgProtocolVersion: (*elbv2model.ProtocolVersion)(awssdk.String(string(elbv2model.ProtocolVersionHTTP1))),
			expected: &elbv2model.HealthCheckMatcher{
				HTTPCode: &httpDefaultMatcher,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{
				defaultHealthCheckMatcherHTTPCode: httpDefaultMatcher,
				defaultHealthCheckMatcherGRPCCode: grpcDefaultMatcher,
			}

			res := builder.buildTargetGroupHealthCheckMatcher(tc.targetGroupProps, tc.tgProtocolVersion, tc.hcProtocol)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func Test_basicHealthCheckParams(t *testing.T) {
	builder := targetGroupBuilderImpl{
		defaultHealthCheckInterval:                1,
		defaultHealthCheckTimeout:                 2,
		defaultHealthyThresholdCount:              3,
		defaultHealthCheckUnhealthyThresholdCount: 4,
	}

	defaultProps := []*elbv2gw.TargetGroupProps{
		nil,
		{},
		{
			HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{},
		},
	}

	for _, prop := range defaultProps {
		assert.Equal(t, int32(1), builder.buildTargetGroupHealthCheckIntervalSeconds(prop, false))
		assert.Equal(t, int32(2), builder.buildTargetGroupHealthCheckTimeoutSeconds(prop, false))
		assert.Equal(t, int32(3), builder.buildTargetGroupHealthCheckHealthyThresholdCount(prop, false))
		assert.Equal(t, int32(4), builder.buildTargetGroupHealthCheckUnhealthyThresholdCount(prop, false))
	}

	filledInProps := &elbv2gw.TargetGroupProps{
		HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
			HealthyThresholdCount:   awssdk.Int32(30),
			HealthCheckInterval:     awssdk.Int32(10),
			HealthCheckPath:         nil,
			HealthCheckPort:         nil,
			HealthCheckProtocol:     nil,
			HealthCheckTimeout:      awssdk.Int32(20),
			UnhealthyThresholdCount: awssdk.Int32(40),
			Matcher:                 nil,
		}}

	assert.Equal(t, int32(10), builder.buildTargetGroupHealthCheckIntervalSeconds(filledInProps, false))
	assert.Equal(t, int32(20), builder.buildTargetGroupHealthCheckTimeoutSeconds(filledInProps, false))
	assert.Equal(t, int32(30), builder.buildTargetGroupHealthCheckHealthyThresholdCount(filledInProps, false))
	assert.Equal(t, int32(40), builder.buildTargetGroupHealthCheckUnhealthyThresholdCount(filledInProps, false))
}

func Test_targetGroupAttributes(t *testing.T) {
	testCases := []struct {
		name     string
		props    *elbv2gw.TargetGroupProps
		expected []elbv2model.TargetGroupAttribute
	}{
		{
			name:     "no props - nil",
			expected: make([]elbv2model.TargetGroupAttribute, 0),
		},
		{
			name:     "no props",
			props:    &elbv2gw.TargetGroupProps{},
			expected: make([]elbv2model.TargetGroupAttribute, 0),
		},
		{
			name: "some props",
			props: &elbv2gw.TargetGroupProps{
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "foo",
						Value: "bar",
					},
					{
						Key:   "foo1",
						Value: "bar1",
					},
					{
						Key:   "foo2",
						Value: "bar2",
					},
				},
			},
			expected: []elbv2model.TargetGroupAttribute{
				{
					Key:   "foo",
					Value: "bar",
				},
				{
					Key:   "foo1",
					Value: "bar1",
				},
				{
					Key:   "foo2",
					Value: "bar2",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{}

			res := builder.convertMapToAttributes(builder.buildTargetGroupAttributes(tc.props))
			assert.ElementsMatch(t, tc.expected, res)
		})
	}
}

func Test_buildTargetGroupBindingNodeSelector(t *testing.T) {
	builder := targetGroupBuilderImpl{}

	res := builder.buildTargetGroupBindingNodeSelector(nil, elbv2model.TargetTypeInstance)
	assert.Nil(t, res)

	propWithSelector := &elbv2gw.TargetGroupProps{
		NodeSelector: &metav1.LabelSelector{},
	}

	res = builder.buildTargetGroupBindingNodeSelector(propWithSelector, elbv2model.TargetTypeIP)
	assert.Nil(t, res)

	assert.NotNil(t, builder.buildTargetGroupBindingNodeSelector(propWithSelector, elbv2model.TargetTypeInstance))
}

func Test_buildTargetGroupBindingMultiClusterFlag(t *testing.T) {
	builder := targetGroupBuilderImpl{}

	assert.False(t, builder.buildTargetGroupBindingMultiClusterFlag(nil))

	props := &elbv2gw.TargetGroupProps{
		EnableMultiCluster: awssdk.Bool(false),
	}

	assert.False(t, builder.buildTargetGroupBindingMultiClusterFlag(props))
	props.EnableMultiCluster = awssdk.Bool(true)
	assert.True(t, builder.buildTargetGroupBindingMultiClusterFlag(props))
}

func protocolPtr(protocol elbv2gw.Protocol) *elbv2gw.Protocol {
	return &protocol
}
