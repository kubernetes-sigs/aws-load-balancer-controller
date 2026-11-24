package targetgroupbinding

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/smithy-go"
	"github.com/golang/mock/gomock"
	discovery "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/v3/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/backend"
	ctrlerrors "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/error"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/networking"

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
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/equality"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			Pod: k8s.PodInfo{
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
			Pod: k8s.PodInfo{
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
			Pod: k8s.PodInfo{
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

	crossAccountRoleArn := "arn:aws:iam::123456789012:role/MyRole"
	podEndpoints := []backend.PodEndpoint{
		{
			IP:   "172.16.0.1",
			Port: 8080,
			Pod: k8s.PodInfo{
				Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
				NodeName: "node-1",
			},
		},
	}

	tests := []struct {
		name              string
		endpoints         []backend.PodEndpoint
		tgb               *elbv2api.TargetGroupBinding
		doAzOverride      func(addr netip.Addr) bool
		usePodAZ          bool
		translatedAZ      *string
		translationErr    error
		wantTranslateCall bool
		wantTargets       []elbv2types.TargetDescription
		wantErr           bool
	}{
		{
			name: "usePodAZ=false, doAzOverride=true - should use 'all'",
			endpoints: []backend.PodEndpoint{
				{
					IP:   "172.16.0.1",
					Port: 8080,
					Pod: k8s.PodInfo{
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
					Pod: k8s.PodInfo{
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
					Pod: k8s.PodInfo{
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
			name:      "cross-account without registerTargetsWithPodAvailabilityZone - should use 'all'",
			endpoints: podEndpoints,
			tgb: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					IamRoleArnToAssume: crossAccountRoleArn,
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
		{
			name:      "cross-account with registerTargetsWithPodAvailabilityZone - should use translated AZ",
			endpoints: podEndpoints,
			tgb: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					IamRoleArnToAssume:                     crossAccountRoleArn,
					RegisterTargetsWithPodAvailabilityZone: awssdk.Bool(true),
				},
			},
			doAzOverride:      func(addr netip.Addr) bool { return true },
			usePodAZ:          false,
			translatedAZ:      awssdk.String("us-east-1c"),
			wantTranslateCall: true,
			wantTargets: []elbv2types.TargetDescription{
				{
					Id:               awssdk.String("172.16.0.1"),
					Port:             awssdk.Int32(8080),
					AvailabilityZone: awssdk.String("us-east-1c"),
				},
			},
		},
		{
			name:      "cross-account with unresolvable AZ - should use 'all'",
			endpoints: podEndpoints,
			tgb: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					IamRoleArnToAssume:                     crossAccountRoleArn,
					RegisterTargetsWithPodAvailabilityZone: awssdk.Bool(true),
				},
			},
			doAzOverride:      func(addr netip.Addr) bool { return true },
			translatedAZ:      nil,
			wantTranslateCall: true,
			wantTargets: []elbv2types.TargetDescription{
				{
					Id:               awssdk.String("172.16.0.1"),
					Port:             awssdk.Int32(8080),
					AvailabilityZone: awssdk.String("all"),
				},
			},
		},
		{
			name:      "cross-account with translation error - should use 'all'",
			endpoints: podEndpoints,
			tgb: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					IamRoleArnToAssume:                     crossAccountRoleArn,
					RegisterTargetsWithPodAvailabilityZone: awssdk.Bool(true),
				},
			},
			doAzOverride:      func(addr netip.Addr) bool { return true },
			translationErr:    errors.New("AccessDenied: ec2:DescribeAvailabilityZones"),
			wantTranslateCall: true,
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
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAZIDTranslator := networking.NewMockAZIDTranslator(ctrl)
			if tt.wantTranslateCall {
				mockAZIDTranslator.EXPECT().
					TranslateAZName(gomock.Any(), crossAccountRoleArn, gomock.Any(), "us-east-1a").
					Return(tt.translatedAZ, tt.translationErr).
					Times(1)
			} else {
				mockAZIDTranslator.EXPECT().TranslateAZName(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			}

			m := &defaultResourceManager{
				k8sClient:        k8sClient,
				logger:           logr.New(&log.NullLogSink{}),
				azIDTranslator:   mockAZIDTranslator,
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

// Test_needReadinessGateFlip tests that needReadinessGateFlip correctly
// identifies pods with a written but not-yet-True readiness gate condition.
func Test_needReadinessGateFlip(t *testing.T) {
	condType := corev1.PodConditionType("target-health.elbv2.k8s.aws/my-tgb")

	tests := []struct {
		name      string
		endpoints []backend.PodEndpoint
		want      bool
	}{
		{
			name: "all pods have target health condition True — no flip needed",
			endpoints: []backend.PodEndpoint{
				{Pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "pod-1"},
					Conditions: []corev1.PodCondition{
						{Type: condType, Status: corev1.ConditionTrue},
					},
				}},
				{Pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "pod-2"},
					Conditions: []corev1.PodCondition{
						{Type: condType, Status: corev1.ConditionTrue},
					},
				}},
			},
			want: false,
		},
		{
			name: "one pod has target health condition False (target unhealthy) — flip needed",
			endpoints: []backend.PodEndpoint{
				{Pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "pod-1"},
					Conditions: []corev1.PodCondition{
						{Type: condType, Status: corev1.ConditionTrue},
					},
				}},
				{Pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "pod-2"},
					Conditions: []corev1.PodCondition{
						{Type: condType, Status: corev1.ConditionFalse},
					},
				}},
			},
			want: true,
		},
		{
			name: "pod has no target health condition written yet — no flip needed (requeue after condition is first written handles it)",
			endpoints: []backend.PodEndpoint{
				{Pod: k8s.PodInfo{
					Key:        types.NamespacedName{Name: "pod-1"},
					Conditions: []corev1.PodCondition{},
				}},
			},
			want: false,
		},
		{
			name:      "no endpoints — no flip needed",
			endpoints: []backend.PodEndpoint{},
			want:      false,
		},
		{
			name: "pod has no target health condition and no readiness gate configured — no flip needed",
			endpoints: []backend.PodEndpoint{
				{Pod: k8s.PodInfo{
					Key:        types.NamespacedName{Name: "pod-1"},
					Conditions: []corev1.PodCondition{},
				}},
			},
			want: false,
		},
		{
			name: "mix — one pod target health condition True, one pod condition not yet written — no flip needed",
			endpoints: []backend.PodEndpoint{
				{Pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "pod-target-healthy"},
					Conditions: []corev1.PodCondition{
						{Type: condType, Status: corev1.ConditionTrue},
					},
				}},
				{Pod: k8s.PodInfo{
					Key:        types.NamespacedName{Name: "pod-no-condition-yet"},
					Conditions: []corev1.PodCondition{},
				}},
			},
			want: false,
		},
		{
			name: "mix — one pod target health condition True, one pod target health condition False (target unhealthy) — flip needed",
			endpoints: []backend.PodEndpoint{
				{Pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "pod-target-healthy"},
					Conditions: []corev1.PodCondition{
						{Type: condType, Status: corev1.ConditionTrue},
					},
				}},
				{Pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "pod-target-unhealthy"},
					Conditions: []corev1.PodCondition{
						{Type: condType, Status: corev1.ConditionFalse},
					},
				}},
			},
			want: true,
		},
		{
			name: "all pods have target health condition False (all targets unhealthy) — flip needed",
			endpoints: []backend.PodEndpoint{
				{Pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "pod-1"},
					Conditions: []corev1.PodCondition{
						{Type: condType, Status: corev1.ConditionFalse},
					},
				}},
				{Pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "pod-2"},
					Conditions: []corev1.PodCondition{
						{Type: condType, Status: corev1.ConditionFalse},
					},
				}},
			},
			want: true,
		},
		{
			name: "pod has multiple TGB conditions — current TGB condition True, other TGB condition False — no flip needed",
			endpoints: []backend.PodEndpoint{
				{Pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "pod-1"},
					Conditions: []corev1.PodCondition{
						{Type: condType, Status: corev1.ConditionTrue},
						{Type: corev1.PodConditionType("target-health.elbv2.k8s.aws/other-tgb"), Status: corev1.ConditionFalse},
					},
				}},
			},
			want: false, // only condType is checked; the other TGB's condition is irrelevant
		},
		{
			name: "pod has multiple TGB conditions — current TGB condition False, other TGB condition True — flip needed",
			endpoints: []backend.PodEndpoint{
				{Pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "pod-1"},
					Conditions: []corev1.PodCondition{
						{Type: condType, Status: corev1.ConditionFalse},
						{Type: corev1.PodConditionType("target-health.elbv2.k8s.aws/other-tgb"), Status: corev1.ConditionTrue},
					},
				}},
			},
			want: true, // condType is False regardless of other TGB conditions
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needReadinessGateFlip(tt.endpoints, condType)
			assert.Equal(t, tt.want, got)
		})
	}
}

type fakePodEndpointResolver struct {
	endpoints []backend.PodEndpoint
	excluded  []backend.PodEndpoint
}

func (f *fakePodEndpointResolver) ResolvePodEndpoints(_ context.Context, _ types.NamespacedName, _ intstr.IntOrString, _ discovery.AddressType) ([]backend.PodEndpoint, []backend.PodEndpoint, error) {
	return f.endpoints, f.excluded, nil
}

func (f *fakePodEndpointResolver) ResolveNodePortEndpoints(_ context.Context, _ types.NamespacedName, _ intstr.IntOrString, _ ...backend.EndpointResolveOption) ([]backend.NodePortEndpoint, error) {
	return nil, nil
}

type countingNetworkingManager struct {
	podCalls int
}

func (n *countingNetworkingManager) ReconcileForPodEndpoints(_ context.Context, _ *elbv2api.TargetGroupBinding, _ []backend.PodEndpoint) error {
	n.podCalls++
	return nil
}

func (n *countingNetworkingManager) ReconcileForNodePortEndpoints(_ context.Context, _ *elbv2api.TargetGroupBinding, _ []backend.NodePortEndpoint) error {
	return nil
}

func (n *countingNetworkingManager) Cleanup(_ context.Context, _ *elbv2api.TargetGroupBinding) error {
	return nil
}

func (n *countingNetworkingManager) AttemptGarbageCollection(_ context.Context) error {
	return nil
}

const nodeNotReadyRequeueReason = "node not ready pod excluded"

func requeueReason(err error) string {
	var requeue *ctrlerrors.RequeueNeeded
	if errors.As(err, &requeue) {
		return requeue.Reason()
	}
	return ""
}

func Test_defaultResourceManager_reconcileWithIPTargetType_nodeNotReadyRequeue(t *testing.T) {
	tgbTemplate := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "tgb"},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/tg/1234567890abcdef",
			TargetType:     &[]elbv2api.TargetType{elbv2api.TargetTypeIP}[0],
			ServiceRef:     elbv2api.ServiceReference{Name: "svc", Port: intstr.FromInt32(80)},
		},
	}
	condType := BuildTargetHealthPodConditionType(tgbTemplate)

	readyEp := backend.PodEndpoint{
		IP: "10.0.0.1", Port: 8080,
		Pod: k8s.PodInfo{Key: types.NamespacedName{Namespace: "default", Name: "ready"}, NodeName: "node-ok"},
	}
	podWithGate := func(gateStatus *corev1.ConditionStatus) backend.PodEndpoint {
		pod := k8s.PodInfo{
			Key:            types.NamespacedName{Namespace: "default", Name: "stuck"},
			NodeName:       "node-1",
			PodIP:          "10.0.0.2",
			ReadinessGates: []corev1.PodReadinessGate{{ConditionType: condType}},
			Conditions:     []corev1.PodCondition{{Type: corev1.ContainersReady, Status: corev1.ConditionTrue}},
		}
		if gateStatus != nil {
			pod.Conditions = append(pod.Conditions, corev1.PodCondition{Type: condType, Status: *gateStatus})
		}
		return backend.PodEndpoint{IP: "10.0.0.2", Port: 8080, Pod: pod}
	}
	gateFalse, gateTrue := corev1.ConditionFalse, corev1.ConditionTrue
	stuckEp := podWithGate(&gateFalse)
	readyTarget := TargetInfo{
		Target:       elbv2types.TargetDescription{Id: awssdk.String("10.0.0.1"), Port: awssdk.Int32(8080)},
		TargetHealth: &elbv2types.TargetHealth{State: elbv2types.TargetHealthStateEnumHealthy},
	}

	type env struct {
		m        *defaultResourceManager
		tgb      *elbv2api.TargetGroupBinding
		resolver *fakePodEndpointResolver
		targets  *MockTargetsManager
		netMgr   *countingNetworkingManager
	}
	setup := func(t *testing.T, endpoints, excluded []backend.PodEndpoint, checkpointMatches bool) env {
		ctrl := gomock.NewController(t)
		k8sSchema := runtime.NewScheme()
		clientgoscheme.AddToScheme(k8sSchema)
		elbv2api.AddToScheme(k8sSchema)
		tgb := tgbTemplate.DeepCopy()
		if checkpointMatches {
			checkpoint, err := calculateTGBReconcileCheckpoint(endpoints, tgb)
			assert.NoError(t, err)
			SaveTGBReconcileCheckpoint(tgb, checkpoint)
		}
		k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).WithObjects(tgb.DeepCopy()).Build()
		assert.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "tgb"}, tgb))

		resolver := &fakePodEndpointResolver{endpoints: endpoints, excluded: excluded}
		targets := NewMockTargetsManager(ctrl)
		netMgr := &countingNetworkingManager{}
		vpcInfoProvider := networking.NewMockVPCInfoProvider(ctrl)
		vpcInfoProvider.EXPECT().FetchVPCInfo(gomock.Any(), gomock.Any()).Return(networking.VPCInfo{}, nil).AnyTimes()
		logger := logr.New(&log.NullLogSink{})
		m := &defaultResourceManager{
			k8sClient:           k8sClient,
			targetsManager:      targets,
			endpointResolver:    resolver,
			networkingManager:   netMgr,
			eventRecorder:       record.NewFakeRecorder(10),
			logger:              logger,
			vpcID:               "vpc-1",
			vpcInfoProvider:     vpcInfoProvider,
			multiClusterManager: NewMultiClusterManager(k8sClient, k8sClient, logger),
			metricsCollector:    lbcmetrics.NewMockCollector(),
			invalidVpcCache:     cache.NewExpiring(),
			invalidVpcCacheTTL:  time.Minute,
			needsPodAZCache:     cache.NewExpiring(),
			needsPodAZCacheTTL:  time.Minute,
			nodeAZCache:         cache.NewExpiring(),
			nodeAZCacheTTL:      time.Minute,
			requeueDuration:     15 * time.Second,
		}
		return env{m: m, tgb: tgb, resolver: resolver, targets: targets, netMgr: netMgr}
	}

	t.Run("checkpoint matches and gated pod excluded: requeue before early exit without ELB calls, every retry", func(t *testing.T) {
		e := setup(t, []backend.PodEndpoint{readyEp}, []backend.PodEndpoint{stuckEp}, true)
		checkpoint := GetTGBReconcileCheckpoint(e.tgb)
		for i := 0; i < 3; i++ {
			deferred, err := e.m.Reconcile(context.Background(), e.tgb)
			assert.False(t, deferred)
			assert.Equal(t, nodeNotReadyRequeueReason, requeueReason(err))
		}
		assert.Equal(t, 0, e.netMgr.podCalls)
		assert.Equal(t, checkpoint, GetTGBReconcileCheckpoint(e.tgb))
	})

	t.Run("gated pod with absent gate condition is requeued", func(t *testing.T) {
		e := setup(t, []backend.PodEndpoint{readyEp}, []backend.PodEndpoint{podWithGate(nil)}, true)
		_, err := e.m.Reconcile(context.Background(), e.tgb)
		assert.Equal(t, nodeNotReadyRequeueReason, requeueReason(err))
	})

	for _, tc := range []struct {
		name     string
		excluded []backend.PodEndpoint
	}{
		{name: "no excluded pod"},
		{name: "excluded pod without gate", excluded: []backend.PodEndpoint{{IP: "10.0.0.3", Port: 8080, Pod: k8s.PodInfo{Key: types.NamespacedName{Namespace: "default", Name: "nogate"}}}}},
		{name: "excluded pod with gate already True", excluded: []backend.PodEndpoint{podWithGate(&gateTrue)}},
	} {
		t.Run("checkpoint matches, "+tc.name+": deferred early exit, no requeue", func(t *testing.T) {
			e := setup(t, []backend.PodEndpoint{readyEp}, tc.excluded, true)
			deferred, err := e.m.Reconcile(context.Background(), e.tgb)
			assert.True(t, deferred)
			assert.NoError(t, err)
		})
	}

	t.Run("checkpoint differs: full reconcile saves checkpoint and requeues, retries then early exit without ELB calls", func(t *testing.T) {
		e := setup(t, []backend.PodEndpoint{readyEp}, []backend.PodEndpoint{stuckEp}, false)
		e.targets.EXPECT().ListTargets(gomock.Any(), gomock.Any()).Return([]TargetInfo{readyTarget}, nil).Times(1)

		deferred, err := e.m.Reconcile(context.Background(), e.tgb)
		assert.False(t, deferred)
		assert.Equal(t, nodeNotReadyRequeueReason, requeueReason(err))
		assert.Equal(t, 1, e.netMgr.podCalls)
		want, _ := calculateTGBReconcileCheckpoint([]backend.PodEndpoint{readyEp}, e.tgb)
		stored := &elbv2api.TargetGroupBinding{}
		assert.NoError(t, e.m.k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "tgb"}, stored))
		assert.Equal(t, want, GetTGBReconcileCheckpoint(stored))

		deferred, err = e.m.Reconcile(context.Background(), stored)
		assert.False(t, deferred)
		assert.Equal(t, nodeNotReadyRequeueReason, requeueReason(err))
		assert.Equal(t, 1, e.netMgr.podCalls)
	})

	t.Run("node back to Ready: pod is resolved again and registered", func(t *testing.T) {
		e := setup(t, []backend.PodEndpoint{readyEp}, []backend.PodEndpoint{stuckEp}, true)
		_, err := e.m.Reconcile(context.Background(), e.tgb)
		assert.Equal(t, nodeNotReadyRequeueReason, requeueReason(err))

		e.resolver.endpoints, e.resolver.excluded = []backend.PodEndpoint{readyEp, stuckEp}, nil
		e.targets.EXPECT().ListTargets(gomock.Any(), gomock.Any()).Return([]TargetInfo{readyTarget}, nil).Times(1)
		e.targets.EXPECT().RegisterTargets(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *elbv2api.TargetGroupBinding, targets []elbv2types.TargetDescription) error {
				assert.Len(t, targets, 1)
				assert.Equal(t, "10.0.0.2", awssdk.ToString(targets[0].Id))
				return nil
			}).Times(1)

		// No Pod object in the fake client, so the gate patch is NotFound and skipped: full reconcile ends with no error.
		deferred, err := e.m.Reconcile(context.Background(), e.tgb)
		assert.False(t, deferred)
		assert.NoError(t, err)
	})
}

// clientPodInfoRepo reads pods from the fake client, so gate patches are seen by the next resolve.
type clientPodInfoRepo struct {
	k8sClient client.Client
}

func (r *clientPodInfoRepo) Get(ctx context.Context, key types.NamespacedName) (k8s.PodInfo, bool, error) {
	pod := &corev1.Pod{}
	if err := r.k8sClient.Get(ctx, key, pod); err != nil {
		if apierrors.IsNotFound(err) {
			return k8s.PodInfo{}, false, nil
		}
		return k8s.PodInfo{}, false, err
	}
	return k8s.PodInfo{
		Key:            key,
		UID:            pod.UID,
		ReadinessGates: pod.Spec.ReadinessGates,
		Conditions:     pod.Status.Conditions,
		NodeName:       pod.Spec.NodeName,
		PodIP:          pod.Status.PodIP,
	}, true, nil
}

func (r *clientPodInfoRepo) ListKeys(_ context.Context) []types.NamespacedName {
	return nil
}

// fakeTargetsManager keeps the target group in memory and counts ELB calls.
type fakeTargetsManager struct {
	targets                    []TargetInfo
	register, deregister, list int
	lastRegistered             []string
	lastDeregistered           []string
}

func (f *fakeTargetsManager) RegisterTargets(_ context.Context, _ *elbv2api.TargetGroupBinding, targets []elbv2types.TargetDescription) error {
	f.register++
	f.lastRegistered = nil
	for _, t := range targets {
		f.lastRegistered = append(f.lastRegistered, awssdk.ToString(t.Id))
		f.targets = append(f.targets, TargetInfo{
			Target:       elbv2types.TargetDescription{Id: t.Id, Port: t.Port},
			TargetHealth: &elbv2types.TargetHealth{State: elbv2types.TargetHealthStateEnumInitial, Reason: elbv2types.TargetHealthReasonEnumRegistrationInProgress},
		})
	}
	return nil
}

func (f *fakeTargetsManager) DeregisterTargets(_ context.Context, _ *elbv2api.TargetGroupBinding, targets []elbv2types.TargetDescription) error {
	f.deregister++
	f.lastDeregistered = nil
	for _, t := range targets {
		id := awssdk.ToString(t.Id)
		f.lastDeregistered = append(f.lastDeregistered, id)
		for i := range f.targets {
			if awssdk.ToString(f.targets[i].Target.Id) == id {
				f.targets = append(f.targets[:i], f.targets[i+1:]...)
				break
			}
		}
	}
	return nil
}

func (f *fakeTargetsManager) ListTargets(_ context.Context, _ *elbv2api.TargetGroupBinding) ([]TargetInfo, error) {
	f.list++
	return append([]TargetInfo(nil), f.targets...), nil
}

func (f *fakeTargetsManager) calls() int {
	return f.register + f.deregister + f.list
}

func (f *fakeTargetsManager) resetCounts() {
	f.register, f.deregister, f.list = 0, 0, 0
	f.lastRegistered, f.lastDeregistered = nil, nil
}

func (f *fakeTargetsManager) setHealthy(id string) {
	for i := range f.targets {
		if awssdk.ToString(f.targets[i].Target.Id) == id {
			f.targets[i].TargetHealth = &elbv2types.TargetHealth{State: elbv2types.TargetHealthStateEnumHealthy}
		}
	}
}

func (f *fakeTargetsManager) ids() []string {
	var ids []string
	for _, t := range f.targets {
		ids = append(ids, awssdk.ToString(t.Target.Id))
	}
	return ids
}

func requeueAfterReason(err error) string {
	var requeue *ctrlerrors.RequeueNeededAfter
	if errors.As(err, &requeue) {
		return requeue.Reason()
	}
	return ""
}

// realResolverEnv wires the real endpoint resolver and resource manager to a fake client:
// pod A (no gate) on node-ok, pod B (gated) on node-b, both behind svc.
type realResolverEnv struct {
	t         *testing.T
	ctx       context.Context
	k8sClient client.Client
	m         *defaultResourceManager
	targets   *fakeTargetsManager
	netMgr    *countingNetworkingManager
	condType  corev1.PodConditionType
	tgbKey    types.NamespacedName
	withA     bool
}

const (
	podAIP = "10.0.0.1"
	podBIP = "10.0.0.2"
)

func readyNodeCond(status corev1.ConditionStatus) []corev1.NodeCondition {
	return []corev1.NodeCondition{{Type: corev1.NodeReady, Status: status}}
}

func newRealResolverEnv(t *testing.T, failOpen, withA bool) *realResolverEnv {
	ctrl := gomock.NewController(t)
	ctx := context.Background()
	k8sSchema := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sSchema)
	elbv2api.AddToScheme(k8sSchema)

	tgb := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "tgb"},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/tg/1234567890abcdef",
			TargetType:     &[]elbv2api.TargetType{elbv2api.TargetTypeIP}[0],
			ServiceRef:     elbv2api.ServiceReference{Name: "svc", Port: intstr.FromInt32(80)},
		},
	}
	condType := BuildTargetHealthPodConditionType(tgb)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "svc"},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080)}}},
	}
	newNode := func(name string) *corev1.Node {
		return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{Conditions: readyNodeCond(corev1.ConditionTrue)}}
	}
	newPod := func(name, node, ip string, gates []corev1.PodReadinessGate) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name, UID: types.UID(name + "-uid")},
			Spec:       corev1.PodSpec{NodeName: node, ReadinessGates: gates},
			Status: corev1.PodStatus{
				PodIP:      ip,
				Conditions: []corev1.PodCondition{{Type: corev1.ContainersReady, Status: corev1.ConditionTrue}},
			},
		}
	}
	eps := &discovery.EndpointSlice{
		ObjectMeta:  metav1.ObjectMeta{Namespace: "default", Name: "svc-abc", Labels: map[string]string{discovery.LabelServiceName: "svc"}},
		AddressType: discovery.AddressTypeIPv4,
		Ports:       []discovery.EndpointPort{{Name: awssdk.String("http"), Port: awssdk.Int32(8080)}},
	}
	k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).WithObjects(
		tgb, svc, eps, newNode("node-ok"), newNode("node-b"),
		newPod("pod-a", "node-ok", podAIP, nil),
		newPod("pod-b", "node-b", podBIP, []corev1.PodReadinessGate{{ConditionType: condType}}),
	).Build()

	logger := logr.New(&log.NullLogSink{})
	podRepo := &clientPodInfoRepo{k8sClient: k8sClient}
	targets := &fakeTargetsManager{}
	if withA {
		// A is already registered and healthy.
		targets.targets = []TargetInfo{{
			Target:       elbv2types.TargetDescription{Id: awssdk.String(podAIP), Port: awssdk.Int32(8080)},
			TargetHealth: &elbv2types.TargetHealth{State: elbv2types.TargetHealthStateEnumHealthy},
		}}
	}
	netMgr := &countingNetworkingManager{}
	vpcInfoProvider := networking.NewMockVPCInfoProvider(ctrl)
	vpcInfoProvider.EXPECT().FetchVPCInfo(gomock.Any(), gomock.Any()).Return(networking.VPCInfo{}, nil).AnyTimes()
	m := &defaultResourceManager{
		k8sClient:           k8sClient,
		targetsManager:      targets,
		endpointResolver:    backend.NewDefaultEndpointResolver(k8sClient, podRepo, failOpen, true, logger),
		networkingManager:   netMgr,
		eventRecorder:       record.NewFakeRecorder(100),
		logger:              logger,
		vpcID:               "vpc-1",
		vpcInfoProvider:     vpcInfoProvider,
		podInfoRepo:         podRepo,
		multiClusterManager: NewMultiClusterManager(k8sClient, k8sClient, logger),
		metricsCollector:    lbcmetrics.NewMockCollector(),
		invalidVpcCache:     cache.NewExpiring(),
		invalidVpcCacheTTL:  time.Minute,
		needsPodAZCache:     cache.NewExpiring(),
		needsPodAZCacheTTL:  time.Minute,
		nodeAZCache:         cache.NewExpiring(),
		nodeAZCacheTTL:      time.Minute,
		requeueDuration:     15 * time.Second,
	}
	return &realResolverEnv{t: t, ctx: ctx, k8sClient: k8sClient, m: m, targets: targets, netMgr: netMgr,
		condType: condType, tgbKey: types.NamespacedName{Namespace: "default", Name: "tgb"}, withA: withA}
}

// setNodeB sets node-b's conditions; nil means no Ready condition.
func (e *realResolverEnv) setNodeB(conds []corev1.NodeCondition) {
	node := &corev1.Node{}
	assert.NoError(e.t, e.k8sClient.Get(e.ctx, types.NamespacedName{Name: "node-b"}, node))
	node.Status.Conditions = conds
	assert.NoError(e.t, e.k8sClient.Status().Update(e.ctx, node))
}

// setSlice rewrites the EndpointSlice with A ready (if present) and B in the given state.
func (e *realResolverEnv) setSlice(bReady, bTerminating bool) {
	ep := func(name, node, ip string, ready, terminating bool) discovery.Endpoint {
		return discovery.Endpoint{
			Addresses:  []string{ip},
			Conditions: discovery.EndpointConditions{Ready: awssdk.Bool(ready), Terminating: awssdk.Bool(terminating)},
			NodeName:   awssdk.String(node),
			TargetRef:  &corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: name},
		}
	}
	eps := &discovery.EndpointSlice{}
	assert.NoError(e.t, e.k8sClient.Get(e.ctx, types.NamespacedName{Namespace: "default", Name: "svc-abc"}, eps))
	eps.Endpoints = nil
	if e.withA {
		eps.Endpoints = append(eps.Endpoints, ep("pod-a", "node-ok", podAIP, true, false))
	}
	eps.Endpoints = append(eps.Endpoints, ep("pod-b", "node-b", podBIP, bReady, bTerminating))
	assert.NoError(e.t, e.k8sClient.Update(e.ctx, eps))
}

// reconcile runs one cycle on a freshly read TGB, as the controller does.
func (e *realResolverEnv) reconcile() (bool, error) {
	tgb := &elbv2api.TargetGroupBinding{}
	assert.NoError(e.t, e.k8sClient.Get(e.ctx, e.tgbKey, tgb))
	return e.m.Reconcile(e.ctx, tgb)
}

func (e *realResolverEnv) checkpoint() string {
	tgb := &elbv2api.TargetGroupBinding{}
	assert.NoError(e.t, e.k8sClient.Get(e.ctx, e.tgbKey, tgb))
	return GetTGBReconcileCheckpoint(tgb)
}

func (e *realResolverEnv) gateB() corev1.ConditionStatus {
	pod := &corev1.Pod{}
	assert.NoError(e.t, e.k8sClient.Get(e.ctx, types.NamespacedName{Namespace: "default", Name: "pod-b"}, pod))
	for _, c := range pod.Status.Conditions {
		if c.Type == e.condType {
			return c.Status
		}
	}
	return ""
}

// registerB runs cycle 1: B ready on a Ready node is registered with gate False.
func (e *realResolverEnv) registerB() {
	e.setSlice(true, false)
	_, err := e.reconcile()
	assert.Equal(e.t, "monitor targetHealth", requeueAfterReason(err))
	assert.Equal(e.t, 1, e.targets.register)
	assert.Equal(e.t, []string{podBIP}, e.targets.lastRegistered)
	assert.Equal(e.t, corev1.ConditionFalse, e.gateB())
}

func Test_defaultResourceManager_Reconcile_nodeNotReadyRequeue_realResolver(t *testing.T) {
	badNodes := []struct {
		name  string
		conds []corev1.NodeCondition
	}{
		{name: "node Unknown", conds: readyNodeCond(corev1.ConditionUnknown)},
		{name: "node Ready=False", conds: readyNodeCond(corev1.ConditionFalse)},
		{name: "node without Ready condition", conds: nil},
	}
	for _, bad := range badNodes {
		for _, failOpen := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, failOpen=%v: requeue loop until node Ready, then register", bad.name, failOpen), func(t *testing.T) {
				e := newRealResolverEnv(t, failOpen, true)

				// Cycle 1: B registered, gate False.
				e.registerB()

				// Cycle 2: node bad, B not ready -> excluded, deregistered, checkpoint saved, rate-limited requeue.
				e.targets.resetCounts()
				e.netMgr.podCalls = 0
				e.setNodeB(bad.conds)
				e.setSlice(false, false)
				deferred, err := e.reconcile()
				assert.False(t, deferred)
				assert.Equal(t, nodeNotReadyRequeueReason, requeueReason(err))
				assert.Equal(t, 1, e.targets.deregister)
				assert.Equal(t, []string{podBIP}, e.targets.lastDeregistered)
				assert.Equal(t, 0, e.targets.register)
				assert.Equal(t, []string{podAIP}, e.targets.ids())
				assert.Equal(t, 1, e.netMgr.podCalls)
				saved := e.checkpoint()
				assert.NotEmpty(t, saved)

				// Cycles 3-5: checkpoint early exit, still requeues, no ELB or networking calls.
				e.targets.resetCounts()
				e.netMgr.podCalls = 0
				for i := 0; i < 3; i++ {
					deferred, err = e.reconcile()
					assert.False(t, deferred)
					assert.Equal(t, nodeNotReadyRequeueReason, requeueReason(err), "retry %d", i)
				}
				assert.Equal(t, 0, e.targets.calls())
				assert.Equal(t, 0, e.netMgr.podCalls)
				assert.Equal(t, saved, e.checkpoint())

				// Node Ready: B resolved again and registered once; no node-not-ready requeue.
				e.setNodeB(readyNodeCond(corev1.ConditionTrue))
				deferred, err = e.reconcile()
				assert.False(t, deferred)
				assert.Empty(t, requeueReason(err))
				assert.Equal(t, "monitor targetHealth", requeueAfterReason(err))
				assert.Equal(t, 1, e.targets.register)
				assert.Equal(t, []string{podBIP}, e.targets.lastRegistered)
				assert.Equal(t, 0, e.targets.deregister)

				// B healthy: gate True, reconcile succeeds, next cycle is a plain early exit.
				e.targets.setHealthy(podBIP)
				_, err = e.reconcile()
				assert.NoError(t, err)
				assert.Equal(t, corev1.ConditionTrue, e.gateB())
				e.targets.resetCounts()
				deferred, err = e.reconcile()
				assert.True(t, deferred)
				assert.NoError(t, err)
				assert.Equal(t, 0, e.targets.calls())
			})
		}
	}

	t.Run("terminating pod on Unknown node: deregistered, no requeue", func(t *testing.T) {
		e := newRealResolverEnv(t, false, true)
		e.registerB()

		e.targets.resetCounts()
		e.setNodeB(readyNodeCond(corev1.ConditionUnknown))
		e.setSlice(false, true)
		deferred, err := e.reconcile()
		assert.False(t, deferred)
		assert.NoError(t, err)
		assert.Equal(t, []string{podBIP}, e.targets.lastDeregistered)

		e.targets.resetCounts()
		deferred, err = e.reconcile()
		assert.True(t, deferred)
		assert.NoError(t, err)
		assert.Equal(t, 0, e.targets.calls())
	})

	t.Run("fail-open, no other ready pod: B on Unknown node registered, no node-not-ready requeue", func(t *testing.T) {
		e := newRealResolverEnv(t, true, false)
		e.setNodeB(readyNodeCond(corev1.ConditionUnknown))
		e.setSlice(false, false)
		deferred, err := e.reconcile()
		assert.False(t, deferred)
		assert.Empty(t, requeueReason(err))
		assert.Equal(t, "monitor targetHealth", requeueAfterReason(err))
		assert.Equal(t, 1, e.targets.register)
		assert.Equal(t, []string{podBIP}, e.targets.lastRegistered)
		assert.Equal(t, corev1.ConditionFalse, e.gateB())

		// B healthy while the node is still Unknown: gate True, loop ends.
		e.targets.setHealthy(podBIP)
		_, err = e.reconcile()
		assert.NoError(t, err)
		assert.Equal(t, corev1.ConditionTrue, e.gateB())
	})
}
