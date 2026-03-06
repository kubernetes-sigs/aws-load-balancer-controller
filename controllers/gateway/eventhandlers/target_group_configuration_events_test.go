package eventhandlers

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
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
					TargetReference: &elbv2gw.Reference{
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
					TargetReference: &elbv2gw.Reference{
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
					TargetReference: &elbv2gw.Reference{
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
					TargetReference: &elbv2gw.Reference{
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
					TargetReference: &elbv2gw.Reference{
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
					TargetReference: &elbv2gw.Reference{
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

func TestEnqueueGatewaysReferencingDefaultTGC(t *testing.T) {

	tests := []struct {
		name            string
		tgconfig        *elbv2gw.TargetGroupConfiguration
		lbConfigs       []*elbv2gw.LoadBalancerConfiguration
		wantLBCEnqueued []types.NamespacedName
	}{
		{
			name: "no LBCs in namespace",
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "default-tgc", Namespace: "test-ns"},
				Spec:       elbv2gw.TargetGroupConfigurationSpec{},
			},
			wantLBCEnqueued: []types.NamespacedName{},
		},
		{
			name: "LBC references TGC, emits LBC event",
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "default-tgc", Namespace: "test-ns"},
				Spec:       elbv2gw.TargetGroupConfigurationSpec{},
			},
			lbConfigs: []*elbv2gw.LoadBalancerConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "my-lbc", Namespace: "test-ns"},
					Spec: elbv2gw.LoadBalancerConfigurationSpec{
						DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{
							Name: "default-tgc",
						},
					},
				},
			},
			wantLBCEnqueued: []types.NamespacedName{
				{Name: "my-lbc", Namespace: "test-ns"},
			},
		},
		{
			name: "LBC references different TGC name, no LBC events emitted",
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "default-tgc", Namespace: "test-ns"},
				Spec:       elbv2gw.TargetGroupConfigurationSpec{},
			},
			lbConfigs: []*elbv2gw.LoadBalancerConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "my-lbc", Namespace: "test-ns"},
					Spec: elbv2gw.LoadBalancerConfigurationSpec{
						DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{
							Name: "other-tgc",
						},
					},
				},
			},
			wantLBCEnqueued: []types.NamespacedName{},
		},
		{
			name: "LBC has no defaultTargetGroupConfiguration",
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "default-tgc", Namespace: "test-ns"},
				Spec:       elbv2gw.TargetGroupConfigurationSpec{},
			},
			lbConfigs: []*elbv2gw.LoadBalancerConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "my-lbc", Namespace: "test-ns"},
					Spec:       elbv2gw.LoadBalancerConfigurationSpec{},
				},
			},
			wantLBCEnqueued: []types.NamespacedName{},
		},
		{
			name: "multiple LBCs, only matching ones emit events",
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "default-tgc", Namespace: "test-ns"},
				Spec:       elbv2gw.TargetGroupConfigurationSpec{},
			},
			lbConfigs: []*elbv2gw.LoadBalancerConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "lbc-match", Namespace: "test-ns"},
					Spec: elbv2gw.LoadBalancerConfigurationSpec{
						DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{
							Name: "default-tgc",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "lbc-no-match", Namespace: "test-ns"},
					Spec: elbv2gw.LoadBalancerConfigurationSpec{
						DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{
							Name: "some-other-tgc",
						},
					},
				},
			},
			wantLBCEnqueued: []types.NamespacedName{
				{Name: "lbc-match", Namespace: "test-ns"},
			},
		},
		{
			name: "LBC in different namespace not matched",
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "default-tgc", Namespace: "test-ns"},
				Spec:       elbv2gw.TargetGroupConfigurationSpec{},
			},
			lbConfigs: []*elbv2gw.LoadBalancerConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "my-lbc", Namespace: "other-ns"},
					Spec: elbv2gw.LoadBalancerConfigurationSpec{
						DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{
							Name: "default-tgc",
						},
					},
				},
			},
			wantLBCEnqueued: []types.NamespacedName{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sClient := testutils.GenerateTestClient()

			for _, lbc := range tt.lbConfigs {
				assert.NoError(t, k8sClient.Create(ctx, lbc))
			}

			logger := logr.New(&log.NullLogSink{})
			queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
			defer queue.ShutDown()

			lbcEventChan := make(chan event.TypedGenericEvent[*elbv2gw.LoadBalancerConfiguration], 10)

			h := &enqueueRequestsForTargetGroupConfigurationEvent{
				k8sClient:    k8sClient,
				logger:       logger,
				gwController: constants.ALBGatewayController,
				lbcEventChan: lbcEventChan,
			}

			h.enqueueGatewaysReferencingDefaultTGC(ctx, tt.tgconfig, queue)

			// Drain the LBC event channel
			close(lbcEventChan)
			gotLBCEnqueued := make([]types.NamespacedName, 0)
			for evt := range lbcEventChan {
				gotLBCEnqueued = append(gotLBCEnqueued, k8s.NamespacedName(evt.Object))
			}

			// Queue should be empty since we now emit LBC events instead
			assert.Equal(t, 0, queue.Len())
			assert.ElementsMatch(t, tt.wantLBCEnqueued, gotLBCEnqueued)
		})
	}
}
