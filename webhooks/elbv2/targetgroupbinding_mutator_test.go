package elbv2

import (
	"context"
	"sync"
	"testing"

	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func makeTargetGroupBinding(tgARN string) *elbv2api.TargetGroupBinding {
	return &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{},
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetGroupARN: tgARN,
		},
	}
}

func Test_targetGroupBindingMutator_MutateCreate(t *testing.T) {
	type describeTargetGroupsAsListCall struct {
		req  *elbv2sdk.DescribeTargetGroupsInput
		resp []elbv2types.TargetGroup
		err  error
	}

	type fields struct {
		describeTargetGroupsAsListCalls []describeTargetGroupsAsListCall
	}

	targetGroupIPAddressTypeIPv4 := elbv2api.TargetGroupIPAddressTypeIPv4
	targetGroupIPAddressTypeIPv6 := elbv2api.TargetGroupIPAddressTypeIPv6
	instanceTargetType := elbv2api.TargetTypeInstance
	ipTargetType := elbv2api.TargetTypeIP
	type args struct {
		obj *elbv2api.TargetGroupBinding
	}
	httpProtocol := elbv2.ProtocolHTTP
	tests := []struct {
		name       string
		fields     fields
		args       args
		want       *elbv2api.TargetGroupBinding
		wantErr    error
		wantMetric bool
	}{
		{
			name: "targetGroupBinding with all fields already set",
			fields: fields{
				describeTargetGroupsAsListCalls: nil,
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN:      "tg-1",
						TargetType:          &instanceTargetType,
						IPAddressType:       &targetGroupIPAddressTypeIPv4,
						VpcID:               "vpcid-01",
						TargetGroupProtocol: &httpProtocol,
					},
				},
			},
			want: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN:      "tg-1",
					TargetType:          &instanceTargetType,
					IPAddressType:       &targetGroupIPAddressTypeIPv4,
					VpcID:               "vpcid-01",
					TargetGroupProtocol: &httpProtocol,
				},
			},
		},
		{
			name: "targetGroupBinding with TargetType absent will be defaulted via AWS API - instance",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-1"),
								TargetType:     elbv2types.TargetTypeEnumInstance,
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN:      "tg-1",
						TargetGroupProtocol: &httpProtocol,
						TargetType:          nil,
					},
				},
			},
			want: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN:      "tg-1",
					TargetType:          &instanceTargetType,
					IPAddressType:       &targetGroupIPAddressTypeIPv4,
					TargetGroupProtocol: &httpProtocol,
				},
			},
		},
		{
			name: "targetGroupBinding with TargetType absent will be defaulted via AWS API - ip",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-1"),
								TargetType:     elbv2types.TargetTypeEnumIp,
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN:      "tg-1",
						TargetGroupProtocol: &httpProtocol,
						TargetType:          nil,
					},
				},
			},
			want: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN:      "tg-1",
					TargetType:          &ipTargetType,
					IPAddressType:       &targetGroupIPAddressTypeIPv4,
					TargetGroupProtocol: &httpProtocol,
				},
			},
		},
		{
			name: "targetGroupBinding with TargetType absent will be defaulted via AWS API - lambda",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-1"),
								TargetType:     elbv2types.TargetTypeEnumLambda,
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN:      "tg-1",
						TargetType:          nil,
						TargetGroupProtocol: &httpProtocol,
					},
				},
			},
			wantErr:    errors.New("unsupported TargetType: lambda"),
			wantMetric: true,
		},
		{
			name: "targetGroupBinding with IPAddressType already set to ipv6",
			fields: fields{
				describeTargetGroupsAsListCalls: nil,
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN:      "tg-1",
						TargetType:          &instanceTargetType,
						IPAddressType:       &targetGroupIPAddressTypeIPv6,
						VpcID:               "vpcid-01",
						TargetGroupProtocol: &httpProtocol,
					},
				},
			},
			want: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN:      "tg-1",
					TargetType:          &instanceTargetType,
					IPAddressType:       &targetGroupIPAddressTypeIPv6,
					VpcID:               "vpcid-01",
					TargetGroupProtocol: &httpProtocol,
				},
			},
		},
		{
			name: "targetGroupBinding with VpcID absent will be defaulted via AWS API",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						resp: []elbv2types.TargetGroup{
							{
								VpcId: awssdk.String("vpcid-01"),
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN:      "tg-1",
						TargetType:          &instanceTargetType,
						IPAddressType:       &targetGroupIPAddressTypeIPv4,
						TargetGroupProtocol: &httpProtocol,
					},
				},
			},
			want: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN:      "tg-1",
					TargetType:          &instanceTargetType,
					IPAddressType:       &targetGroupIPAddressTypeIPv4,
					VpcID:               "vpcid-01",
					TargetGroupProtocol: &httpProtocol,
				},
			},
		},
		{
			name: "targetGroupBinding with VpcID absent will be defaulted via AWS API - error",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						err: errors.New("vpcid not found"),
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv4,
					},
				},
			},
			wantErr:    errors.New("unable to get target group VpcID: vpcid not found"),
			wantMetric: true,
		},
		{
			name: "targetGroupBinding with TargetGroupName instead of TargetGroupARN",
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
								TargetGroupArn: awssdk.String("tg-arn"),
								TargetType:     elbv2types.TargetTypeEnumInstance,
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupName:     "tg-name",
						TargetType:          &instanceTargetType,
						IPAddressType:       &targetGroupIPAddressTypeIPv4,
						TargetGroupProtocol: &httpProtocol,
					},
				},
			},
			want: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN:      "tg-arn",
					TargetGroupName:     "tg-name",
					TargetType:          &instanceTargetType,
					IPAddressType:       &targetGroupIPAddressTypeIPv4,
					TargetGroupProtocol: &httpProtocol,
				},
			},
		},
		{
			name: "targetGroupBinding with protocol absent will be defaulted via AWS API",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						resp: []elbv2types.TargetGroup{
							{
								Protocol: elbv2types.ProtocolEnumHttp,
							},
						},
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv4,
						VpcID:          "vpcid-01",
					},
				},
			},
			want: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN:      "tg-1",
					TargetType:          &instanceTargetType,
					IPAddressType:       &targetGroupIPAddressTypeIPv4,
					VpcID:               "vpcid-01",
					TargetGroupProtocol: &httpProtocol,
				},
			},
		},
		{
			name: "targetGroupBinding with protocol absent will be defaulted via AWS API - error",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						err: errors.New("connection error"),
					},
				},
			},
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
						IPAddressType:  &targetGroupIPAddressTypeIPv4,
						VpcID:          "vpcid-01",
					},
				},
			},
			wantErr:    errors.New("couldn't determine TargetGroup protocol: connection error"),
			wantMetric: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			ctx := context.Background()
			for _, call := range tt.fields.describeTargetGroupsAsListCalls {
				elbv2Client.EXPECT().DescribeTargetGroupsAsList(gomock.Any(), call.req).Return(call.resp, call.err).AnyTimes()
				elbv2Client.EXPECT().AssumeRole(ctx, gomock.Any(), gomock.Any()).Return(elbv2Client, nil).AnyTimes()
			}
			mockMetricsCollector := lbcmetrics.NewMockCollector()
			m := &targetGroupBindingMutator{
				elbv2Client:      elbv2Client,
				logger:           logr.New(&log.NullLogSink{}),
				metricsCollector: mockMetricsCollector,
			}
			got, err := m.MutateCreate(context.Background(), tt.args.obj)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			mockCollector := m.metricsCollector.(*lbcmetrics.MockCollector)
			assert.Equal(t, tt.wantMetric, len(mockCollector.Invocations[lbcmetrics.MetricWebhookMutationFailure]) == 1)
		})

	}
}

func Test_targetGroupBindingMutator_obtainSDKTargetTypeFromAWS(t *testing.T) {
	type describeTargetGroupsAsListCall struct {
		req  *elbv2sdk.DescribeTargetGroupsInput
		resp []elbv2types.TargetGroup
		err  error
	}

	type fields struct {
		describeTargetGroupsAsListCalls []describeTargetGroupsAsListCall
	}
	type args struct {
		tgARN string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr error
	}{
		{
			name: "standard case - instance targetType",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetType: elbv2types.TargetTypeEnumInstance,
							},
						},
					},
				},
			},
			args: args{
				tgARN: "tg-1",
			},
			want: "instance",
		},
		{
			name: "standard case - ip targetType",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetType: elbv2types.TargetTypeEnumIp,
							},
						},
					},
				},
			},
			args: args{
				tgARN: "tg-1",
			},
			want: "ip",
		},
		{
			name: "some error during describeTargetGroupCall",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						err: errors.New("targetGroup not found"),
					},
				},
			},
			args: args{
				tgARN: "tg-1",
			},
			wantErr: errors.New("targetGroup not found"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ctx := context.Background()

			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeTargetGroupsAsListCalls {
				elbv2Client.EXPECT().DescribeTargetGroupsAsList(gomock.Any(), call.req).Return(call.resp, call.err)
				elbv2Client.EXPECT().AssumeRole(ctx, gomock.Any(), gomock.Any()).Return(elbv2Client, nil).AnyTimes()
			}
			mockMetricsCollector := lbcmetrics.NewMockCollector()
			m := &targetGroupBindingMutator{
				elbv2Client:      elbv2Client,
				logger:           logr.New(&log.NullLogSink{}),
				metricsCollector: mockMetricsCollector,
			}

			tgb := makeTargetGroupBinding(tt.args.tgARN)
			targetGroupCache := sync.OnceValue(func() tgCacheObject {
				targetGroup, err := getTargetGroupFromAWS(ctx, m.elbv2Client, tgb)
				return tgCacheObject{
					tg:    targetGroup,
					error: err,
				}
			})

			got, err := m.obtainSDKTargetTypeFromAWS(targetGroupCache)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_targetGroupBindingMutator_getIPAddressTypeFromAWS(t *testing.T) {
	type describeTargetGroupsAsListCall struct {
		req  *elbv2sdk.DescribeTargetGroupsInput
		resp []elbv2types.TargetGroup
		err  error
	}

	type fields struct {
		describeTargetGroupsAsListCalls []describeTargetGroupsAsListCall
	}
	type args struct {
		tgARN string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    elbv2api.TargetGroupIPAddressType
		wantErr error
	}{
		{
			name: "target ip address type empty",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetType: elbv2types.TargetTypeEnumInstance,
							},
						},
					},
				},
			},
			args: args{
				tgARN: "tg-1",
			},
			want: "ipv4",
		},
		{
			name: "target ip address type ipv4",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetType:    elbv2types.TargetTypeEnumIp,
								IpAddressType: elbv2types.TargetGroupIpAddressTypeEnumIpv4,
							},
						},
					},
				},
			},
			args: args{
				tgARN: "tg-1",
			},
			want: "ipv4",
		},
		{
			name: "target ip address type ipv6",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						resp: []elbv2types.TargetGroup{
							{
								TargetType:    elbv2types.TargetTypeEnumIp,
								IpAddressType: elbv2types.TargetGroupIpAddressTypeEnumIpv6,
							},
						},
					},
				},
			},
			args: args{
				tgARN: "tg-1",
			},
			want: "ipv6",
		},
		{
			name: "some error during describeTargetGroupCall",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						err: errors.New("targetGroup not found"),
					},
				},
			},
			args: args{
				tgARN: "tg-1",
			},
			wantErr: errors.New("targetGroup not found"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ctx := context.Background()

			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeTargetGroupsAsListCalls {
				elbv2Client.EXPECT().DescribeTargetGroupsAsList(gomock.Any(), call.req).Return(call.resp, call.err)
				elbv2Client.EXPECT().AssumeRole(ctx, gomock.Any(), gomock.Any()).Return(elbv2Client, nil).AnyTimes()
			}

			mockMetricsCollector := lbcmetrics.NewMockCollector()
			m := &targetGroupBindingMutator{
				elbv2Client:      elbv2Client,
				logger:           logr.New(&log.NullLogSink{}),
				metricsCollector: mockMetricsCollector,
			}
			tgb := makeTargetGroupBinding(tt.args.tgARN)
			targetGroupCache := sync.OnceValue(func() tgCacheObject {
				targetGroup, err := getTargetGroupFromAWS(ctx, m.elbv2Client, tgb)
				return tgCacheObject{
					tg:    targetGroup,
					error: err,
				}
			})
			got, err := m.getTargetGroupIPAddressTypeFromAWS(targetGroupCache)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_targetGroupBindingMutator_obtainSDKVpcIDFromAWS(t *testing.T) {
	type describeTargetGroupsAsListCall struct {
		req  *elbv2sdk.DescribeTargetGroupsInput
		resp []elbv2types.TargetGroup
		err  error
	}

	type fields struct {
		describeTargetGroupsAsListCalls []describeTargetGroupsAsListCall
	}
	type args struct {
		tgARN string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr error
	}{
		{
			name: "fetch vpcid from aws",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						resp: []elbv2types.TargetGroup{
							{
								VpcId: awssdk.String("vpcid-01"),
							},
						},
					},
				},
			},
			args: args{
				tgARN: "tg-1",
			},
			want: "vpcid-01",
		},
		{
			name: "some error while fetching vpcId",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{
							TargetGroupArns: []string{"tg-1"},
						},
						err: errors.New("vpcid not found"),
					},
				},
			},
			args: args{
				tgARN: "tg-1",
			},
			wantErr: errors.New("vpcid not found"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ctx := context.Background()

			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeTargetGroupsAsListCalls {
				elbv2Client.EXPECT().DescribeTargetGroupsAsList(gomock.Any(), call.req).Return(call.resp, call.err)
				elbv2Client.EXPECT().AssumeRole(ctx, gomock.Any(), gomock.Any()).Return(elbv2Client, nil).AnyTimes()
			}
			mockMetricsCollector := lbcmetrics.NewMockCollector()
			m := &targetGroupBindingMutator{
				elbv2Client:      elbv2Client,
				logger:           logr.New(&log.NullLogSink{}),
				metricsCollector: mockMetricsCollector,
			}
			tgb := makeTargetGroupBinding(tt.args.tgARN)
			targetGroupCache := sync.OnceValue(func() tgCacheObject {
				targetGroup, err := getTargetGroupFromAWS(ctx, m.elbv2Client, tgb)
				return tgCacheObject{
					tg:    targetGroup,
					error: err,
				}
			})
			got, err := m.getVpcIDFromAWS(targetGroupCache)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
