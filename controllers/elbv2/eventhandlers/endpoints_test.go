package eventhandlers

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	mock_client "sigs.k8s.io/aws-load-balancer-controller/mocks/controller-runtime/client"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_enqueueRequestsForEndpointsEvent_enqueueImpactedTargetGroupBindings(t *testing.T) {
	instanceTargetType := elbv2api.TargetTypeInstance
	ipTargetType := elbv2api.TargetTypeIP

	type tgbListCall struct {
		opts []client.ListOption
		tgbs []*elbv2api.TargetGroupBinding
		err  error
	}
	type fields struct {
		tgbListCalls []tgbListCall
	}
	type args struct {
		eps *corev1.Endpoints
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		wantRequests []ctrl.Request
	}{
		{
			name: "service event should enqueue impacted ip TargetType TGBs",
			fields: fields{
				tgbListCalls: []tgbListCall{
					{
						opts: []client.ListOption{
							client.InNamespace("awesome-ns"),
							client.MatchingFields{"spec.serviceRef.name": "awesome-svc"},
						},
						tgbs: []*elbv2api.TargetGroupBinding{
							{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "tgb-1",
								},
								Spec: elbv2api.TargetGroupBindingSpec{
									TargetType: &ipTargetType,
								},
							},
							{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "tgb-2",
								},
								Spec: elbv2api.TargetGroupBindingSpec{
									TargetType: &instanceTargetType,
								},
							},
							{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "tgb-3",
								},
								Spec: elbv2api.TargetGroupBindingSpec{
									TargetType: &ipTargetType,
								},
							},
						},
					},
				},
			},
			args: args{
				eps: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-svc",
					},
				},
			},
			wantRequests: []ctrl.Request{
				{
					NamespacedName: types.NamespacedName{Namespace: "awesome-ns", Name: "tgb-1"},
				},
				{
					NamespacedName: types.NamespacedName{Namespace: "awesome-ns", Name: "tgb-3"},
				},
			},
		},
		{
			name: "service event should enqueue impacted ip TargetType TGBs - ignore nil TargetType",
			fields: fields{
				tgbListCalls: []tgbListCall{
					{
						opts: []client.ListOption{
							client.InNamespace("awesome-ns"),
							client.MatchingFields{"spec.serviceRef.name": "awesome-svc"},
						},
						tgbs: []*elbv2api.TargetGroupBinding{
							{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "tgb-1",
								},
								Spec: elbv2api.TargetGroupBindingSpec{
									TargetType: &ipTargetType,
								},
							},
							{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "tgb-2",
								},
								Spec: elbv2api.TargetGroupBindingSpec{
									TargetType: nil,
								},
							},
						},
					},
				},
			},
			args: args{
				eps: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-svc",
					},
				},
			},
			wantRequests: []ctrl.Request{
				{
					NamespacedName: types.NamespacedName{Namespace: "awesome-ns", Name: "tgb-1"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			k8sClient := mock_client.NewMockClient(ctrl)
			for _, call := range tt.fields.tgbListCalls {
				var extraMatchers []interface{}
				for _, opt := range call.opts {
					extraMatchers = append(extraMatchers, testutils.NewListOptionEquals(opt))
				}
				k8sClient.EXPECT().List(gomock.Any(), gomock.Any(), extraMatchers...).DoAndReturn(
					func(ctx context.Context, tgbList *elbv2api.TargetGroupBindingList, opts ...client.ListOption) error {
						for _, tgb := range call.tgbs {
							tgbList.Items = append(tgbList.Items, *(tgb.DeepCopy()))
						}
						return call.err
					},
				)
			}

			h := &enqueueRequestsForEndpointsEvent{
				k8sClient: k8sClient,
				logger:    &log.NullLogger{},
			}
			queue := controllertest.Queue{Interface: workqueue.New()}
			h.enqueueImpactedTargetGroupBindings(queue, tt.args.eps)
			gotRequests := testutils.ExtractCTRLRequestsFromQueue(queue)
			assert.True(t, cmp.Equal(tt.wantRequests, gotRequests),
				"diff", cmp.Diff(tt.wantRequests, gotRequests))
		})
	}
}
