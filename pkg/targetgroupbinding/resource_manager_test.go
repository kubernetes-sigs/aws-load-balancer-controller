package targetgroupbinding

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultResourceManager_updateTargetHealthPodConditionForPod(t *testing.T) {
	type args struct {
		pod                  *corev1.Pod
		targetHealth         *elbv2sdk.TargetHealth
		targetHealthCondType corev1.PodConditionType
	}

	tests := []struct {
		name    string
		args    args
		want    bool
		wantPod *corev1.Pod
		wantErr error
	}{
		{
			name: "pod contains readinessGate and targetHealth is healthy - add pod condition",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "my-pod",
					},
					Spec: corev1.PodSpec{
						ReadinessGates: []corev1.PodReadinessGate{
							{
								ConditionType: "target-health.elbv2.k8s.aws/my-tgb",
							},
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				targetHealth: &elbv2sdk.TargetHealth{
					State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
				},
				targetHealthCondType: "target-health.elbv2.k8s.aws/my-tgb",
			},
			want: false,
			wantPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-pod",
				},
				Spec: corev1.PodSpec{
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "target-health.elbv2.k8s.aws/my-tgb",
						},
					},
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   "target-health.elbv2.k8s.aws/my-tgb",
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		},
		{
			name: "pod contains readinessGate and targetHealth is healthy - update pod condition",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "my-pod",
					},
					Spec: corev1.PodSpec{
						ReadinessGates: []corev1.PodReadinessGate{
							{
								ConditionType: "target-health.elbv2.k8s.aws/my-tgb",
							},
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:    "target-health.elbv2.k8s.aws/my-tgb",
								Status:  corev1.ConditionFalse,
								Reason:  elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress,
								Message: "Target registration is in progress",
							},
						},
					},
				},
				targetHealth: &elbv2sdk.TargetHealth{
					State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
				},
				targetHealthCondType: "target-health.elbv2.k8s.aws/my-tgb",
			},
			want: false,
			wantPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-pod",
				},
				Spec: corev1.PodSpec{
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "target-health.elbv2.k8s.aws/my-tgb",
						},
					},
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   "target-health.elbv2.k8s.aws/my-tgb",
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		},
		{
			name: "pod contains readinessGate and targetHealth is unhealthy - add pod condition",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "my-pod",
					},
					Spec: corev1.PodSpec{
						ReadinessGates: []corev1.PodReadinessGate{
							{
								ConditionType: "target-health.elbv2.k8s.aws/my-tgb",
							},
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:    "target-health.elbv2.k8s.aws/my-tgb",
								Status:  corev1.ConditionFalse,
								Reason:  elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress,
								Message: "Target registration is in progress",
							},
						},
					},
				},
				targetHealth: &elbv2sdk.TargetHealth{
					State:       awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
					Reason:      awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetFailedHealthChecks),
					Description: awssdk.String("Health checks failed"),
				},
				targetHealthCondType: "target-health.elbv2.k8s.aws/my-tgb",
			},
			want: true,
			wantPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-pod",
				},
				Spec: corev1.PodSpec{
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "target-health.elbv2.k8s.aws/my-tgb",
						},
					},
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:    "target-health.elbv2.k8s.aws/my-tgb",
							Status:  corev1.ConditionFalse,
							Reason:  elbv2sdk.TargetHealthReasonEnumTargetFailedHealthChecks,
							Message: "Health checks failed",
						},
					},
				},
			},
		},
		{
			name: "pod contains readinessGate and targetHealth is nil - currentPod has no condition",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "my-pod",
					},
					Spec: corev1.PodSpec{
						ReadinessGates: []corev1.PodReadinessGate{
							{
								ConditionType: "target-health.elbv2.k8s.aws/my-tgb",
							},
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				targetHealth:         nil,
				targetHealthCondType: "target-health.elbv2.k8s.aws/my-tgb",
			},
			want: true,
			wantPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-pod",
				},
				Spec: corev1.PodSpec{
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "target-health.elbv2.k8s.aws/my-tgb",
						},
					},
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   "target-health.elbv2.k8s.aws/my-tgb",
							Status: corev1.ConditionUnknown,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)

			m := &defaultResourceManager{
				k8sClient: k8sClient,
				logger:    &log.NullLogger{},
			}

			ctx := context.Background()
			pod := tt.args.pod.DeepCopy()
			err := k8sClient.Create(ctx, pod)
			assert.NoError(t, err)

			got, err := m.updateTargetHealthPodConditionForPod(context.Background(),
				pod, tt.args.targetHealth, tt.args.targetHealthCondType)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)

				updatedPod := &corev1.Pod{}
				err := m.k8sClient.Get(ctx, k8s.NamespacedName(pod), updatedPod)
				assert.NoError(t, err)

				opts := cmp.Options{
					equality.IgnoreFakeClientPopulatedFields(),
					cmpopts.IgnoreTypes(metav1.Time{}),
				}
				assert.True(t, cmp.Equal(tt.wantPod, updatedPod, opts), "diff", cmp.Diff(tt.wantPod, updatedPod, opts))
			}
		})
	}
}

func Test_containsTargetsInInitialState(t *testing.T) {
	type args struct {
		matchedEndpointAndTargets []podEndpointAndTargetPair
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "contains initial targets",
			args: args{
				matchedEndpointAndTargets: []podEndpointAndTargetPair{
					{
						target: TargetInfo{
							TargetHealth: &elbv2sdk.TargetHealth{
								State:       awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
								Reason:      awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
								Description: awssdk.String("Target registration is in progress"),
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "contains no initial targets",
			args: args{
				matchedEndpointAndTargets: []podEndpointAndTargetPair{
					{
						target: TargetInfo{
							TargetHealth: &elbv2sdk.TargetHealth{
								State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
							},
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsTargetsInInitialState(tt.args.matchedEndpointAndTargets)
			assert.Equal(t, tt.want, got)
		})
	}
}
