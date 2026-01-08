package targetgroupbinding

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/smithy-go"
	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/util/cache"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/backend"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"

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

func Test_isAZValidationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "AZ validation error",
			err: &smithy.GenericAPIError{
				Code:    "ValidationError",
				Message: "you must specify an Availability Zone for IP target",
			},
			want: true,
		},
		{
			name: "Different validation error",
			err: &smithy.GenericAPIError{
				Code:    "ValidationError",
				Message: "some other validation error",
			},
			want: false,
		},
		{
			name: "Different error code",
			err: &smithy.GenericAPIError{
				Code:    "InvalidParameterException",
				Message: "you must specify an Availability Zone for IP target",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAZValidationError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultResourceManager_getPodAvailabilityZone(t *testing.T) {
	tests := []struct {
		name       string
		pod        k8s.PodInfo
		nodeLabels map[string]string
		wantAZ     *string
	}{
		{
			name: "node with standard topology zone label",
			pod: k8s.PodInfo{
				Key:      types.NamespacedName{Namespace: "default", Name: "my-pod"},
				NodeName: "node-1",
			},
			nodeLabels: map[string]string{
				corev1.LabelTopologyZone: "us-east-1a",
			},
			wantAZ: awssdk.String("us-east-1a"),
		},
		{
			name: "node without zone label",
			pod: k8s.PodInfo{
				Key:      types.NamespacedName{Namespace: "default", Name: "my-pod"},
				NodeName: "node-1",
			},
			nodeLabels: map[string]string{},
			wantAZ:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)

			var k8sClient *testclient.ClientBuilder
			if tt.pod.NodeName != "" {
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   tt.pod.NodeName,
						Labels: tt.nodeLabels,
					},
				}
				k8sClient = testclient.NewClientBuilder().WithScheme(k8sSchema).WithObjects(node)
			} else {
				k8sClient = testclient.NewClientBuilder().WithScheme(k8sSchema)
			}

			m := &defaultResourceManager{
				k8sClient:   k8sClient.Build(),
				logger:      logr.New(&log.NullLogSink{}),
				nodeAZCache: cache.NewExpiring(),
			}

			ctx := context.Background()
			got, err := m.getPodAvailabilityZone(ctx, tt.pod)
			assert.NoError(t, err)

			if tt.wantAZ == nil {
				assert.Nil(t, got)
			} else {
				assert.NotNil(t, got)
				assert.Equal(t, *tt.wantAZ, *got)
			}
		})
	}
}

func Test_defaultResourceManager_getPodAvailabilityZone_Cache(t *testing.T) {
	k8sSchema := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sSchema)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				corev1.LabelTopologyZone: "us-east-1a",
			},
		},
	}

	k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).WithObjects(node).Build()

	m := &defaultResourceManager{
		k8sClient:      k8sClient,
		logger:         logr.New(&log.NullLogSink{}),
		nodeAZCache:    cache.NewExpiring(),
		nodeAZCacheTTL: 60 * time.Minute,
	}

	pod := k8s.PodInfo{
		Key:      types.NamespacedName{Namespace: "default", Name: "my-pod"},
		NodeName: "node-1",
	}

	ctx := context.Background()

	// First call - should fetch from K8s API
	az1, err := m.getPodAvailabilityZone(ctx, pod)
	assert.NoError(t, err)
	assert.NotNil(t, az1)
	assert.Equal(t, "us-east-1a", *az1)

	// Verify cache was populated
	m.nodeAZCacheMutex.RLock()
	cachedValue, found := m.nodeAZCache.Get("node-1")
	m.nodeAZCacheMutex.RUnlock()
	assert.True(t, found, "Cache should be populated after first call")
	assert.Equal(t, "us-east-1a", cachedValue.(string))

	// Delete the node from K8s to prove second call uses cache
	err = k8sClient.Delete(ctx, node)
	assert.NoError(t, err)

	// Second call - should use cache, not fail even though node is deleted
	az2, err := m.getPodAvailabilityZone(ctx, pod)
	assert.NoError(t, err)
	assert.NotNil(t, az2)
	assert.Equal(t, "us-east-1a", *az2, "Should return cached value even after node deletion")
}

func Test_defaultResourceManager_getPodAvailabilityZone_CacheForMultiplePods(t *testing.T) {
	k8sSchema := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sSchema)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				corev1.LabelTopologyZone: "us-east-1b",
			},
		},
	}

	k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).WithObjects(node).Build()

	m := &defaultResourceManager{
		k8sClient:      k8sClient,
		logger:         logr.New(&log.NullLogSink{}),
		nodeAZCache:    cache.NewExpiring(),
		nodeAZCacheTTL: 60 * time.Minute,
	}

	ctx := context.Background()

	// Multiple pods on same node
	pods := []k8s.PodInfo{
		{Key: types.NamespacedName{Namespace: "default", Name: "pod-1"}, NodeName: "node-1"},
		{Key: types.NamespacedName{Namespace: "default", Name: "pod-2"}, NodeName: "node-1"},
		{Key: types.NamespacedName{Namespace: "default", Name: "pod-3"}, NodeName: "node-1"},
	}

	// All pods should get same AZ from cache after first lookup
	for i, pod := range pods {
		az, err := m.getPodAvailabilityZone(ctx, pod)
		assert.NoError(t, err)
		assert.NotNil(t, az, "Pod %d should have AZ", i)
		assert.Equal(t, "us-east-1b", *az, "Pod %d should have correct AZ", i)
	}

	// Verify only one entry in cache
	m.nodeAZCacheMutex.RLock()
	cachedValue, found := m.nodeAZCache.Get("node-1")
	m.nodeAZCacheMutex.RUnlock()
	assert.True(t, found)
	assert.Equal(t, "us-east-1b", cachedValue.(string))
}

func Test_defaultResourceManager_registerPodEndpoints_RetryWithCache(t *testing.T) {
	k8sSchema := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sSchema)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				corev1.LabelTopologyZone: "us-east-1a",
			},
		},
	}

	k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).WithObjects(node).Build()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTargetsManager := NewMockTargetsManager(ctrl)

	tgb := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-tgb",
			Namespace: "default",
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/my-tg/1234567890abcdef",
		},
	}

	endpoints := []backend.PodEndpoint{
		{
			IP:   "172.16.0.1",
			Port: 8080,
			Pod: &k8s.PodInfo{
				Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
				NodeName: "node-1",
			},
		},
	}

	azValidationError := &smithy.GenericAPIError{
		Code:    "ValidationError",
		Message: "you must specify an Availability Zone for IP target",
	}

	// First call: RegisterTargets with AZ="all" fails with ValidationError
	mockTargetsManager.EXPECT().
		RegisterTargets(gomock.Any(), tgb, gomock.Any()).
		DoAndReturn(func(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targets []elbv2types.TargetDescription) error {
			assert.Equal(t, "all", awssdk.ToString(targets[0].AvailabilityZone))
			return azValidationError
		}).
		Times(1)

	// Second call: RegisterTargets with pod AZ succeeds
	mockTargetsManager.EXPECT().
		RegisterTargets(gomock.Any(), tgb, gomock.Any()).
		DoAndReturn(func(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targets []elbv2types.TargetDescription) error {
			assert.Equal(t, "us-east-1a", awssdk.ToString(targets[0].AvailabilityZone))
			return nil
		}).
		Times(1)

	mockVPCInfoProvider := networking.NewMockVPCInfoProvider(ctrl)
	mockVPCInfoProvider.EXPECT().FetchVPCInfo(gomock.Any(), gomock.Any()).Return(networking.VPCInfo{}, nil).AnyTimes()

	m := &defaultResourceManager{
		k8sClient:            k8sClient,
		targetsManager:       mockTargetsManager,
		logger:               logr.New(&log.NullLogSink{}),
		vpcID:                "vpc-123",
		vpcInfoProvider:      mockVPCInfoProvider,
		invalidVpcCache:      cache.NewExpiring(),
		invalidVpcCacheTTL:   60 * time.Minute,
		needsPodAZCache:      cache.NewExpiring(),
		needsPodAZCacheTTL:   60 * time.Minute,
		needsPodAZCacheMutex: sync.RWMutex{},
		nodeAZCache:          cache.NewExpiring(),
		nodeAZCacheTTL:       60 * time.Minute,
		nodeAZCacheMutex:     sync.RWMutex{},
	}

	ctx := context.Background()

	// First registration - should fail then retry and succeed
	err := m.registerPodEndpoints(ctx, tgb, endpoints)
	assert.NoError(t, err)

	// Verify cache was set
	tgbKey := "default/my-tgb"
	m.needsPodAZCacheMutex.RLock()
	_, cached := m.needsPodAZCache.Get(tgbKey)
	m.needsPodAZCacheMutex.RUnlock()
	assert.True(t, cached, "Cache should be set after retry")

	// Third call: Second registration should use pod AZ directly (no retry)
	endpoints2 := []backend.PodEndpoint{
		{
			IP:   "172.16.0.2",
			Port: 8080,
			Pod: &k8s.PodInfo{
				Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
				NodeName: "node-1",
			},
		},
	}

	mockTargetsManager.EXPECT().
		RegisterTargets(gomock.Any(), tgb, gomock.Any()).
		DoAndReturn(func(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targets []elbv2types.TargetDescription) error {
			// Verify it goes directly to pod AZ (no "all" attempt)
			assert.Equal(t, "us-east-1a", awssdk.ToString(targets[0].AvailabilityZone))
			return nil
		}).
		Times(1)

	// Second registration - should use pod AZ directly
	err = m.registerPodEndpoints(ctx, tgb, endpoints2)
	assert.NoError(t, err)
}

func Test_defaultResourceManager_registerPodEndpoints_RetryFails(t *testing.T) {
	k8sSchema := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sSchema)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				corev1.LabelTopologyZone: "us-east-1a",
			},
		},
	}

	k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).WithObjects(node).Build()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTargetsManager := NewMockTargetsManager(ctrl)

	tgb := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-tgb",
			Namespace: "default",
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/my-tg/1234567890abcdef",
		},
	}

	endpoints := []backend.PodEndpoint{
		{
			IP:   "172.16.0.1",
			Port: 8080,
			Pod: &k8s.PodInfo{
				Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
				NodeName: "node-1",
			},
		},
	}

	azValidationError := &smithy.GenericAPIError{
		Code:    "ValidationError",
		Message: "you must specify an Availability Zone for IP target",
	}

	// First call: RegisterTargets with AZ="all" fails
	mockTargetsManager.EXPECT().
		RegisterTargets(gomock.Any(), tgb, gomock.Any()).
		Return(azValidationError).
		Times(1)

	// Second call: RegisterTargets with pod AZ also fails
	mockTargetsManager.EXPECT().
		RegisterTargets(gomock.Any(), tgb, gomock.Any()).
		Return(azValidationError).
		Times(1)

	mockVPCInfoProvider := networking.NewMockVPCInfoProvider(ctrl)
	mockVPCInfoProvider.EXPECT().FetchVPCInfo(gomock.Any(), gomock.Any()).Return(networking.VPCInfo{}, nil).AnyTimes()

	m := &defaultResourceManager{
		k8sClient:            k8sClient,
		targetsManager:       mockTargetsManager,
		logger:               logr.New(&log.NullLogSink{}),
		vpcID:                "vpc-123",
		vpcInfoProvider:      mockVPCInfoProvider,
		invalidVpcCache:      cache.NewExpiring(),
		invalidVpcCacheTTL:   60 * time.Minute,
		needsPodAZCache:      cache.NewExpiring(),
		needsPodAZCacheTTL:   60 * time.Minute,
		needsPodAZCacheMutex: sync.RWMutex{},
		nodeAZCache:          cache.NewExpiring(),
		nodeAZCacheTTL:       60 * time.Minute,
		nodeAZCacheMutex:     sync.RWMutex{},
	}

	ctx := context.Background()

	// Registration should fail after retry
	err := m.registerPodEndpoints(ctx, tgb, endpoints)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "you must specify an Availability Zone")

	// Verify cache was still set (for future attempts)
	tgbKey := "default/my-tgb"
	m.needsPodAZCacheMutex.RLock()
	_, cached := m.needsPodAZCache.Get(tgbKey)
	m.needsPodAZCacheMutex.RUnlock()
	assert.True(t, cached, "Cache should be set even after failed retry")
}

func Test_defaultResourceManager_prepareRegistrationCall(t *testing.T) {
	k8sSchema := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sSchema)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				corev1.LabelTopologyZone: "us-east-1a",
			},
		},
	}

	k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).WithObjects(node).Build()

	tests := []struct {
		name         string
		endpoints    []backend.PodEndpoint
		tgb          *elbv2api.TargetGroupBinding
		doAzOverride func(addr netip.Addr) bool
		usePodAZ     bool
		wantTargets  []elbv2types.TargetDescription
		wantErr      bool
	}{
		{
			name: "usePodAZ=false, doAzOverride=true - should use 'all'",
			endpoints: []backend.PodEndpoint{
				{
					IP:   "172.16.0.1",
					Port: 8080,
					Pod: &k8s.PodInfo{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						NodeName: "node-1",
					},
				},
			},
			tgb: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{},
			},
			doAzOverride: func(addr netip.Addr) bool { return true },
			usePodAZ:     false,
			wantTargets: []elbv2types.TargetDescription{
				{
					Id:               awssdk.String("172.16.0.1"),
					Port:             awssdk.Int32(8080),
					AvailabilityZone: awssdk.String("all"),
				},
			},
		},
		{
			name: "usePodAZ=true, doAzOverride=true - should use pod AZ",
			endpoints: []backend.PodEndpoint{
				{
					IP:   "172.16.0.1",
					Port: 8080,
					Pod: &k8s.PodInfo{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						NodeName: "node-1",
					},
				},
			},
			tgb: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{},
			},
			doAzOverride: func(addr netip.Addr) bool { return true },
			usePodAZ:     true,
			wantTargets: []elbv2types.TargetDescription{
				{
					Id:               awssdk.String("172.16.0.1"),
					Port:             awssdk.Int32(8080),
					AvailabilityZone: awssdk.String("us-east-1a"),
				},
			},
		},
		{
			name: "usePodAZ=false, doAzOverride=false - no AZ set",
			endpoints: []backend.PodEndpoint{
				{
					IP:   "172.16.0.1",
					Port: 8080,
					Pod: &k8s.PodInfo{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						NodeName: "node-1",
					},
				},
			},
			tgb: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{},
			},
			doAzOverride: func(addr netip.Addr) bool { return false },
			usePodAZ:     false,
			wantTargets: []elbv2types.TargetDescription{
				{
					Id:   awssdk.String("172.16.0.1"),
					Port: awssdk.Int32(8080),
				},
			},
		},
		{
			name: "usePodAZ=true with cross-account - should use 'all'",
			endpoints: []backend.PodEndpoint{
				{
					IP:   "172.16.0.1",
					Port: 8080,
					Pod: &k8s.PodInfo{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						NodeName: "node-1",
					},
				},
			},
			tgb: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					IamRoleArnToAssume: "arn:aws:iam::123456789012:role/MyRole",
				},
			},
			doAzOverride: func(addr netip.Addr) bool { return true },
			usePodAZ:     true,
			wantTargets: []elbv2types.TargetDescription{
				{
					Id:               awssdk.String("172.16.0.1"),
					Port:             awssdk.Int32(8080),
					AvailabilityZone: awssdk.String("all"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &defaultResourceManager{
				k8sClient:        k8sClient,
				logger:           logr.New(&log.NullLogSink{}),
				nodeAZCache:      cache.NewExpiring(),
				nodeAZCacheTTL:   60 * time.Minute,
				nodeAZCacheMutex: sync.RWMutex{},
			}

			ctx := context.Background()
			got, err := m.prepareRegistrationCall(ctx, tt.endpoints, tt.tgb, tt.doAzOverride, tt.usePodAZ)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, len(tt.wantTargets), len(got))
			for i := range tt.wantTargets {
				assert.Equal(t, awssdk.ToString(tt.wantTargets[i].Id), awssdk.ToString(got[i].Id))
				assert.Equal(t, awssdk.ToInt32(tt.wantTargets[i].Port), awssdk.ToInt32(got[i].Port))
				assert.Equal(t, awssdk.ToString(tt.wantTargets[i].AvailabilityZone), awssdk.ToString(got[i].AvailabilityZone))
			}
		})
	}
}
