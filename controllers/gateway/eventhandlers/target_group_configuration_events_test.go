package eventhandlers

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	"testing"
)

func TestGetImpactedTCPRoutes(t *testing.T) {
	tests := []struct {
		name     string
		list     *gwalpha2.TCPRouteList
		tgconfig *elbv2gw.TargetGroupConfiguration
		want     []types.NamespacedName
	}{
		{
			name: "no routes",
			list: &gwalpha2.TCPRouteList{},
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Name: "test-gateway",
						Kind: awssdk.String("Gateway"),
					},
				},
			},
			want: []types.NamespacedName{},
		},
		{
			name: "matching gateway backend",
			list: &gwalpha2.TCPRouteList{
				Items: []gwalpha2.TCPRoute{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "route1", Namespace: "test-ns"},
						Spec: gwalpha2.TCPRouteSpec{
							Rules: []gwalpha2.TCPRouteRule{
								{
									BackendRefs: []gwalpha2.BackendRef{
										{
											BackendObjectReference: gwalpha2.BackendObjectReference{
												Name: "test-gateway",
												Kind: (*gwalpha2.Kind)(awssdk.String("Gateway")),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Name: "test-gateway",
						Kind: awssdk.String("Gateway"),
					},
				},
			},
			want: []types.NamespacedName{
				{Name: "route1", Namespace: "test-ns"},
			},
		},
		{
			name: "non-matching gateway name",
			list: &gwalpha2.TCPRouteList{
				Items: []gwalpha2.TCPRoute{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "route1", Namespace: "test-ns"},
						Spec: gwalpha2.TCPRouteSpec{
							Rules: []gwalpha2.TCPRouteRule{
								{
									BackendRefs: []gwalpha2.BackendRef{
										{
											BackendObjectReference: gwalpha2.BackendObjectReference{
												Name: "other-gateway",
												Kind: (*gwalpha2.Kind)(awssdk.String("Gateway")),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Name: "test-gateway",
						Kind: awssdk.String("Gateway"),
					},
				},
			},
			want: []types.NamespacedName{},
		},
		{
			name: "different namespace",
			list: &gwalpha2.TCPRouteList{
				Items: []gwalpha2.TCPRoute{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "route1", Namespace: "other-ns"},
						Spec: gwalpha2.TCPRouteSpec{
							Rules: []gwalpha2.TCPRouteRule{
								{
									BackendRefs: []gwalpha2.BackendRef{
										{
											BackendObjectReference: gwalpha2.BackendObjectReference{
												Name: "test-gateway",
												Kind: (*gwalpha2.Kind)(awssdk.String("Gateway")),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Name: "test-gateway",
						Kind: awssdk.String("Gateway"),
					},
				},
			},
			want: []types.NamespacedName{},
		},
		{
			name: "cross-namespace with explicit namespace",
			list: &gwalpha2.TCPRouteList{
				Items: []gwalpha2.TCPRoute{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "route1", Namespace: "route-ns"},
						Spec: gwalpha2.TCPRouteSpec{
							Rules: []gwalpha2.TCPRouteRule{
								{
									BackendRefs: []gwalpha2.BackendRef{
										{
											BackendObjectReference: gwalpha2.BackendObjectReference{
												Name:      "test-gateway",
												Kind:      (*gwalpha2.Kind)(awssdk.String("Gateway")),
												Namespace: (*gwalpha2.Namespace)(awssdk.String("test-ns")),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Name: "test-gateway",
						Kind: awssdk.String("Gateway"),
					},
				},
			},
			want: []types.NamespacedName{
				{Name: "route1", Namespace: "route-ns"},
			},
		},
		{
			name: "duplicate routes filtered",
			list: &gwalpha2.TCPRouteList{
				Items: []gwalpha2.TCPRoute{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "route1", Namespace: "test-ns"},
						Spec: gwalpha2.TCPRouteSpec{
							Rules: []gwalpha2.TCPRouteRule{
								{
									BackendRefs: []gwalpha2.BackendRef{
										{
											BackendObjectReference: gwalpha2.BackendObjectReference{
												Name: "test-gateway",
												Kind: (*gwalpha2.Kind)(awssdk.String("Gateway")),
											},
										},
										{
											BackendObjectReference: gwalpha2.BackendObjectReference{
												Name: "test-gateway",
												Kind: (*gwalpha2.Kind)(awssdk.String("Gateway")),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Name: "test-gateway",
						Kind: awssdk.String("Gateway"),
					},
				},
			},
			want: []types.NamespacedName{
				{Name: "route1", Namespace: "test-ns"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getImpactedTCPRoutes(tt.list, tt.tgconfig)
			res := make([]types.NamespacedName, 0)

			for i := range got {
				res = append(res, k8s.NamespacedName(got[i]))
			}

			assert.Equal(t, tt.want, res)
		})
	}
}
