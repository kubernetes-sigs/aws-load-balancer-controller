package model

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2modelk8s "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_utils"
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
		backend                  *routeutils.ServiceBackendConfig
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
			backend: routeutils.NewServiceBackendConfig(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				nil,
				&corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				}),
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
			backend: routeutils.NewServiceBackendConfig(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				nil,
				&corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				}),
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
			backend: routeutils.NewServiceBackendConfig(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				nil,
				&corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				}),
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
			backend: routeutils.NewServiceBackendConfig(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				nil,
				&corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			),
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
			backend: routeutils.NewServiceBackendConfig(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				nil,
				&corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
				},
			),
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			tagger := &mockTagHelper{
				tags: tc.tags,
				err:  tc.tagErr,
			}

			builder := newTargetGroupBuilder("my-cluster", "vpc-xxx", tagger, tc.lbType, &mockTargetGroupBindingNetworkingBuilder{}, gateway.NewTargetGroupConfigConstructor(), tc.defaultTargetType, nil)

			out, err := builder.(*targetGroupBuilderImpl).buildTargetGroupSpec(tc.gateway, tc.route, elbv2model.ProtocolHTTP, elbv2model.IPAddressTypeIPV4, tc.backend, nil)
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
		name                  string
		tags                  map[string]string
		lbType                elbv2model.LoadBalancerType
		defaultTargetType     string
		gateway               *gwv1.Gateway
		route                 *routeutils.MockRoute
		backend               *routeutils.ServiceBackendConfig
		tagErr                error
		expectErr             bool
		expectedTgSpec        elbv2model.TargetGroupSpec
		expectedTgBindingSpec elbv2modelk8s.TargetGroupBindingResourceSpec
	}{
		{
			name:              "no tg config - instance - nlb",
			tags:              make(map[string]string),
			lbType:            elbv2model.LoadBalancerTypeNetwork,
			defaultTargetType: string(elbv2model.TargetTypeInstance),
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
			backend: routeutils.NewServiceBackendConfig(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				nil,
				&corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			),
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
			name:              "no tg config - instance - alb",
			tags:              make(map[string]string),
			lbType:            elbv2model.LoadBalancerTypeApplication,
			defaultTargetType: string(elbv2model.TargetTypeInstance),
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
			backend: routeutils.NewServiceBackendConfig(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				nil,
				&corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			),
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
			name:              "no tg config - ip - nlb",
			tags:              make(map[string]string),
			lbType:            elbv2model.LoadBalancerTypeNetwork,
			defaultTargetType: string(elbv2model.TargetTypeIP),
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
			backend: routeutils.NewServiceBackendConfig(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				nil,
				&corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			),
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
			name:              "no tg config - ip - alb",
			tags:              make(map[string]string),
			lbType:            elbv2model.LoadBalancerTypeApplication,
			defaultTargetType: string(elbv2model.TargetTypeIP),
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
			backend: routeutils.NewServiceBackendConfig(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				nil,
				&corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			),
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
			name:              "no tg config - ip - alb - add infra annotations / labels",
			tags:              make(map[string]string),
			lbType:            elbv2model.LoadBalancerTypeApplication,
			defaultTargetType: string(elbv2model.TargetTypeIP),
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
			backend: routeutils.NewServiceBackendConfig(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "my-svc",
					},
				},
				nil,
				&corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
					NodePort: 8080,
				},
			),
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

			builder := newTargetGroupBuilder("my-cluster", "vpc-xxx", tagger, tc.lbType, &mockTargetGroupBindingNetworkingBuilder{}, gateway.NewTargetGroupConfigConstructor(), tc.defaultTargetType, nil)

			out, err := builder.(*targetGroupBuilderImpl).buildTargetGroupBindingSpec(tc.gateway, nil, tc.expectedTgSpec, nil, *tc.backend)

			assert.Equal(t, tc.expectedTgBindingSpec, out)
			assert.NoError(t, err)
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
		{
			name:             "with target control port",
			targetGroupProps: &elbv2gw.TargetGroupProps{TargetControlPort: awssdk.Int32(3000)},
			expected:         "k8s-myns-myroute-54e81471e4",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{
				clusterName: clusterName,
			}

			var targetControlPort *int32
			if tc.targetGroupProps != nil {
				targetControlPort = tc.targetGroupProps.TargetControlPort
			}

			result := builder.buildTargetGroupName(tc.targetGroupProps, gwKey, routeKey, routeutils.HTTPRouteKind, svcKey, 80, elbv2model.TargetTypeIP, elbv2model.ProtocolTCP, tc.protocolVersion, targetControlPort)
			assert.Equal(t, tc.expected, result)
		})
	}
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
			res, err := builder.buildTargetGroupIPAddressType(routeutils.NewServiceBackendConfig(tc.svc, nil, nil), tc.loadBalancerIPAddressType)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, res)

		})
	}
}

func Test_buildTargetGroupProtocol(t *testing.T) {
	testCases := []struct {
		name             string
		listenerProtocol elbv2model.Protocol
		lbType           elbv2model.LoadBalancerType
		targetGroupProps *elbv2gw.TargetGroupProps
		route            routeutils.RouteDescriptor
		expected         elbv2model.Protocol
		expectErr        bool
	}{
		{
			name:             "alb - auto detect - http",
			listenerProtocol: elbv2model.ProtocolHTTPS,
			lbType:           elbv2model.LoadBalancerTypeApplication,
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolHTTP,
		},
		{
			name:             "alb - auto detect - grpc",
			listenerProtocol: elbv2model.ProtocolHTTPS,
			lbType:           elbv2model.LoadBalancerTypeApplication,
			route: &routeutils.MockRoute{
				Kind:      routeutils.GRPCRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolHTTP,
		},
		{
			name:             "alb - auto detect - tls",
			listenerProtocol: elbv2model.ProtocolHTTPS,
			lbType:           elbv2model.LoadBalancerTypeApplication,
			route: &routeutils.MockRoute{
				Kind:      routeutils.TLSRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolHTTPS,
		},
		{
			name:             "nlb - auto detect - tcp",
			listenerProtocol: elbv2model.ProtocolTLS,
			lbType:           elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTCP,
		},
		{
			name:             "alb - auto detect - udp",
			listenerProtocol: elbv2model.ProtocolUDP,
			lbType:           elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.UDPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolUDP,
		},
		{
			name:             "nlb - auto detect - tls",
			listenerProtocol: elbv2model.ProtocolTCP,
			lbType:           elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.TLSRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTLS,
		},
		{
			name:             "alb - specified - http",
			listenerProtocol: elbv2model.ProtocolHTTP,
			lbType:           elbv2model.LoadBalancerTypeApplication,
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
			name:             "alb - specified - https",
			listenerProtocol: elbv2model.ProtocolHTTPS,
			lbType:           elbv2model.LoadBalancerTypeApplication,
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
			name:             "alb - specified - invalid protocol",
			listenerProtocol: elbv2model.ProtocolHTTPS,
			lbType:           elbv2model.LoadBalancerTypeApplication,
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
			name:             "nlb - auto detect - tcp",
			listenerProtocol: elbv2model.ProtocolTCP,
			lbType:           elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTCP,
		},
		{
			name:             "nlb - auto detect - tcp_udp",
			listenerProtocol: elbv2model.ProtocolTCP_UDP,
			lbType:           elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.UDPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTCP_UDP,
		},
		{
			name:             "nlb - auto detect - tls",
			listenerProtocol: elbv2model.ProtocolTLS,
			lbType:           elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.TLSRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTLS,
		},
		{
			name:             "nlb - specified - tcp protocol",
			listenerProtocol: elbv2model.ProtocolTCP,
			lbType:           elbv2model.LoadBalancerTypeNetwork,
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
			name:             "nlb - specified - udp protocol",
			listenerProtocol: elbv2model.ProtocolUDP,
			lbType:           elbv2model.LoadBalancerTypeNetwork,
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
			name:             "nlb - specified - tcpudp protocol",
			listenerProtocol: elbv2model.ProtocolTCP,
			lbType:           elbv2model.LoadBalancerTypeNetwork,
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
			name:             "nlb - specified - tls protocol",
			listenerProtocol: elbv2model.ProtocolTCP,
			lbType:           elbv2model.LoadBalancerTypeNetwork,
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
			name:             "nlb - specified - invalid protocol",
			listenerProtocol: elbv2model.ProtocolTCP,
			lbType:           elbv2model.LoadBalancerTypeNetwork,
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
		{
			name:             "nlb - tcp_udp listener",
			listenerProtocol: elbv2model.ProtocolTCP_UDP,
			lbType:           elbv2model.LoadBalancerTypeNetwork,
			route: &routeutils.MockRoute{
				Kind:      routeutils.TCPRouteKind,
				Name:      "r1",
				Namespace: "ns",
			},
			expected: elbv2model.ProtocolTCP_UDP,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{
				loadBalancerType: tc.lbType,
			}
			res, err := builder.buildTargetGroupProtocol(tc.targetGroupProps, tc.route, tc.listenerProtocol)
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
		tgConfigurator   routeutils.TargetGroupConfigurator
		listenerProtocol elbv2model.Protocol
		route            routeutils.RouteDescriptor
		targetGroupProps *elbv2gw.TargetGroupProps
		expected         *elbv2model.ProtocolVersion
	}{
		{
			name:             "nlb - no props",
			loadBalancerType: elbv2model.LoadBalancerTypeNetwork,
			listenerProtocol: elbv2model.ProtocolTCP,
			tgConfigurator: routeutils.NewServiceBackendConfig(nil, nil, &corev1.ServicePort{
				Protocol: corev1.ProtocolTCP,
				Port:     80,
				TargetPort: intstr.IntOrString{
					IntVal: 80,
					Type:   intstr.Int,
				},
			}),
			route: &routeutils.MockRoute{Kind: routeutils.TCPRouteKind},
		},
		{
			name:             "nlb - with props",
			loadBalancerType: elbv2model.LoadBalancerTypeNetwork,
			listenerProtocol: elbv2model.ProtocolTCP,
			tgConfigurator: routeutils.NewServiceBackendConfig(nil, nil, &corev1.ServicePort{
				Protocol: corev1.ProtocolTCP,
				Port:     80,
				TargetPort: intstr.IntOrString{
					IntVal: 80,
					Type:   intstr.Int,
				},
			}),
			route: &routeutils.MockRoute{Kind: routeutils.TCPRouteKind},
			targetGroupProps: &elbv2gw.TargetGroupProps{
				ProtocolVersion: &http2Gw,
			},
		},
		{
			name:             "alb - no props",
			listenerProtocol: elbv2model.ProtocolHTTPS,
			tgConfigurator: routeutils.NewServiceBackendConfig(nil, nil, &corev1.ServicePort{
				Protocol: corev1.ProtocolTCP,
				Port:     80,
				TargetPort: intstr.IntOrString{
					IntVal: 80,
					Type:   intstr.Int,
				},
			}),
			route:            &routeutils.MockRoute{Kind: routeutils.HTTPRouteKind},
			loadBalancerType: elbv2model.LoadBalancerTypeApplication,
			expected:         &http1Elb,
		},
		{
			name:             "alb - no props - grpc",
			listenerProtocol: elbv2model.ProtocolHTTPS,
			tgConfigurator: routeutils.NewServiceBackendConfig(nil, nil, &corev1.ServicePort{
				Protocol: corev1.ProtocolTCP,
				Port:     80,
				TargetPort: intstr.IntOrString{
					IntVal: 80,
					Type:   intstr.Int,
				},
			}),
			route:            &routeutils.MockRoute{Kind: routeutils.GRPCRouteKind},
			loadBalancerType: elbv2model.LoadBalancerTypeApplication,
			expected:         &grpcElb,
		},
		{
			name:             "alb - with props",
			listenerProtocol: elbv2model.ProtocolHTTPS,
			tgConfigurator: routeutils.NewServiceBackendConfig(nil, nil, &corev1.ServicePort{
				Protocol: corev1.ProtocolTCP,
				Port:     80,
				TargetPort: intstr.IntOrString{
					IntVal: 80,
					Type:   intstr.Int,
				},
			}),
			route:            &routeutils.MockRoute{Kind: routeutils.HTTPRouteKind},
			loadBalancerType: elbv2model.LoadBalancerTypeApplication,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				ProtocolVersion: &http2Gw,
			},
			expected: &http2Elb,
		},
		{
			name:             "alb - with props - http protocol",
			listenerProtocol: elbv2model.ProtocolHTTP,
			tgConfigurator: routeutils.NewServiceBackendConfig(nil, nil, &corev1.ServicePort{
				Protocol: corev1.ProtocolTCP,
				Port:     80,
				TargetPort: intstr.IntOrString{
					IntVal: 80,
					Type:   intstr.Int,
				},
			}),
			route:            &routeutils.MockRoute{Kind: routeutils.HTTPRouteKind},
			loadBalancerType: elbv2model.LoadBalancerTypeApplication,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				ProtocolVersion: &http2Gw,
			},
			expected: &http1Elb,
		},
		{
			name:             "alb - pv found on svc port - http listener",
			listenerProtocol: elbv2model.ProtocolHTTP,
			tgConfigurator: routeutils.NewServiceBackendConfig(nil, nil, &corev1.ServicePort{
				Protocol:    corev1.ProtocolTCP,
				Port:        80,
				AppProtocol: awssdk.String("kubernetes.io/h2c"),
				TargetPort: intstr.IntOrString{
					IntVal: 80,
					Type:   intstr.Int,
				},
			}),
			route:            &routeutils.MockRoute{Kind: routeutils.HTTPRouteKind},
			loadBalancerType: elbv2model.LoadBalancerTypeApplication,
			expected:         &http1Elb,
		},
		{
			name:             "alb - pv found on svc port - https listener",
			listenerProtocol: elbv2model.ProtocolHTTPS,
			tgConfigurator: routeutils.NewServiceBackendConfig(nil, nil, &corev1.ServicePort{
				Protocol:    corev1.ProtocolTCP,
				Port:        80,
				AppProtocol: awssdk.String("kubernetes.io/h2c"),
				TargetPort: intstr.IntOrString{
					IntVal: 80,
					Type:   intstr.Int,
				},
			}),
			route:            &routeutils.MockRoute{Kind: routeutils.HTTPRouteKind},
			loadBalancerType: elbv2model.LoadBalancerTypeApplication,
			expected:         &http2Elb,
		},
		{
			name:             "alb - unknown pv found on svc port - https listener",
			listenerProtocol: elbv2model.ProtocolHTTPS,
			tgConfigurator: routeutils.NewServiceBackendConfig(nil, nil, &corev1.ServicePort{
				Protocol:    corev1.ProtocolTCP,
				Port:        80,
				AppProtocol: awssdk.String("foo"),
				TargetPort: intstr.IntOrString{
					IntVal: 80,
					Type:   intstr.Int,
				},
			}),
			route:            &routeutils.MockRoute{Kind: routeutils.HTTPRouteKind},
			loadBalancerType: elbv2model.LoadBalancerTypeApplication,
			expected:         &http1Elb,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{
				loadBalancerType: tc.loadBalancerType,
			}
			res := builder.buildTargetGroupProtocolVersion(tc.targetGroupProps, tc.tgConfigurator, tc.listenerProtocol, tc.route)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func Test_buildTargetGroupHealthCheckProtocol(t *testing.T) {
	testCases := []struct {
		name             string
		targetType       elbv2model.TargetType
		lbType           elbv2model.LoadBalancerType
		targetGroupProps *elbv2gw.TargetGroupProps
		tgProtocol       elbv2model.Protocol
		expected         elbv2model.Protocol
	}{
		{
			name:       "nlb - default",
			targetType: elbv2model.TargetTypeInstance,
			lbType:     elbv2model.LoadBalancerTypeNetwork,
			tgProtocol: elbv2model.ProtocolUDP,
			expected:   elbv2model.ProtocolTCP,
		},
		{
			name:       "alb - default",
			targetType: elbv2model.TargetTypeInstance,
			lbType:     elbv2model.LoadBalancerTypeApplication,
			tgProtocol: elbv2model.ProtocolHTTP,
			expected:   elbv2model.ProtocolHTTP,
		},
		{
			name:       "specified http",
			targetType: elbv2model.TargetTypeInstance,
			lbType:     elbv2model.LoadBalancerTypeApplication,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckProtocol: (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.ProtocolHTTP))),
				},
			},
			tgProtocol: elbv2model.ProtocolHTTP,
			expected:   elbv2model.ProtocolHTTP,
		},
		{
			name:       "specified https",
			targetType: elbv2model.TargetTypeInstance,
			lbType:     elbv2model.LoadBalancerTypeApplication,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckProtocol: (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.ProtocolHTTPS))),
				},
			},
			tgProtocol: elbv2model.ProtocolHTTP,
			expected:   elbv2model.ProtocolHTTPS,
		},
		{
			name:       "specified tcp",
			targetType: elbv2model.TargetTypeInstance,
			lbType:     elbv2model.LoadBalancerTypeApplication,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckProtocol: (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.ProtocolTCP))),
				},
			},
			tgProtocol: elbv2model.ProtocolTCP,
			expected:   elbv2model.ProtocolTCP,
		},
		{
			name:       "nlb with alb target default",
			targetType: elbv2model.TargetTypeALB,
			lbType:     elbv2model.LoadBalancerTypeNetwork,
			tgProtocol: elbv2model.ProtocolTCP,
			expected:   elbv2model.ProtocolHTTP,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := targetGroupBuilderImpl{
				loadBalancerType: tc.lbType,
			}

			res := builder.buildTargetGroupHealthCheckProtocol(tc.targetGroupProps, tc.targetType, tc.tgProtocol, false)
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

func Test_buildTargetGroupFromStaticName(t *testing.T) {

	mockMapper := &shared_utils.MockTargetGroupARNMapper{
		ARN:   "my-arn",
		Error: nil,
	}
	impl := targetGroupBuilderImpl{
		targetGroupNameToArnMapper: mockMapper,
	}

	cfg := routeutils.LiteralTargetGroupConfig{Name: "foo"}

	result, err := impl.buildTargetGroupFromStaticName(cfg)
	assert.Nil(t, err)

	resultArn, _ := result.Resolve(context.Background())
	assert.Equal(t, "my-arn", resultArn)

	mockMapper.Error = errors.New("bad")

	_, err = impl.buildTargetGroupFromStaticName(cfg)
	assert.Error(t, err)
}

func Test_buildTargetGroupTags(t *testing.T) {
	testCases := []struct {
		name         string
		defaultTags  map[string]string
		userTags     *map[string]string
		expectErr    bool
		expectedTags map[string]string
	}{
		{
			name: "successful tag retrieval with default tags",
			defaultTags: map[string]string{
				"Environment": "test",
			},
			expectedTags: map[string]string{
				"Environment": "test",
			},
		},
		{
			name:         "empty tags",
			defaultTags:  map[string]string{},
			expectedTags: map[string]string{},
		},
		{
			name: "user-specified tags",
			defaultTags: map[string]string{
				"Environment": "test",
			},
			userTags: &map[string]string{
				"CustomTag": "CustomValue",
				"Team":      "backend",
			},
			expectedTags: map[string]string{
				"Environment": "test",
				"CustomTag":   "CustomValue",
				"Team":        "backend",
			},
		},
		{
			name: "user tags override default tags",
			defaultTags: map[string]string{
				"Environment": "test",
			},
			userTags: &map[string]string{
				"Environment": "production",
			},
			expectedTags: map[string]string{
				"Environment": "production", // User tag overrides default
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a real tag helper without tracking provider
			tagger := newTagHelper(nil, tc.defaultTags, tc.name == "user tags override default tags")

			builder := newTargetGroupBuilder("test-cluster", "vpc-xxx", tagger, elbv2model.LoadBalancerTypeApplication, &mockTargetGroupBindingNetworkingBuilder{}, gateway.NewTargetGroupConfigConstructor(), string(elbv2model.TargetTypeIP), nil)

			gateway := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-gateway",
				},
			}

			route := &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "test-route",
				Namespace: "test-namespace",
			}

			backend := routeutils.NewServiceBackendConfig(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-namespace",
						Name:      "test-service",
					},
				},
				nil,
				&corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
						Type:   intstr.Int,
					},
				},
			)

			// Create target group props with user tags if specified
			var tgProps *elbv2gw.TargetGroupProps
			if tc.userTags != nil {
				tgProps = &elbv2gw.TargetGroupProps{
					Tags: tc.userTags,
				}
			}

			tgSpec, err := builder.(*targetGroupBuilderImpl).buildTargetGroupSpec(gateway, route, elbv2model.ProtocolHTTP, elbv2model.IPAddressTypeIPV4, backend, tgProps)

			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedTags, tgSpec.Tags)
		})
	}
}

func Test_buildTargetGroupFromGateway(t *testing.T) {
	testCases := []struct {
		name                 string
		gateway              *gwv1.Gateway
		listenerPort         int32
		lbIPType             elbv2model.IPAddressType
		route                *routeutils.MockRoute
		backendConfig        *routeutils.GatewayBackendConfig
		existingTG           bool
		expectedTGName       string
		expectedFrontendData bool
	}{
		{
			name: "new target group creation",
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-gw",
				},
			},
			listenerPort: 80,
			lbIPType:     elbv2model.IPAddressTypeIPV4,
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "test-route",
				Namespace: "test-ns",
			},
			backendConfig:        routeutils.NewGatewayBackendConfig(&gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "backend-gw", Namespace: "backend-ns"}}, nil, "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-alb/1234567890123456", 8080),
			expectedTGName:       "k8s-testns-testrout",
			expectedFrontendData: true,
		},
		{
			name: "existing target group reuse",
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-gw",
				},
			},
			listenerPort: 80,
			lbIPType:     elbv2model.IPAddressTypeIPV4,
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "test-route",
				Namespace: "test-ns",
			},
			backendConfig:        routeutils.NewGatewayBackendConfig(&gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "backend-gw", Namespace: "backend-ns"}}, nil, "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-alb/1234567890123456", 8080),
			existingTG:           true,
			expectedFrontendData: false,
		},
		{
			name: "with target group props",
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-gw",
				},
			},
			listenerPort: 443,
			lbIPType:     elbv2model.IPAddressTypeIPV4,
			route: &routeutils.MockRoute{
				Kind:      routeutils.HTTPRouteKind,
				Name:      "test-route",
				Namespace: "test-ns",
			},
			backendConfig: routeutils.NewGatewayBackendConfig(&gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "backend-gw", Namespace: "backend-ns"}}, &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("custom-tg-name"),
			}, "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-alb/1234567890123456", 8443),
			expectedTGName:       "custom-tg-name",
			expectedFrontendData: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tagger := &mockTagHelper{
				tags: make(map[string]string),
			}

			builder := newTargetGroupBuilder("test-cluster", "vpc-xxx", tagger, elbv2model.LoadBalancerTypeApplication, &mockTargetGroupBindingNetworkingBuilder{}, gateway.NewTargetGroupConfigConstructor(), string(elbv2model.TargetTypeALB), nil)
			impl := builder.(*targetGroupBuilderImpl)

			stack := core.NewDefaultStack(core.StackID{Namespace: "test", Name: "test"})

			// Pre-populate existing target group if needed
			if tc.existingTG {
				tgResID := impl.buildTargetGroupResourceID(k8s.NamespacedName(tc.gateway), tc.backendConfig.GetBackendNamespacedName(), tc.route.GetRouteNamespacedName(), tc.route.GetRouteKind(), tc.backendConfig.GetIdentifierPort(), nil)
				existingTG := elbv2model.NewTargetGroup(stack, tgResID, elbv2model.TargetGroupSpec{
					Name: "existing-tg",
				})
				impl.tgByResID[tgResID] = existingTG
			}

			result, err := impl.buildTargetGroupFromGateway(stack, tc.gateway, tc.listenerPort, elbv2model.ProtocolHTTP, tc.lbIPType, tc.route, *tc.backendConfig)

			assert.NoError(t, err)
			assert.NotNil(t, result)

			if tc.existingTG {
				assert.Equal(t, "existing-tg", result.Spec.Name)
			} else {
				assert.Contains(t, result.Spec.Name, tc.expectedTGName)
				assert.Equal(t, elbv2model.TargetTypeALB, result.Spec.TargetType)
			}

			// Check frontend NLB data
			if tc.expectedFrontendData {
				frontendData, exists := impl.localFrontendNlbData[result.Spec.Name]
				assert.True(t, exists)
				assert.Equal(t, result.Spec.Name, frontendData.Name)
				assert.Equal(t, tc.listenerPort, frontendData.Port)
				assert.Equal(t, *result.Spec.Port, frontendData.TargetPort)
			}
		})
	}
}

func protocolPtr(protocol elbv2gw.Protocol) *elbv2gw.Protocol {
	return &protocol
}
