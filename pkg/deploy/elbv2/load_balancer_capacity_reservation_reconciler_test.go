package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultLoadBalancerCapacityReservationReconciler_updateSDKLoadBalancerWithCapacityReservation(t *testing.T) {
	type describeCapacityReservationWithContextCall struct {
		req  *elbv2sdk.DescribeCapacityReservationInput
		resp *elbv2sdk.DescribeCapacityReservationOutput
		err  error
	}

	type modifyCapacityReservationWithContextCall struct {
		req  *elbv2sdk.ModifyCapacityReservationInput
		resp *elbv2sdk.ModifyCapacityReservationOutput
		err  error
	}

	type fields struct {
		describeCapacityReservationWithContextCalls []describeCapacityReservationWithContextCall
		modifyCapacityReservationWithContextCalls   []modifyCapacityReservationWithContextCall
	}
	type args struct {
		sdkLB LoadBalancerWithTags
		resLB *elbv2model.LoadBalancer
	}

	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
	tests := []struct {
		name         string
		featureGates map[config.Feature]bool
		fields       fields
		args         args
		wantErr      error
	}{
		{
			name: "capacity reservation should be updated",
			featureGates: map[config.Feature]bool{
				config.LBCapacityReservation: true,
			},
			fields: fields{
				describeCapacityReservationWithContextCalls: []describeCapacityReservationWithContextCall{
					{
						req: &elbv2sdk.DescribeCapacityReservationInput{
							LoadBalancerArn: awssdk.String("my-arn"),
						},
						resp: &elbv2sdk.DescribeCapacityReservationOutput{
							CapacityReservationState:    nil,
							MinimumLoadBalancerCapacity: nil,
						},
					},
				},
				modifyCapacityReservationWithContextCalls: []modifyCapacityReservationWithContextCall{
					{
						req: &elbv2sdk.ModifyCapacityReservationInput{
							LoadBalancerArn:             awssdk.String("my-arn"),
							MinimumLoadBalancerCapacity: &elbv2types.MinimumLoadBalancerCapacity{CapacityUnits: awssdk.Int32(1200)},
						},
					},
				},
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
					Spec: elbv2model.LoadBalancerSpec{
						MinimumLoadBalancerCapacity: &elbv2model.MinimumLoadBalancerCapacity{CapacityUnits: 1200},
					},
				},
			},
		},
		{
			name: "reset capacity with zero value",
			featureGates: map[config.Feature]bool{
				config.LBCapacityReservation: true,
			},
			fields: fields{
				describeCapacityReservationWithContextCalls: []describeCapacityReservationWithContextCall{
					{
						req: &elbv2sdk.DescribeCapacityReservationInput{
							LoadBalancerArn: awssdk.String("my-arn"),
						},
						resp: &elbv2sdk.DescribeCapacityReservationOutput{
							CapacityReservationState:    []elbv2types.ZonalCapacityReservationState{},
							MinimumLoadBalancerCapacity: &elbv2types.MinimumLoadBalancerCapacity{CapacityUnits: awssdk.Int32(1200)},
						},
					},
				},
				modifyCapacityReservationWithContextCalls: []modifyCapacityReservationWithContextCall{
					{
						req: &elbv2sdk.ModifyCapacityReservationInput{
							LoadBalancerArn:          awssdk.String("my-arn"),
							ResetCapacityReservation: awssdk.Bool(true),
						},
					},
				},
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
					Spec: elbv2model.LoadBalancerSpec{
						MinimumLoadBalancerCapacity: &elbv2model.MinimumLoadBalancerCapacity{CapacityUnits: 0},
					},
				},
			},
		},
		{
			name: "no capacity reservation should be updated as their is no change",
			featureGates: map[config.Feature]bool{
				config.LBCapacityReservation: true,
			},
			fields: fields{
				describeCapacityReservationWithContextCalls: []describeCapacityReservationWithContextCall{
					{
						req: &elbv2sdk.DescribeCapacityReservationInput{
							LoadBalancerArn: awssdk.String("my-arn"),
						},
						resp: &elbv2sdk.DescribeCapacityReservationOutput{
							CapacityReservationState:    []elbv2types.ZonalCapacityReservationState{},
							MinimumLoadBalancerCapacity: &elbv2types.MinimumLoadBalancerCapacity{CapacityUnits: awssdk.Int32(1200)},
						},
					},
				},
				modifyCapacityReservationWithContextCalls: nil,
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
					Spec: elbv2model.LoadBalancerSpec{
						MinimumLoadBalancerCapacity: &elbv2model.MinimumLoadBalancerCapacity{CapacityUnits: 1200},
					},
				},
			},
		},
		{
			name: "no capacity reservation should be updated as their is no specification on resource",
			featureGates: map[config.Feature]bool{
				config.LBCapacityReservation: true,
			},
			fields: fields{
				describeCapacityReservationWithContextCalls: nil,
				modifyCapacityReservationWithContextCalls:   nil,
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
					Spec: elbv2model.LoadBalancerSpec{
						MinimumLoadBalancerCapacity: nil,
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

			for _, call := range tt.fields.describeCapacityReservationWithContextCalls {
				elbv2Client.EXPECT().DescribeCapacityReservationWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			for _, call := range tt.fields.modifyCapacityReservationWithContextCalls {
				elbv2Client.EXPECT().ModifyCapacityReservationWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			r := &defaultLoadBalancerCapacityReservationReconciler{
				elbv2Client: elbv2Client,
				logger:      logr.New(&log.NullLogSink{}),
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

func Test_defaultLoadBalancerCapacityReservationReconciler_getCurrentLoadBalancerCapacityReservation(t *testing.T) {
	type describeCapacityReservationWithContextCall struct {
		req  *elbv2sdk.DescribeCapacityReservationInput
		resp *elbv2sdk.DescribeCapacityReservationOutput
		err  error
	}
	type fields struct {
		describeCapacityReservationWithContextCalls []describeCapacityReservationWithContextCall
	}
	type args struct {
		sdkLB LoadBalancerWithTags
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *elbv2model.MinimumLoadBalancerCapacity
		wantErr error
	}{
		{
			name: "no capacity reservation case",
			fields: fields{
				describeCapacityReservationWithContextCalls: []describeCapacityReservationWithContextCall{
					{
						req: &elbv2sdk.DescribeCapacityReservationInput{
							LoadBalancerArn: awssdk.String("my-arn"),
						},
						resp: &elbv2sdk.DescribeCapacityReservationOutput{
							CapacityReservationState:    nil,
							MinimumLoadBalancerCapacity: nil,
						},
					},
				},
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
					Tags: nil,
				},
			},
			want: nil,
		},
		{
			name: "standard case",
			fields: fields{
				describeCapacityReservationWithContextCalls: []describeCapacityReservationWithContextCall{
					{
						req: &elbv2sdk.DescribeCapacityReservationInput{
							LoadBalancerArn: awssdk.String("my-arn"),
						},
						resp: &elbv2sdk.DescribeCapacityReservationOutput{
							CapacityReservationState:    []elbv2types.ZonalCapacityReservationState{},
							MinimumLoadBalancerCapacity: &elbv2types.MinimumLoadBalancerCapacity{CapacityUnits: awssdk.Int32(3000)},
						},
					},
				},
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
					Tags: nil,
				},
			},
			want: &elbv2model.MinimumLoadBalancerCapacity{CapacityUnits: 3000},
		},
		{
			name: "error case",
			fields: fields{
				describeCapacityReservationWithContextCalls: []describeCapacityReservationWithContextCall{
					{
						req: &elbv2sdk.DescribeCapacityReservationInput{
							LoadBalancerArn: awssdk.String("my-arn"),
						},
						err: errors.New("some error"),
					},
				},
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
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
			featureGates := config.NewFeatureGates()
			for _, call := range tt.fields.describeCapacityReservationWithContextCalls {
				elbv2Client.EXPECT().DescribeCapacityReservationWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			r := &defaultLoadBalancerCapacityReservationReconciler{
				elbv2Client:  elbv2Client,
				featureGates: featureGates,
			}
			got, err := r.getCurrentCapacityReservation(context.Background(), tt.args.sdkLB)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
