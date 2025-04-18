package model

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

func Test_buildTargetGroup(t *testing.T) {
	instanceType := elbv2api.TargetType(elbv2model.TargetTypeInstance)
	ipType := elbv2api.TargetType(elbv2model.TargetTypeIP)
	http1 := elbv2model.ProtocolVersionHTTP1
	testCases := []struct {
		name                     string
		tags                     map[string]string
		lbType                   elbv2model.LoadBalancerType
		disableRestrictedSGRules bool
		defaultTargetType        string
		gateway                  *gwv1.Gateway
		route                    *routeutils.MockRoute
		backend                  routeutils.Backend
		tagErr                   error
		expectErr                bool
		expectedTgSpec           elbv2model.TargetGroupSpec
		expectedTgBindingSpec    elbv2model.TargetGroupBindingResourceSpec
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
			backend: routeutils.Backend{
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
				Name:          "k8s-myrouten-myroute-1949ae79d7",
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
			expectedTgBindingSpec: elbv2model.TargetGroupBindingResourceSpec{
				Template: elbv2model.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "k8s-myrouten-myroute-1949ae79d7",
					},
					Spec: elbv2model.TargetGroupBindingSpec{
						TargetType: &instanceType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromInt32(80), // TODO - Figure out why this port is added and not the node port.
						},
						IPAddressType: elbv2api.TargetGroupIPAddressType(elbv2model.IPAddressTypeIPV4),
						VpcID:         "vpc-xxx",
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
			backend: routeutils.Backend{
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
				Name:            "k8s-myrouten-myroute-e99d898968",
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
			expectedTgBindingSpec: elbv2model.TargetGroupBindingResourceSpec{
				Template: elbv2model.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "k8s-myrouten-myroute-e99d898968",
					},
					Spec: elbv2model.TargetGroupBindingSpec{
						TargetType: &instanceType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromInt32(80), // TODO - Figure out why this port is added and not the node port.
						},
						IPAddressType: elbv2api.TargetGroupIPAddressType(elbv2model.IPAddressTypeIPV4),
						VpcID:         "vpc-xxx",
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
			backend: routeutils.Backend{
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
				Name:          "k8s-myrouten-myroute-7ac9e90fa0",
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
			expectedTgBindingSpec: elbv2model.TargetGroupBindingResourceSpec{
				Template: elbv2model.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "k8s-myrouten-myroute-7ac9e90fa0",
					},
					Spec: elbv2model.TargetGroupBindingSpec{
						TargetType: &ipType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromInt32(80),
						},
						IPAddressType: elbv2api.TargetGroupIPAddressType(elbv2model.IPAddressTypeIPV4),
						VpcID:         "vpc-xxx",
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
			backend: routeutils.Backend{
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
				Name:            "k8s-myrouten-myroute-8a97d3dcbe",
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
			expectedTgBindingSpec: elbv2model.TargetGroupBindingResourceSpec{
				Template: elbv2model.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-svc-ns",
						Name:      "k8s-myrouten-myroute-8a97d3dcbe",
					},
					Spec: elbv2model.TargetGroupBindingSpec{
						TargetType: &ipType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromInt32(80), // TODO - Figure out why this port is added and not the node port.
						},
						IPAddressType: elbv2api.TargetGroupIPAddressType(elbv2model.IPAddressTypeIPV4),
						VpcID:         "vpc-xxx",
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

			builder := newTargetGroupBuilder("my-cluster", "vpc-xxx", tagger, tc.lbType, tc.disableRestrictedSGRules, tc.defaultTargetType)

			result := make(map[string]buildTargetGroupOutput)

			out, err := builder.buildTargetGroup(&result, tc.gateway, nil, elbv2model.IPAddressTypeIPV4, tc.route, tc.backend, nil)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedTgSpec, out.targetGroupSpec)
			assert.Equal(t, tc.expectedTgBindingSpec, out.bindingSpec)
			assert.Equal(t, 1, len(result))
			for _, v := range result {
				assert.Equal(t, tc.expectedTgSpec, v.targetGroupSpec)
				assert.Equal(t, tc.expectedTgBindingSpec, v.bindingSpec)
			}
		})
	}
}
