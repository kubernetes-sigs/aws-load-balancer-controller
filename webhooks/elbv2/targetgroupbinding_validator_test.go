package elbv2

import (
	"context"
	"crypto/rand"
	"fmt"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/uuid"
	"math/big"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_targetGroupBindingValidator_ValidateCreate(t *testing.T) {
	targetGroupIPAddressTypeIPv4 := elbv2api.TargetGroupIPAddressTypeIPv4
	targetGroupIPAddressTypeIPv6 := elbv2api.TargetGroupIPAddressTypeIPv6
	type describeTargetGroupsAsListCall struct {
		req  *elbv2sdk.DescribeTargetGroupsInput
		resp []elbv2types.TargetGroup
		err  error
	}

	type fields struct {
		describeTargetGroupsAsListCalls []describeTargetGroupsAsListCall
	}
	type args struct {
		obj *elbv2api.TargetGroupBinding
	}
	instanceTargetType := elbv2api.TargetTypeInstance
	ipTargetType := elbv2api.TargetTypeIP
	clusterVpcID := "vpc-123456ab"
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "targetType is not set",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     nil,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding must specify these fields: spec.targetType"),
		},
		{
			name: "targetType is set",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-2"),
								TargetType:     elbv2types.TargetTypeEnum("instance"),
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "[err] targetType is ip, nodeSelector is set",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
						NodeSelector:   &v1.LabelSelector{},
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding cannot set NodeSelector when TargetType is ip"),
		},
		{
			name: "ipAddressType matches TargetGroup",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-2"),
								TargetType:     elbv2types.TargetTypeEnumInstance,
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv4,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "TargetGroupName can be resolved",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							Names: []string{"tg-name"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn:  awssdk.String("tg-arn"),
								TargetGroupName: awssdk.String("tg-name"),
								TargetType:      elbv2types.TargetTypeEnumInstance,
							},
						},
					},
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-arn"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn:  awssdk.String("tg-arn"),
								TargetGroupName: awssdk.String("tg-name"),
								TargetType:      elbv2types.TargetTypeEnumInstance,
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupName: "tg-name",
						TargetType:      &instanceTargetType,
						IPAddressType:   &targetGroupIPAddressTypeIPv4,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ipAddressType mismatch with TargetGroup",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-2"),
								TargetType:     elbv2types.TargetTypeEnumInstance,
								IpAddressType:  elbv2types.TargetGroupIpAddressTypeEnumIpv4,
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv6,
					},
				},
			},
			wantErr: errors.New("invalid IP address type ipv4 for TargetGroup tg-2"),
		},
		{
			name: "ipAddressType unspecified in tgb, ipv6 in TargetGroup",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-2"),
								TargetType:     elbv2types.TargetTypeEnumInstance,
								IpAddressType:  elbv2types.TargetGroupIpAddressTypeEnumIpv6,
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: errors.New("invalid IP address type ipv6 for TargetGroup tg-2"),
		},
		{
			name: "VpcID in spec matches with TG vpc",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-2"),
								TargetType:     elbv2types.TargetTypeEnumInstance,
								IpAddressType:  elbv2types.TargetGroupIpAddressTypeEnumIpv6,
								VpcId:          &clusterVpcID,
							},
						},
					},
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-2"),
								TargetType:     elbv2types.TargetTypeEnumInstance,
								IpAddressType:  elbv2types.TargetGroupIpAddressTypeEnumIpv6,
								VpcId:          &clusterVpcID,
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv6,
						VpcID:          clusterVpcID,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "VpcID provided doesnt match TG VpcID mismatch",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-2"),
								TargetType:     elbv2types.TargetTypeEnumInstance,
								IpAddressType:  elbv2types.TargetGroupIpAddressTypeEnumIpv6,
								VpcId:          &clusterVpcID,
							},
						},
					},
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-2"),
								TargetType:     elbv2types.TargetTypeEnumInstance,
								IpAddressType:  elbv2types.TargetGroupIpAddressTypeEnumIpv6,
								VpcId:          &clusterVpcID,
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv6,
						VpcID:          "vpc-1234567a",
					},
				},
			},
			wantErr: errors.New("invalid VpcID vpc-1234567a doesnt match VpcID from TargetGroup tg-2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeTargetGroupsAsListCalls {
				elbv2Client.EXPECT().DescribeTargetGroupsAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			v := &targetGroupBindingValidator{
				k8sClient:   k8sClient,
				elbv2Client: elbv2Client,
				logger:      logr.New(&log.NullLogSink{}),
			}
			err := v.ValidateCreate(context.Background(), tt.args.obj)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_targetGroupBindingValidator_ValidateUpdate(t *testing.T) {
	targetGroupIPAddressTypeIPv4 := elbv2api.TargetGroupIPAddressTypeIPv4
	targetGroupIPAddressTypeIPv6 := elbv2api.TargetGroupIPAddressTypeIPv6
	instanceTargetType := elbv2api.TargetTypeInstance
	ipTargetType := elbv2api.TargetTypeIP

	type args struct {
		obj    *elbv2api.TargetGroupBinding
		oldObj *elbv2api.TargetGroupBinding
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "tgb updated removes TargetType",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     nil,
					},
				},
				oldObj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding must specify these fields: spec.targetType"),
		},
		{
			name: "tgb updated mutates TargetGroupARN",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
					},
				},
				oldObj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.targetGroupARN"),
		},
		{
			name: "[err] targetType is ip, nodeSelector is set",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
						NodeSelector:   &v1.LabelSelector{},
					},
				},
				oldObj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
						NodeSelector:   &v1.LabelSelector{},
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding cannot set NodeSelector when TargetType is ip"),
		},
		{
			name: "ipAddressType modified",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv6,
					},
				},
				oldObj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv4,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.ipAddressType"),
		},
		{
			name: "[ok] no update to spec",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
					},
				},
				oldObj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &targetGroupBindingValidator{
				logger: logr.New(&log.NullLogSink{}),
			}
			err := v.ValidateUpdate(context.Background(), tt.args.obj, tt.args.oldObj)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_targetGroupBindingValidator_checkRequiredFields(t *testing.T) {
	type args struct {
		tgb *elbv2api.TargetGroupBinding
	}
	instanceTargetType := elbv2api.TargetTypeInstance
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "targetType is not set",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     nil,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding must specify these fields: spec.targetType"),
		},
		{
			name: "either TargetGroupARN or TargetGroupName must be specified",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN:  "",
						TargetGroupName: "",
						// TargetType:     &ipTargetType,
						TargetType: &instanceTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding must specify these fields: either TargetGroupARN or TargetGroupName"),
		},
		{
			name: "targetType is set",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &targetGroupBindingValidator{
				logger: logr.New(&log.NullLogSink{}),
			}
			err := v.checkRequiredFields(context.Background(), tt.args.tgb)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_targetGroupBindingValidator_checkImmutableFields(t *testing.T) {
	targetGroupIPAddressTypeIPv4 := elbv2api.TargetGroupIPAddressTypeIPv4
	targetGroupIPAddressTypeIPv6 := elbv2api.TargetGroupIPAddressTypeIPv6
	type args struct {
		tgb    *elbv2api.TargetGroupBinding
		oldTGB *elbv2api.TargetGroupBinding
	}
	instanceTargetType := elbv2api.TargetTypeInstance
	ipTargetType := elbv2api.TargetTypeIP
	clusterVpcID := "cluster-vpc-id"
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "targetGroupARN is changed",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.targetGroupARN"),
		},
		{
			name: "targetType is changed",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.targetType"),
		},
		{
			name: "targetType is changed from unset to set",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     nil,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.targetType"),
		},
		{
			name: "targetType is changed from set to unset",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     nil,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.targetType"),
		},
		{
			name: "both targetGroupARN and targetType are changed",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.targetGroupARN,spec.targetType"),
		},
		{
			name: "both targetGroupARN and targetType are not changed",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ipAddressType changed",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv4,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv6,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.ipAddressType"),
		},
		{
			name: "ipAddressType modified, old value nil",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv6,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.ipAddressType"),
		},
		{
			name: "ipAddressType modified from nil to ipv4",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv4,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ipAddressType modified from ipv4 to nil",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv4,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.ipAddressType"),
		},
		{
			name: "ipAddressType modified from nil to ipv6",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv6,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.ipAddressType"),
		},
		{
			name: "VpcID modified from vpc-0aaaaaaa to vpc-0bbbbbbb",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
						VpcID:          "vpc-0bbbbbbb",
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
						VpcID:          "vpc-0aaaaaaa",
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.vpcID"),
		},
		{
			name: "VpcID modified from vpc-0aaaaaaa to nil",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
						VpcID:          "vpc-0aaaaaaa",
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.vpcID"),
		},
		{
			name: "VpcID modified from nil to vpc-0aaaaaaa",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
						VpcID:          "vpc-0aaaaaaa",
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.vpcID"),
		},
		{
			name: "VpcID modified from nil to cluster vpc-id is allowed",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
						VpcID:          clusterVpcID,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &targetGroupBindingValidator{
				logger: logr.New(&log.NullLogSink{}),
				vpcID:  clusterVpcID,
			}
			err := v.checkImmutableFields(tt.args.tgb, tt.args.oldTGB)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_targetGroupBindingValidator_checkNodeSelector(t *testing.T) {
	type args struct {
		tgb *elbv2api.TargetGroupBinding
	}
	instanceTargetType := elbv2api.TargetTypeInstance
	ipTargetType := elbv2api.TargetTypeIP
	nodeSelector := v1.LabelSelector{}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "[ok] targetType is ip, nodeSelector is nil",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-4",
						TargetType:     &ipTargetType,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "[ok] targetType is instance, nodeSelector is nil",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-5",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "[ok] targetType is instance, nodeSelector is set",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-6",
						TargetType:     &instanceTargetType,
						NodeSelector:   &nodeSelector,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "[err] targetType is ip, nodeSelector is set",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-7",
						TargetType:     &ipTargetType,
						NodeSelector:   &nodeSelector,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding cannot set NodeSelector when TargetType is ip"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &targetGroupBindingValidator{
				logger: logr.New(&log.NullLogSink{}),
			}
			err := v.checkNodeSelector(tt.args.tgb)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_targetGroupBindingValidator_checkExistingTargetGroups(t *testing.T) {

	type env struct {
		existingTGBs []elbv2api.TargetGroupBinding
	}

	type args struct {
		tgb *elbv2api.TargetGroupBinding
	}

	tests := []struct {
		name    string
		env     env
		args    args
		wantErr error
	}{
		{
			name: "[ok] no existing target groups",
			env:  env{},
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb1",
						Namespace: "ns1",
					},
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     nil,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "[ok] no duplicate target groups - one target group binding",
			env: env{
				existingTGBs: []elbv2api.TargetGroupBinding{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "tgb1",
							Namespace: "ns1",
						},
						Spec: elbv2api.TargetGroupBindingSpec{
							TargetGroupARN: "tg-1",
							TargetType:     nil,
						},
					},
				},
			},
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb2",
						Namespace: "ns2",
					},
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     nil,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "[ok] no duplicate target groups - multiple target group bindings",
			env: env{
				existingTGBs: []elbv2api.TargetGroupBinding{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "tgb1",
							Namespace: "ns1",
						},
						Spec: elbv2api.TargetGroupBindingSpec{
							TargetGroupARN: "tg-1",
							TargetType:     nil,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "tgb2",
							Namespace: "ns2",
						},
						Spec: elbv2api.TargetGroupBindingSpec{
							TargetGroupARN: "tg-2",
							TargetType:     nil,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "tgb3",
							Namespace: "ns3",
						},
						Spec: elbv2api.TargetGroupBindingSpec{
							TargetGroupARN: "tg-3",
							TargetType:     nil,
						},
					},
				},
			},
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb22",
						Namespace: "ns1",
					},
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-22",
						TargetType:     nil,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "[err] duplicate target groups - one target group binding",
			env: env{
				existingTGBs: []elbv2api.TargetGroupBinding{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "tgb1",
							Namespace: "ns1",
						},
						Spec: elbv2api.TargetGroupBindingSpec{
							TargetGroupARN: "tg-1",
							TargetType:     nil,
						},
					},
				},
			},
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb2",
						Namespace: "ns1",
					},
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     nil,
					},
				},
			},
			wantErr: errors.New("TargetGroup tg-1 is already bound to TargetGroupBinding ns1/tgb1"),
		},
		{
			name: "[err] duplicate target groups - one target group binding",
			env: env{
				existingTGBs: []elbv2api.TargetGroupBinding{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "tgb1",
							Namespace: "ns1",
						},
						Spec: elbv2api.TargetGroupBindingSpec{
							TargetGroupARN: "tg-1",
							TargetType:     nil,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "tgb2",
							Namespace: "ns2",
						},
						Spec: elbv2api.TargetGroupBindingSpec{
							TargetGroupARN: "tg-111",
							TargetType:     nil,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "tgb3",
							Namespace: "ns3",
						},
						Spec: elbv2api.TargetGroupBindingSpec{
							TargetGroupARN: "tg-3",
							TargetType:     nil,
						},
					},
				},
			},
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb111",
						Namespace: "ns1",
					},
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-111",
						TargetType:     nil,
					},
				},
			},
			wantErr: errors.New("TargetGroup tg-111 is already bound to TargetGroupBinding ns2/tgb2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
			v := &targetGroupBindingValidator{
				k8sClient: k8sClient,
				logger:    logr.New(&log.NullLogSink{}),
			}
			for _, tgb := range tt.env.existingTGBs {
				assert.NoError(t, k8sClient.Create(context.Background(), tgb.DeepCopy()))
			}
			err := v.checkExistingTargetGroups(tt.args.tgb)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
		})
	}
}

func Test_targetGroupBindingValidator_checkTargetGroupVpcID(t *testing.T) {
	type args struct {
		obj *elbv2api.TargetGroupBinding
	}
	type describeTargetGroupsAsListCall struct {
		req  *elbv2sdk.DescribeTargetGroupsInput
		resp []elbv2types.TargetGroup
		err  error
	}
	type fields struct {
		describeTargetGroupsAsListCalls []describeTargetGroupsAsListCall
	}
	var (
		vpcID8Chars              = fmt.Sprintf("vpc-%s", generateRandomString(8))
		vpcID17Chars             = fmt.Sprintf("vpc-%s", generateRandomString(17))
		vpcIDUUID                = fmt.Sprintf("vpc-%s", generateVpcUUID())
		vpcID8CharsWrongPrefix   = fmt.Sprintf("vpcid-%s", generateRandomString(8))
		vpcID8CharsIllegalChars  = fmt.Sprintf("vpc-%s", generateRandomString(6, '@', '!'))
		vpcID17CharsWrongPrefix  = fmt.Sprintf("vpcid-%s", generateRandomString(17))
		vpcID17CharsIllegalChars = fmt.Sprintf("vpc-%s", generateRandomString(15, 'G', 'L'))
		vpcIDUUIDWrongPrefix     = fmt.Sprintf("vpcid-%s", generateRandomString(32))
		vpcIDUUIDIllegalChars    = fmt.Sprintf("vpc-%s", generateRandomString(30, 'z', 'Y'))
	)

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "[ok] Valid VpcID with 8 Characters",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						resp: []elbv2types.TargetGroup{
							{
								VpcId: awssdk.String(vpcID8Chars),
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						VpcID:          vpcID8Chars,
					},
				},
			},
		},
		{
			name: "[ok] Valid VpcID with 17 Characters",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						resp: []elbv2types.TargetGroup{
							{
								VpcId: awssdk.String(vpcID17Chars),
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						VpcID:          vpcID17Chars,
					},
				},
			},
		},
		{
			name: "[ok] Valid VpcID with UUID",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						resp: []elbv2types.TargetGroup{
							{
								VpcId: awssdk.String(vpcIDUUID),
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						VpcID:          vpcIDUUID,
					},
				},
			},
		},
		{
			name: "[err] Provided VpcID doesn't match VpcID from TargetGroup",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						resp: []elbv2types.TargetGroup{
							{
								VpcId: awssdk.String(fmt.Sprintf("vpc-%s", generateRandomString(17))),
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						VpcID:          vpcID17Chars,
					},
				},
			},
			wantErr: fmt.Errorf(vpcIDNotMatchErr, vpcID17Chars, "tg-2"),
		},
		{
			name: "[ok] VpcID is not set",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{},
				},
			},
			wantErr: nil,
		},
		{
			name: "[err] vpcID is not found",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-2"},
						},
						err: errors.New("vpcid not found"),
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						VpcID:          "vpc-b234567a",
					},
				},
			},
			wantErr: errors.New("unable to get target group VpcID: vpcid not found"),
		},
		{
			name: "[err] vpcID 8 chars is not valid - invalid prefix",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						VpcID:          vpcID8CharsWrongPrefix,
					},
				},
			},
			wantErr: errors.New(fmt.Sprintf(vpcIDValidationErr, vpcID8CharsWrongPrefix)),
		},
		{
			name: "[err] vpcID 8 chars is not valid - illegal chars",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						VpcID:          vpcID8CharsIllegalChars,
					},
				},
			},
			wantErr: errors.New(fmt.Sprintf(vpcIDValidationErr, vpcID8CharsIllegalChars)),
		},
		{
			name: "[err] vpcID 17 chars is not valid - invalid prefix",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						VpcID:          vpcID17CharsWrongPrefix,
					},
				},
			},
			wantErr: errors.New(fmt.Sprintf(vpcIDValidationErr, vpcID17CharsWrongPrefix)),
		},
		{
			name: "[err] vpcID 17 chars is not valid - illegal chars",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						VpcID:          vpcID17CharsIllegalChars,
					},
				},
			},
			wantErr: errors.New(fmt.Sprintf(vpcIDValidationErr, vpcID17CharsIllegalChars)),
		},
		{
			name: "[err] vpcID UUID is not valid - invalid prefix",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						VpcID:          vpcIDUUIDWrongPrefix,
					},
				},
			},
			wantErr: errors.New(fmt.Sprintf(vpcIDValidationErr, vpcIDUUIDWrongPrefix)),
		},
		{
			name: "[err] vpcID UUID is not valid - illegal chars",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						VpcID:          vpcIDUUIDIllegalChars,
					},
				},
			},
			wantErr: errors.New(fmt.Sprintf(vpcIDValidationErr, vpcIDUUIDIllegalChars)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeTargetGroupsAsListCalls {
				elbv2Client.EXPECT().DescribeTargetGroupsAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			v := &targetGroupBindingValidator{
				k8sClient:   k8sClient,
				elbv2Client: elbv2Client,
				logger:      logr.New(&log.NullLogSink{}),
			}
			err := v.checkTargetGroupVpcID(context.Background(), tt.args.obj)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func generateRandomString(n int, addChars ...rune) string {
	const letters = "0123456789abcdef"

	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return ""
		}
		ret[i] = letters[num.Int64()]
	}

	w := string(ret)
	for _, c := range addChars {
		w += string(c)
	}
	return w
}

func generateVpcUUID() string {
	u := strings.ToLower(uuid.New().String())
	return strings.Replace(u, "-", "", -1)
}
