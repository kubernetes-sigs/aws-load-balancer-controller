package eventhandlers

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
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
		pod *corev1.Pod
	}
	tests := []struct {
		name         string
		args         args
		wantRequests []reconcile.Request
	}{
		{
			name: "pod event should enqueue TGBs used as readiness gates",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-pod",
					},
					Spec: corev1.PodSpec{
						ReadinessGates: []corev1.PodReadinessGate{
							{ConditionType: "target-health.elbv2.k8s.aws/tgb-1"},
							{ConditionType: "target-health.alb.ingress.k8s.aws/tgb-3"},
						},
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
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-pod",
					},
					Spec: corev1.PodSpec{
						ReadinessGates: []corev1.PodReadinessGate{
							{ConditionType: "ignored-prefix/tgb-2"},
						},
					},
				},
			},
			wantRequests: nil,
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
