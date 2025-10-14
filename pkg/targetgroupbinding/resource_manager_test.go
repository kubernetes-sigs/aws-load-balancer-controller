package targetgroupbinding

import (
	"context"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/smithy-go"
	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/util/cache"
	"net/netip"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_defaultResourceManager_updateTargetHealthPodConditionForPod(t *testing.T) {
	type env struct {
		pods []*corev1.Pod
	}

	tgbName := "tgb"
	tgbNamespace := "tgbNamespace"

	type args struct {
		pod                  k8s.PodInfo
		targetHealth         *elbv2types.TargetHealth
		targetHealthCondType corev1.PodConditionType
	}

	tests := []struct {
		name       string
		env        env
		args       args
		want       bool
		wantPod    *corev1.Pod
		wantMetric bool
		wantErr    error
	}{
		{
			name: "pod contains readinessGate and targetHealth is healthy - add pod condition",
			env: env{
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "my-pod",
							UID:       "my-pod-uuid",
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
				},
			},
			args: args{
				pod: k8s.PodInfo{
					Key: types.NamespacedName{Namespace: "default", Name: "my-pod"},
					UID: "my-pod-uuid",
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "target-health.elbv2.k8s.aws/my-tgb",
						},
					},
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
				targetHealth: &elbv2types.TargetHealth{
					State: elbv2types.TargetHealthStateEnumHealthy,
				},
				targetHealthCondType: "target-health.elbv2.k8s.aws/my-tgb",
			},
			want: false,
			wantPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-pod",
					UID:       "my-pod-uuid",
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
							Type:   "target-health.elbv2.k8s.aws/my-tgb",
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		},
		{
			name: "pod contains readinessGate and targetHealth is healthy - update pod condition",
			env: env{
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "my-pod",
							UID:       "my-pod-uuid",
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
									Type:               "target-health.elbv2.k8s.aws/my-tgb",
									Message:            string(elbv2types.TargetHealthReasonEnumRegistrationInProgress),
									Reason:             "Elb.RegistrationInProgress",
									Status:             corev1.ConditionFalse,
									LastTransitionTime: metav1.Now(),
								},
								{
									Type:   corev1.ContainersReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			args: args{
				pod: k8s.PodInfo{
					Key: types.NamespacedName{Namespace: "default", Name: "my-pod"},
					UID: "my-pod-uuid",
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "target-health.elbv2.k8s.aws/my-tgb",
						},
					},
					Conditions: []corev1.PodCondition{
						{
							Type:               "target-health.elbv2.k8s.aws/my-tgb",
							Message:            string(elbv2types.TargetHealthReasonEnumRegistrationInProgress),
							Reason:             "Elb.RegistrationInProgress",
							Status:             corev1.ConditionFalse,
							LastTransitionTime: metav1.Now(),
						},
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
				targetHealth: &elbv2types.TargetHealth{
					State: elbv2types.TargetHealthStateEnumHealthy,
				},
				targetHealthCondType: "target-health.elbv2.k8s.aws/my-tgb",
			},
			want:       false,
			wantMetric: true,
			wantPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-pod",
					UID:       "my-pod-uuid",
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
							Type:   "target-health.elbv2.k8s.aws/my-tgb",
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		},
		{
			name: "pod contains readinessGate and targetHealth is unhealthy - update pod condition",
			env: env{
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "my-pod",
							UID:       "my-pod-uuid",
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
									Type:               "target-health.elbv2.k8s.aws/my-tgb",
									Status:             corev1.ConditionFalse,
									Reason:             string(elbv2types.TargetHealthReasonEnumRegistrationInProgress),
									Message:            "Target registration is in progress",
									LastTransitionTime: metav1.Now(),
								},
								{
									Type:   corev1.ContainersReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			args: args{
				pod: k8s.PodInfo{
					Key: types.NamespacedName{Namespace: "default", Name: "my-pod"},
					UID: "my-pod-uuid",
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "target-health.elbv2.k8s.aws/my-tgb",
						},
					},
					Conditions: []corev1.PodCondition{
						{
							Type:    "target-health.elbv2.k8s.aws/my-tgb",
							Status:  corev1.ConditionFalse,
							Reason:  string(elbv2types.TargetHealthReasonEnumRegistrationInProgress),
							Message: "Target registration is in progress",
						},
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
				targetHealth: &elbv2types.TargetHealth{
					State:       elbv2types.TargetHealthStateEnumUnhealthy,
					Reason:      elbv2types.TargetHealthReasonEnumFailedHealthChecks,
					Description: awssdk.String("Health checks failed"),
				},
				targetHealthCondType: "target-health.elbv2.k8s.aws/my-tgb",
			},
			want: true,
			wantPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-pod",
					UID:       "my-pod-uuid",
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
							Type:    "target-health.elbv2.k8s.aws/my-tgb",
							Status:  corev1.ConditionFalse,
							Reason:  string(elbv2types.TargetHealthReasonEnumFailedHealthChecks),
							Message: "Health checks failed",
						},
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		},
		{
			name: "pod contains readinessGate and targetHealth is nil",
			env: env{
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "my-pod",
							UID:       "my-pod-uuid",
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
				},
			},
			args: args{
				pod: k8s.PodInfo{
					Key: types.NamespacedName{Namespace: "default", Name: "my-pod"},
					UID: "my-pod-uuid",
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "target-health.elbv2.k8s.aws/my-tgb",
						},
					},
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
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
					UID:       "my-pod-uuid",
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
							Type:   "target-health.elbv2.k8s.aws/my-tgb",
							Status: corev1.ConditionUnknown,
						},
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
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
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

			m := &defaultResourceManager{
				k8sClient:        k8sClient,
				logger:           logr.New(&log.NullLogSink{}),
				metricsCollector: lbcmetrics.NewMockCollector(),
			}

			tgb := &elbv2api.TargetGroupBinding{}
			tgb.Name = tgbName
			tgb.Namespace = tgbNamespace

			ctx := context.Background()
			for _, pod := range tt.env.pods {
				err := k8sClient.Create(ctx, pod.DeepCopy())
				assert.NoError(t, err)
			}

			got, err := m.updateTargetHealthPodConditionForPod(context.Background(),
				tt.args.pod, tt.args.targetHealth, tt.args.targetHealthCondType, tgb)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)

				updatedPod := &corev1.Pod{}
				err := m.k8sClient.Get(ctx, tt.args.pod.Key, updatedPod)
				assert.NoError(t, err)

				opts := cmp.Options{
					equality.IgnoreFakeClientPopulatedFields(),
					cmpopts.IgnoreTypes(metav1.Time{}),
				}
				assert.True(t, cmp.Equal(tt.wantPod, updatedPod, opts), "diff", cmp.Diff(tt.wantPod, updatedPod, opts))
			}

			mockCollector := m.metricsCollector.(*lbcmetrics.MockCollector)
			assert.Equal(t, tt.wantMetric, len(mockCollector.Invocations[lbcmetrics.MetricPodReadinessGateReady]) == 1)
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
							TargetHealth: &elbv2types.TargetHealth{
								State:       elbv2types.TargetHealthStateEnumInitial,
								Reason:      elbv2types.TargetHealthReasonEnumRegistrationInProgress,
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
							TargetHealth: &elbv2types.TargetHealth{
								State: elbv2types.TargetHealthStateEnumHealthy,
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

func Test_defaultResourceManager_GenerateOverrideAzFn(t *testing.T) {

	vpcId := "foo"

	type ipTestCase struct {
		ip     netip.Addr
		result bool
	}

	testCases := []struct {
		name         string
		vpcInfoCalls int
		assumeRole   string
		vpcInfo      networking.VPCInfo
		vpcInfoError error
		ipTestCases  []ipTestCase
		expectErr    bool
	}{
		{
			name:         "standard case ipv4",
			vpcInfoCalls: 1,
			vpcInfo: networking.VPCInfo{
				CidrBlockAssociationSet: []ec2types.VpcCidrBlockAssociation{
					{
						CidrBlock: awssdk.String("127.0.0.0/24"),
						CidrBlockState: &ec2types.VpcCidrBlockState{
							State: ec2types.VpcCidrBlockStateCodeAssociated,
						},
					},
				},
			},
			ipTestCases: []ipTestCase{
				{
					ip:     netip.MustParseAddr("172.0.0.0"),
					result: true,
				},
				{
					ip:     netip.MustParseAddr("127.0.0.1"),
					result: false,
				},
				{
					ip:     netip.MustParseAddr("127.0.0.2"),
					result: false,
				},
			},
		},
		{
			name:         "standard case ipv6",
			vpcInfoCalls: 1,
			vpcInfo: networking.VPCInfo{
				Ipv6CidrBlockAssociationSet: []ec2types.VpcIpv6CidrBlockAssociation{
					{
						Ipv6CidrBlock: awssdk.String("2001:db8::/32"),
						Ipv6CidrBlockState: &ec2types.VpcCidrBlockState{
							State: ec2types.VpcCidrBlockStateCodeAssociated,
						},
					},
				},
			},
			ipTestCases: []ipTestCase{
				{
					ip:     netip.MustParseAddr("5001:db8:ffff:ffff:ffff:ffff:ffff:ffff"),
					result: true,
				},
				{
					ip:     netip.MustParseAddr("2001:db8:0:0:0:0:0:0"),
					result: false,
				},
				{
					ip:     netip.MustParseAddr("2001:db8:ffff:ffff:ffff:ffff:ffff:ffff"),
					result: false,
				},
			},
		},
		{
			name:         "assume role case ram shared vpc ipv4",
			vpcInfoCalls: 1,
			assumeRole:   "foo",
			vpcInfo: networking.VPCInfo{
				CidrBlockAssociationSet: []ec2types.VpcCidrBlockAssociation{
					{
						CidrBlock: awssdk.String("127.0.0.0/24"),
						CidrBlockState: &ec2types.VpcCidrBlockState{
							State: ec2types.VpcCidrBlockStateCodeAssociated,
						},
					},
				},
			},
			ipTestCases: []ipTestCase{
				{
					ip:     netip.MustParseAddr("172.0.0.0"),
					result: true,
				},
				{
					ip:     netip.MustParseAddr("127.0.0.1"),
					result: false,
				},
				{
					ip:     netip.MustParseAddr("127.0.0.2"),
					result: false,
				},
			},
		},
		{
			name:         "assume role ram shared vpc case ipv6",
			vpcInfoCalls: 1,
			assumeRole:   "foo",
			vpcInfo: networking.VPCInfo{
				Ipv6CidrBlockAssociationSet: []ec2types.VpcIpv6CidrBlockAssociation{
					{
						Ipv6CidrBlock: awssdk.String("2001:db8::/32"),
						Ipv6CidrBlockState: &ec2types.VpcCidrBlockState{
							State: ec2types.VpcCidrBlockStateCodeAssociated,
						},
					},
				},
			},
			ipTestCases: []ipTestCase{
				{
					ip:     netip.MustParseAddr("5001:db8:ffff:ffff:ffff:ffff:ffff:ffff"),
					result: true,
				},
				{
					ip:     netip.MustParseAddr("2001:db8:0:0:0:0:0:0"),
					result: false,
				},
				{
					ip:     netip.MustParseAddr("2001:db8:ffff:ffff:ffff:ffff:ffff:ffff"),
					result: false,
				},
			},
		},
		{
			name:         "assume role case peered vpc ipv4",
			vpcInfoCalls: 1,
			assumeRole:   "foo",
			vpcInfoError: &smithy.GenericAPIError{Code: "InvalidVpcID.NotFound", Message: ""},
			ipTestCases: []ipTestCase{
				{
					ip:     netip.MustParseAddr("172.0.0.0"),
					result: true,
				},
				{
					ip:     netip.MustParseAddr("127.0.0.1"),
					result: true,
				},
				{
					ip:     netip.MustParseAddr("127.0.0.2"),
					result: true,
				},
			},
		},
		{
			name:         "assume role peered vpc case ipv6",
			vpcInfoCalls: 1,
			assumeRole:   "foo",
			vpcInfoError: &smithy.GenericAPIError{Code: "InvalidVpcID.NotFound", Message: ""},
			ipTestCases: []ipTestCase{
				{
					ip:     netip.MustParseAddr("5001:db8:ffff:ffff:ffff:ffff:ffff:ffff"),
					result: true,
				},
				{
					ip:     netip.MustParseAddr("2001:db8:0:0:0:0:0:0"),
					result: true,
				},
				{
					ip:     netip.MustParseAddr("2001:db8:ffff:ffff:ffff:ffff:ffff:ffff"),
					result: true,
				},
			},
		},
		{
			name:         "not found error from vpc info should be propagated when not using assume role",
			vpcInfoCalls: 1,
			vpcInfoError: &smithy.GenericAPIError{Code: "InvalidVpcID.NotFound", Message: ""},
			expectErr:    true,
		},
		{
			name:         "assume role case peered vpc other error should get propagated",
			vpcInfoCalls: 1,
			assumeRole:   "foo",
			vpcInfoError: &smithy.GenericAPIError{Code: "other error", Message: ""},
			expectErr:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			vpcInfoProvider := networking.NewMockVPCInfoProvider(ctrl)
			vpcInfoProvider.EXPECT().FetchVPCInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.vpcInfo, tc.vpcInfoError).Times(tc.vpcInfoCalls)
			m := &defaultResourceManager{
				logger:          logr.New(&log.NullLogSink{}),
				invalidVpcCache: cache.NewExpiring(),
				vpcInfoProvider: vpcInfoProvider,
			}

			returnedFn, err := m.generateOverrideAzFn(context.Background(), vpcId, tc.assumeRole)

			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			for _, iptc := range tc.ipTestCases {
				assert.Equal(t, iptc.result, returnedFn(iptc.ip), iptc.ip)
			}
		})
	}
}
