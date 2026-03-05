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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
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
	albController := constants.ALBGatewayController
	nlbController := constants.NLBGatewayController

	tests := []struct {
		name         string
		tgconfig     *elbv2gw.TargetGroupConfiguration
		lbConfigs    []*elbv2gw.LoadBalancerConfiguration
		gateways     []*gwv1.Gateway
		gwClasses    []*gwv1.GatewayClass
		gwController string
		wantEnqueued []types.NamespacedName
	}{
		{
			name: "no LBCs in namespace",
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "default-tgc", Namespace: "test-ns"},
				Spec:       elbv2gw.TargetGroupConfigurationSpec{},
			},
			gwController: albController,
			wantEnqueued: []types.NamespacedName{},
		},
		{
			name: "LBC references TGC, gateway references LBC directly",
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
			gateways: []*gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "test-ns"},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "alb-class",
						Infrastructure: &gwv1.GatewayInfrastructure{
							ParametersRef: &gwv1.LocalParametersReference{
								Kind: "LoadBalancerConfiguration",
								Name: "my-lbc",
							},
						},
					},
				},
			},
			gwClasses: []*gwv1.GatewayClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "alb-class"},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: gwv1.GatewayController(albController),
					},
				},
			},
			gwController: albController,
			wantEnqueued: []types.NamespacedName{
				{Name: "my-gw", Namespace: "test-ns"},
			},
		},
		{
			name: "LBC references different TGC name, no gateways enqueued",
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
			gateways: []*gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "test-ns"},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "alb-class",
						Infrastructure: &gwv1.GatewayInfrastructure{
							ParametersRef: &gwv1.LocalParametersReference{
								Kind: "LoadBalancerConfiguration",
								Name: "my-lbc",
							},
						},
					},
				},
			},
			gwClasses: []*gwv1.GatewayClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "alb-class"},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: gwv1.GatewayController(albController),
					},
				},
			},
			gwController: albController,
			wantEnqueued: []types.NamespacedName{},
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
			gwController: albController,
			wantEnqueued: []types.NamespacedName{},
		},
		{
			name: "LBC referenced by GatewayClass, gateways enqueued via GatewayClass path",
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "default-tgc", Namespace: "test-ns"},
				Spec:       elbv2gw.TargetGroupConfigurationSpec{},
			},
			lbConfigs: []*elbv2gw.LoadBalancerConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "class-lbc", Namespace: "test-ns"},
					Spec: elbv2gw.LoadBalancerConfigurationSpec{
						DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{
							Name: "default-tgc",
						},
					},
				},
			},
			gateways: []*gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "gw-1", Namespace: "test-ns"},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "nlb-class",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "gw-2", Namespace: "other-ns"},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "nlb-class",
					},
				},
			},
			gwClasses: []*gwv1.GatewayClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "nlb-class"},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: gwv1.GatewayController(nlbController),
						ParametersRef: &gwv1.ParametersReference{
							Group:     "gateway.k8s.aws",
							Kind:      "LoadBalancerConfiguration",
							Name:      "class-lbc",
							Namespace: namespacePtr("test-ns"),
						},
					},
				},
			},
			gwController: nlbController,
			wantEnqueued: []types.NamespacedName{
				{Name: "gw-1", Namespace: "test-ns"},
				{Name: "gw-2", Namespace: "other-ns"},
			},
		},
		{
			name: "gateway found via both direct LBC and GatewayClass path, enqueued only once",
			tgconfig: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "default-tgc", Namespace: "test-ns"},
				Spec:       elbv2gw.TargetGroupConfigurationSpec{},
			},
			lbConfigs: []*elbv2gw.LoadBalancerConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "shared-lbc", Namespace: "test-ns"},
					Spec: elbv2gw.LoadBalancerConfigurationSpec{
						DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{
							Name: "default-tgc",
						},
					},
				},
			},
			gateways: []*gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "test-ns"},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "alb-class",
						Infrastructure: &gwv1.GatewayInfrastructure{
							ParametersRef: &gwv1.LocalParametersReference{
								Kind: "LoadBalancerConfiguration",
								Name: "shared-lbc",
							},
						},
					},
				},
			},
			gwClasses: []*gwv1.GatewayClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "alb-class"},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: gwv1.GatewayController(albController),
						ParametersRef: &gwv1.ParametersReference{
							Group:     "gateway.k8s.aws",
							Kind:      "LoadBalancerConfiguration",
							Name:      "shared-lbc",
							Namespace: namespacePtr("test-ns"),
						},
					},
				},
			},
			gwController: albController,
			wantEnqueued: []types.NamespacedName{
				{Name: "my-gw", Namespace: "test-ns"},
			},
		},
		{
			name: "multiple LBCs, only matching ones trigger enqueue",
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
			gateways: []*gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "gw-match", Namespace: "test-ns"},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "alb-class",
						Infrastructure: &gwv1.GatewayInfrastructure{
							ParametersRef: &gwv1.LocalParametersReference{
								Kind: "LoadBalancerConfiguration",
								Name: "lbc-match",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "gw-no-match", Namespace: "test-ns"},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "alb-class",
						Infrastructure: &gwv1.GatewayInfrastructure{
							ParametersRef: &gwv1.LocalParametersReference{
								Kind: "LoadBalancerConfiguration",
								Name: "lbc-no-match",
							},
						},
					},
				},
			},
			gwClasses: []*gwv1.GatewayClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "alb-class"},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: gwv1.GatewayController(albController),
					},
				},
			},
			gwController: albController,
			wantEnqueued: []types.NamespacedName{
				{Name: "gw-match", Namespace: "test-ns"},
			},
		},
		{
			name: "gateway managed by different controller not enqueued",
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
			gateways: []*gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "nlb-gw", Namespace: "test-ns"},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "nlb-class",
						Infrastructure: &gwv1.GatewayInfrastructure{
							ParametersRef: &gwv1.LocalParametersReference{
								Kind: "LoadBalancerConfiguration",
								Name: "my-lbc",
							},
						},
					},
				},
			},
			gwClasses: []*gwv1.GatewayClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "nlb-class"},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: gwv1.GatewayController(nlbController),
					},
				},
			},
			gwController: albController,
			wantEnqueued: []types.NamespacedName{},
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
			gwController: albController,
			wantEnqueued: []types.NamespacedName{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sClient := testutils.GenerateTestClient()

			for _, lbc := range tt.lbConfigs {
				assert.NoError(t, k8sClient.Create(ctx, lbc))
			}
			for _, gwClass := range tt.gwClasses {
				assert.NoError(t, k8sClient.Create(ctx, gwClass))
			}
			for _, gw := range tt.gateways {
				assert.NoError(t, k8sClient.Create(ctx, gw))
			}

			logger := logr.New(&log.NullLogSink{})
			queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
			defer queue.ShutDown()

			h := &enqueueRequestsForTargetGroupConfigurationEvent{
				k8sClient:    k8sClient,
				logger:       logger,
				gwController: tt.gwController,
			}

			h.enqueueGatewaysReferencingDefaultTGC(ctx, tt.tgconfig, queue)

			gotEnqueued := make([]types.NamespacedName, 0)
			for queue.Len() > 0 {
				item, _ := queue.Get()
				gotEnqueued = append(gotEnqueued, item.NamespacedName)
				queue.Done(item)
			}

			assert.ElementsMatch(t, tt.wantEnqueued, gotEnqueued)
		})
	}
}

func namespacePtr(ns string) *gwv1.Namespace {
	n := gwv1.Namespace(ns)
	return &n
}
