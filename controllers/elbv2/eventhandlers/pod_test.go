package eventhandlers

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/v3/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_enqueueRequestsForPodEvent_enqueueImpactedTargetGroupBindings(t *testing.T) {
	type tgbListCall struct {
		opts []client.ListOption
		tgbs []*elbv2api.TargetGroupBinding
		err  error
	}
	type args struct {
		pod *k8s.PodInfo
	}
	tests := []struct {
		name         string
		args         args
		wantRequests []reconcile.Request
	}{
		{
			name: "pod event should enqueue TGBs used as readiness gates",
			args: args{
				pod: &k8s.PodInfo{
					Key: types.NamespacedName{
						Namespace: "awesome-ns",
						Name:      "awesome-pod",
					},
					ReadinessGates: []corev1.PodReadinessGate{
						{ConditionType: "target-health.elbv2.k8s.aws/tgb-1"},
						{ConditionType: "target-health.alb.ingress.k8s.aws/tgb-3"},
					},
				},
			},
			wantRequests: []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{Namespace: "awesome-ns", Name: "tgb-1"},
				},
				{
					NamespacedName: types.NamespacedName{Namespace: "awesome-ns", Name: "tgb-3"},
				},
			},
		},
		{
			name: "pod event without matching readiness gates are ignored",
			args: args{
				pod: &k8s.PodInfo{
					Key: types.NamespacedName{
						Namespace: "awesome-ns",
						Name:      "awesome-pod",
					},
					ReadinessGates: []corev1.PodReadinessGate{
						{ConditionType: "ignored-prefix/tgb-2"},
					},
				},
			},
			wantRequests: nil,
		},
		{
			name: "condition type equal to the prefix does not panic and is ignored",
			args: args{
				pod: &k8s.PodInfo{
					Key: types.NamespacedName{
						Namespace: "awesome-ns",
						Name:      "awesome-pod",
					},
					ReadinessGates: []corev1.PodReadinessGate{
						{ConditionType: "target-health.elbv2.k8s.aws"},
						{ConditionType: "target-health.alb.ingress.k8s.aws"},
					},
				},
			},
			wantRequests: nil,
		},
		{
			name: "condition type with prefix and trailing slash but empty name is ignored",
			args: args{
				pod: &k8s.PodInfo{
					Key: types.NamespacedName{
						Namespace: "awesome-ns",
						Name:      "awesome-pod",
					},
					ReadinessGates: []corev1.PodReadinessGate{
						{ConditionType: "target-health.elbv2.k8s.aws/"},
					},
				},
			},
			wantRequests: nil,
		},
		{
			name: "condition type with invalid name characters is ignored",
			args: args{
				pod: &k8s.PodInfo{
					Key: types.NamespacedName{
						Namespace: "awesome-ns",
						Name:      "awesome-pod",
					},
					ReadinessGates: []corev1.PodReadinessGate{
						// underscores and uppercase pass qualified-name validation
						// but are not valid TargetGroupBinding names.
						{ConditionType: "target-health.elbv2.k8s.aws/Invalid_Name"},
					},
				},
			},
			wantRequests: nil,
		},
		{
			name: "valid and invalid readiness gates are handled independently",
			args: args{
				pod: &k8s.PodInfo{
					Key: types.NamespacedName{
						Namespace: "awesome-ns",
						Name:      "awesome-pod",
					},
					ReadinessGates: []corev1.PodReadinessGate{
						{ConditionType: "target-health.elbv2.k8s.aws"},
						{ConditionType: "target-health.elbv2.k8s.aws/tgb-1"},
					},
				},
			},
			wantRequests: []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{Namespace: "awesome-ns", Name: "tgb-1"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &enqueueRequestsForPodEvent{
				logger: logr.New(&log.NullLogSink{}),
			}
			queue := &controllertest.TypedQueue[reconcile.Request]{TypedInterface: workqueue.NewTyped[reconcile.Request]()}
			h.enqueueImpactedTargetGroupBindings(context.Background(), queue, tt.args.pod)
			gotRequests := testutils.ExtractCTRLRequestsFromQueue(queue)
			assert.True(t, cmp.Equal(tt.wantRequests, gotRequests),
				"diff", cmp.Diff(tt.wantRequests, gotRequests))
		})
	}
}

func Test_parseTargetGroupBindingName(t *testing.T) {
	const prefix = "target-health.elbv2.k8s.aws"
	type args struct {
		gateCondition string
		targetPrefix  string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "valid prefix and name",
			args: args{gateCondition: prefix + "/tgb-1", targetPrefix: prefix},
			want: "tgb-1",
		},
		{
			name: "valid name with dots and dashes",
			args: args{gateCondition: prefix + "/my.tgb-name", targetPrefix: prefix},
			want: "my.tgb-name",
		},
		{
			name: "condition type equal to prefix returns empty",
			args: args{gateCondition: prefix, targetPrefix: prefix},
			want: "",
		},
		{
			name: "prefix with trailing slash and empty name returns empty",
			args: args{gateCondition: prefix + "/", targetPrefix: prefix},
			want: "",
		},
		{
			name: "uppercase in name is rejected",
			args: args{gateCondition: prefix + "/InvalidName", targetPrefix: prefix},
			want: "",
		},
		{
			name: "underscore in name is rejected",
			args: args{gateCondition: prefix + "/invalid_name", targetPrefix: prefix},
			want: "",
		},
		{
			name: "extra slash in name is rejected",
			args: args{gateCondition: prefix + "/foo/bar", targetPrefix: prefix},
			want: "",
		},
		{
			name: "non-matching prefix returns empty",
			args: args{gateCondition: "other-prefix/tgb-1", targetPrefix: prefix},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTargetGroupBindingName(tt.args.gateCondition, tt.args.targetPrefix)
			assert.Equal(t, tt.want, got)
		})
	}
}
