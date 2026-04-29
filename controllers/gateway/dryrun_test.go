package gateway

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func newDryRunTestReconciler(t *testing.T, gw *gwv1.Gateway) *gatewayReconciler {
	t.Helper()
	k8sClient := testutils.GenerateTestClient()
	if gw != nil {
		assert.NoError(t, k8sClient.Create(context.Background(), gw))
	}
	return &gatewayReconciler{
		k8sClient:       k8sClient,
		logger:          logr.Discard(),
		eventRecorder:   record.NewFakeRecorder(10),
		stackMarshaller: deploy.NewDefaultStackMarshaller(),
	}
}

func Test_isDryRunEnabled(t *testing.T) {
	tests := []struct {
		name string
		gw   *gwv1.Gateway
		want bool
	}{
		{
			name: "nil gateway",
			gw:   nil,
			want: false,
		},
		{
			name: "no annotations",
			gw:   &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw"}},
			want: false,
		},
		{
			name: "annotation set to true",
			gw: &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:        "gw",
				Annotations: map[string]string{gateway_constants.AnnotationDryRun: "true"},
			}},
			want: true,
		},
		{
			name: "annotation set to non-true value",
			gw: &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:        "gw",
				Annotations: map[string]string{gateway_constants.AnnotationDryRun: "yes"},
			}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDryRunEnabled(tt.gw))
		})
	}
}

func Test_hasDryRunPlanAnnotation(t *testing.T) {
	tests := []struct {
		name string
		gw   *gwv1.Gateway
		want bool
	}{
		{
			name: "nil gateway",
			gw:   nil,
			want: false,
		},
		{
			name: "annotation absent",
			gw:   &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw"}},
			want: false,
		},
		{
			name: "annotation present",
			gw: &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:        "gw",
				Annotations: map[string]string{gateway_constants.AnnotationDryRunPlan: "{}"},
			}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasDryRunPlanAnnotation(tt.gw))
		})
	}
}

func Test_reconcileDryRun(t *testing.T) {
	tests := []struct {
		name               string
		gw                 *gwv1.Gateway
		buildStack         func(gw *gwv1.Gateway) core.Stack
		wantErr            bool
		wantPlanAnnotation bool
		wantPlanStackID    string
		wantTags           map[string]string
	}{
		{
			name: "empty stack writes annotation",
			gw: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "gw-1",
					Namespace:   "ns-1",
					Annotations: map[string]string{gateway_constants.AnnotationDryRun: "true"},
				},
			},
			buildStack: func(gw *gwv1.Gateway) core.Stack {
				return core.NewDefaultStack(core.StackID(k8s.NamespacedName(gw)))
			},
			wantPlanAnnotation: true,
			wantPlanStackID:    "ns-1/gw-1",
		},
		{
			name: "stack with tagged LoadBalancer includes tags in plan",
			gw: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "gw-tags",
					Namespace:   "ns-tags",
					Annotations: map[string]string{gateway_constants.AnnotationDryRun: "true"},
				},
			},
			buildStack: func(gw *gwv1.Gateway) core.Stack {
				stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(gw)))
				elbv2model.NewLoadBalancer(stack, "LoadBalancer", elbv2model.LoadBalancerSpec{
					Name:   "k8s-nstags-gwtags-abc123",
					Type:   elbv2model.LoadBalancerTypeApplication,
					Scheme: elbv2model.LoadBalancerSchemeInternetFacing,
					Tags: map[string]string{
						"gateway.k8s.aws/migrated-from": "ingress/ns-tags/my-ingress",
						"Environment":                   "production",
					},
				})
				return stack
			},
			wantPlanAnnotation: true,
			wantPlanStackID:    "ns-tags/gw-tags",
			wantTags: map[string]string{
				"gateway.k8s.aws/migrated-from": "ingress/ns-tags/my-ingress",
				"Environment":                   "production",
			},
		},
		{
			name: "idempotent: second run produces identical plan",
			gw: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "gw-2",
					Namespace:   "ns-2",
					Annotations: map[string]string{gateway_constants.AnnotationDryRun: "true"},
				},
			},
			buildStack: func(gw *gwv1.Gateway) core.Stack {
				return core.NewDefaultStack(core.StackID(k8s.NamespacedName(gw)))
			},
			wantPlanAnnotation: true,
			wantPlanStackID:    "ns-2/gw-2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newDryRunTestReconciler(t, tt.gw)

			current := &gwv1.Gateway{}
			assert.NoError(t, r.k8sClient.Get(context.Background(), k8s.NamespacedName(tt.gw), current))

			stack := tt.buildStack(tt.gw)
			err := r.reconcileDryRun(context.Background(), current, stack)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			stored := &gwv1.Gateway{}
			assert.NoError(t, r.k8sClient.Get(context.Background(), k8s.NamespacedName(tt.gw), stored))

			planJSON, ok := stored.Annotations[gateway_constants.AnnotationDryRunPlan]
			assert.Equal(t, tt.wantPlanAnnotation, ok, "dry-run-plan annotation presence")
			if tt.wantPlanAnnotation {
				assert.NotEmpty(t, planJSON)
				var payload map[string]interface{}
				assert.NoError(t, json.Unmarshal([]byte(planJSON), &payload))
				assert.Equal(t, tt.wantPlanStackID, payload["id"])
			}

			if tt.wantTags != nil {
				var payload map[string]interface{}
				assert.NoError(t, json.Unmarshal([]byte(planJSON), &payload))
				resources := payload["resources"].(map[string]interface{})
				lbType := resources["AWS::ElasticLoadBalancingV2::LoadBalancer"].(map[string]interface{})
				lb := lbType["LoadBalancer"].(map[string]interface{})
				spec := lb["spec"].(map[string]interface{})
				tags, ok := spec["tags"].(map[string]interface{})
				assert.True(t, ok, "LoadBalancer spec must contain tags")
				for k, v := range tt.wantTags {
					assert.Equal(t, v, tags[k])
				}
			}

			// Idempotency check
			if tt.name == "idempotent: second run produces identical plan" {
				current2 := &gwv1.Gateway{}
				assert.NoError(t, r.k8sClient.Get(context.Background(), k8s.NamespacedName(tt.gw), current2))
				assert.NoError(t, r.reconcileDryRun(context.Background(), current2, stack))
				stored2 := &gwv1.Gateway{}
				assert.NoError(t, r.k8sClient.Get(context.Background(), k8s.NamespacedName(tt.gw), stored2))
				assert.Equal(t, planJSON, stored2.Annotations[gateway_constants.AnnotationDryRunPlan],
					"identical stacks should produce identical dry-run plans")
			}
		})
	}
}

func Test_cleanupDryRunState(t *testing.T) {
	tests := []struct {
		name                   string
		gw                     *gwv1.Gateway
		wantPlanAnnotationGone bool
	}{
		{
			name: "removes annotation",
			gw: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw-3",
					Namespace: "ns-3",
					Annotations: map[string]string{
						gateway_constants.AnnotationDryRunPlan: `{"id":"ns-3/gw-3","resources":{}}`,
					},
				},
			},
			wantPlanAnnotationGone: true,
		},
		{
			name: "no-op when nothing to clean",
			gw: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw-4",
					Namespace: "ns-4",
				},
			},
			wantPlanAnnotationGone: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newDryRunTestReconciler(t, tt.gw)

			current := &gwv1.Gateway{}
			assert.NoError(t, r.k8sClient.Get(context.Background(), k8s.NamespacedName(tt.gw), current))
			assert.NoError(t, r.cleanupDryRunState(context.Background(), current))

			stored := &gwv1.Gateway{}
			assert.NoError(t, r.k8sClient.Get(context.Background(), k8s.NamespacedName(tt.gw), stored))

			if tt.wantPlanAnnotationGone {
				_, ok := stored.Annotations[gateway_constants.AnnotationDryRunPlan]
				assert.False(t, ok, "dry-run-plan annotation should be removed")
			}
		})
	}
}
