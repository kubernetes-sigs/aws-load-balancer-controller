package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultLoadBalancerAttributeReconciler_updateSDKLoadBalancerWithAttributes(t *testing.T) {
	type describeLoadBalancerAttributesWithContextCall struct {
		req  *elbv2sdk.DescribeLoadBalancerAttributesInput
		resp *elbv2sdk.DescribeLoadBalancerAttributesOutput
		err  error
	}

	type modifyLoadBalancerAttributesWithContextCall struct {
		req  *elbv2sdk.ModifyLoadBalancerAttributesInput
		resp *elbv2sdk.ModifyLoadBalancerAttributesOutput
		err  error
	}

	type fields struct {
		describeLoadBalancerAttributesWithContextCalls []describeLoadBalancerAttributesWithContextCall
		modifyLoadBalancerAttributesWithContextCalls   []modifyLoadBalancerAttributesWithContextCall
	}
	type args struct {
		sdkLB LoadBalancerWithTags
		resLB *elbv2model.LoadBalancer
	}

	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "multiple attributes should be updated",
			fields: fields{
				describeLoadBalancerAttributesWithContextCalls: []describeLoadBalancerAttributesWithContextCall{
					{
						req: &elbv2sdk.DescribeLoadBalancerAttributesInput{
							LoadBalancerArn: awssdk.String("my-arn"),
						},
						resp: &elbv2sdk.DescribeLoadBalancerAttributesOutput{
							Attributes: []*elbv2sdk.LoadBalancerAttribute{
								{
									Key:   awssdk.String("idle_timeout.timeout_seconds"),
									Value: awssdk.String("50"),
								},
								{
									Key:   awssdk.String("load_balancing.cross_zone.enabled"),
									Value: awssdk.String("false"),
								},
							},
						},
					},
				},
				modifyLoadBalancerAttributesWithContextCalls: []modifyLoadBalancerAttributesWithContextCall{
					{
						req: &elbv2sdk.ModifyLoadBalancerAttributesInput{
							LoadBalancerArn: awssdk.String("my-arn"),
							Attributes: []*elbv2sdk.LoadBalancerAttribute{
								{
									Key:   awssdk.String("idle_timeout.timeout_seconds"),
									Value: awssdk.String("100"),
								},
								{
									Key:   awssdk.String("load_balancing.cross_zone.enabled"),
									Value: awssdk.String("true"),
								},
							},
						},
					},
				},
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
					Spec: elbv2model.LoadBalancerSpec{
						LoadBalancerAttributes: []elbv2model.LoadBalancerAttribute{
							{
								Key:   "idle_timeout.timeout_seconds",
								Value: "100",
							},
							{
								Key:   "load_balancing.cross_zone.enabled",
								Value: "true",
							},
						},
					},
				},
			},
		},
		{
			name: "no attributes should be updated",
			fields: fields{
				describeLoadBalancerAttributesWithContextCalls: []describeLoadBalancerAttributesWithContextCall{
					{
						req: &elbv2sdk.DescribeLoadBalancerAttributesInput{
							LoadBalancerArn: awssdk.String("my-arn"),
						},
						resp: &elbv2sdk.DescribeLoadBalancerAttributesOutput{
							Attributes: []*elbv2sdk.LoadBalancerAttribute{
								{
									Key:   awssdk.String("idle_timeout.timeout_seconds"),
									Value: awssdk.String("50"),
								},
								{
									Key:   awssdk.String("load_balancing.cross_zone.enabled"),
									Value: awssdk.String("false"),
								},
							},
						},
					},
				},
				modifyLoadBalancerAttributesWithContextCalls: nil,
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
					Spec: elbv2model.LoadBalancerSpec{
						LoadBalancerAttributes: []elbv2model.LoadBalancerAttribute{
							{
								Key:   "idle_timeout.timeout_seconds",
								Value: "50",
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeLoadBalancerAttributesWithContextCalls {
				elbv2Client.EXPECT().DescribeLoadBalancerAttributesWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			for _, call := range tt.fields.modifyLoadBalancerAttributesWithContextCalls {
				elbv2Client.EXPECT().ModifyLoadBalancerAttributesWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			r := &defaultLoadBalancerAttributeReconciler{
				elbv2Client: elbv2Client,
				logger:      &log.NullLogger{},
			}
			err := r.Reconcile(context.Background(), tt.args.resLB, tt.args.sdkLB)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_defaultLoadBalancerAttributeReconciler_getDesiredLoadBalancerAttributes(t *testing.T) {
	type args struct {
		resLB *elbv2model.LoadBalancer
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "standard case",
			args: args{
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						LoadBalancerAttributes: []elbv2model.LoadBalancerAttribute{
							{
								Key:   "keyA",
								Value: "valueA",
							},
							{
								Key:   "keyB",
								Value: "valueB",
							},
						},
					},
				},
			},
			want: map[string]string{
				"keyA": "valueA",
				"keyB": "valueB",
			},
		},
		{
			name: "nil attributes case",
			args: args{
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						LoadBalancerAttributes: nil,
					},
				},
			},
			want: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &defaultLoadBalancerAttributeReconciler{}
			got := r.getDesiredLoadBalancerAttributes(context.Background(), tt.args.resLB)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultLoadBalancerAttributeReconciler_getCurrentLoadBalancerAttributes(t *testing.T) {
	type describeLoadBalancerAttributesWithContextCall struct {
		req  *elbv2sdk.DescribeLoadBalancerAttributesInput
		resp *elbv2sdk.DescribeLoadBalancerAttributesOutput
		err  error
	}
	type fields struct {
		describeLoadBalancerAttributesWithContextCalls []describeLoadBalancerAttributesWithContextCall
	}
	type args struct {
		sdkLB LoadBalancerWithTags
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[string]string
		wantErr error
	}{
		{
			name: "standard case",
			fields: fields{
				describeLoadBalancerAttributesWithContextCalls: []describeLoadBalancerAttributesWithContextCall{
					{
						req: &elbv2sdk.DescribeLoadBalancerAttributesInput{
							LoadBalancerArn: awssdk.String("my-arn"),
						},
						resp: &elbv2sdk.DescribeLoadBalancerAttributesOutput{
							Attributes: []*elbv2sdk.LoadBalancerAttribute{
								{
									Key:   awssdk.String("keyA"),
									Value: awssdk.String("valueA"),
								},
								{
									Key:   awssdk.String("keyB"),
									Value: awssdk.String("valueB"),
								},
							},
						},
					},
				},
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
					Tags: nil,
				},
			},
			want: map[string]string{
				"keyA": "valueA",
				"keyB": "valueB",
			},
		},
		{
			name: "error case",
			fields: fields{
				describeLoadBalancerAttributesWithContextCalls: []describeLoadBalancerAttributesWithContextCall{
					{
						req: &elbv2sdk.DescribeLoadBalancerAttributesInput{
							LoadBalancerArn: awssdk.String("my-arn"),
						},
						err: errors.New("some error"),
					},
				},
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
					Tags: nil,
				},
			},
			wantErr: errors.New("some error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeLoadBalancerAttributesWithContextCalls {
				elbv2Client.EXPECT().DescribeLoadBalancerAttributesWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			r := &defaultLoadBalancerAttributeReconciler{
				elbv2Client: elbv2Client,
			}
			got, err := r.getCurrentLoadBalancerAttributes(context.Background(), tt.args.sdkLB)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
