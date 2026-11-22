package shield

import (
	"context"
	shieldtypes "github.com/aws/aws-sdk-go-v2/service/shield/types"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	shieldsdk "github.com/aws/aws-sdk-go-v2/service/shield"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_defaultProtectionManager_IsSubscribed(t *testing.T) {
	type getSubscriptionStateCall struct {
		req  *shieldsdk.GetSubscriptionStateInput
		resp *shieldsdk.GetSubscriptionStateOutput
		err  error
	}
	type fields struct {
		subscriptionStateCacheTTL time.Duration
		getSubscriptionStateCalls []getSubscriptionStateCall
	}
	type isSubscribedCall struct {
		want    bool
		wantErr error
	}
	tests := []struct {
		name              string
		fields            fields
		isSubscribedCalls []isSubscribedCall
	}{
		{
			name: "invoke isSubscribed once without cache - subscriptionState == ACTIVE",
			fields: fields{
				subscriptionStateCacheTTL: 2 * time.Hour,
				getSubscriptionStateCalls: []getSubscriptionStateCall{
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						resp: &shieldsdk.GetSubscriptionStateOutput{
							SubscriptionState: shieldtypes.SubscriptionStateActive,
						},
					},
				},
			},
			isSubscribedCalls: []isSubscribedCall{
				{
					want: true,
				},
			},
		},
		{
			name: "invoke isSubscribed once without cache - subscriptionState == INACTIVE",
			fields: fields{
				subscriptionStateCacheTTL: 2 * time.Hour,
				getSubscriptionStateCalls: []getSubscriptionStateCall{
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						resp: &shieldsdk.GetSubscriptionStateOutput{
							SubscriptionState: shieldtypes.SubscriptionStateInactive,
						},
					},
				},
			},
			isSubscribedCalls: []isSubscribedCall{
				{
					want: false,
				},
			},
		},
		{
			name: "invoke isSubscribed once without cache - AWS API error",
			fields: fields{
				subscriptionStateCacheTTL: 2 * time.Hour,
				getSubscriptionStateCalls: []getSubscriptionStateCall{
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						err: errors.New("some aws api error"),
					},
				},
			},
			isSubscribedCalls: []isSubscribedCall{
				{
					wantErr: errors.New("some aws api error"),
				},
			},
		},
		{
			name: "invoke isSubscribed twice with cache - two call within cacheTTL",
			fields: fields{
				subscriptionStateCacheTTL: 2 * time.Hour,
				getSubscriptionStateCalls: []getSubscriptionStateCall{
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						resp: &shieldsdk.GetSubscriptionStateOutput{
							SubscriptionState: shieldtypes.SubscriptionStateInactive,
						},
					},
				},
			},
			isSubscribedCalls: []isSubscribedCall{
				{
					want: false,
				},
				{
					want: false,
				},
			},
		},
		{
			name: "invoke isSubscribed twice with cache - two call beyond cacheTTL",
			fields: fields{
				subscriptionStateCacheTTL: 0,
				getSubscriptionStateCalls: []getSubscriptionStateCall{
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						resp: &shieldsdk.GetSubscriptionStateOutput{
							SubscriptionState: shieldtypes.SubscriptionStateInactive,
						},
					},
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						resp: &shieldsdk.GetSubscriptionStateOutput{
							SubscriptionState: shieldtypes.SubscriptionStateActive,
						},
					},
				},
			},
			isSubscribedCalls: []isSubscribedCall{
				{
					want: false,
				},
				{
					want: true,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			shieldClient := services.NewMockShield(ctrl)
			for _, call := range tt.fields.getSubscriptionStateCalls {
				shieldClient.EXPECT().GetSubscriptionStateWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			m := &defaultProtectionManager{
				shieldClient:              shieldClient,
				logger:                    logr.New(&log.NullLogSink{}),
				subscriptionStateCache:    cache.NewExpiring(),
				subscriptionStateCacheTTL: tt.fields.subscriptionStateCacheTTL,
			}
			for _, call := range tt.isSubscribedCalls {
				got, err := m.IsSubscribed(context.Background())
				if call.wantErr != nil {
					assert.EqualError(t, err, call.wantErr.Error())
				} else {
					assert.NoError(t, err)
					assert.Equal(t, call.want, got)
				}
			}
		})
	}
}

func Test_defaultProtectionManager_ResolveProtectionResourceARNs(t *testing.T) {
	const nlbARN = "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/my-nlb/abc123"
	type describeLBCalls struct {
		req  *elbv2sdk.DescribeLoadBalancersInput
		resp []elbv2types.LoadBalancer
		err  error
	}
	tests := []struct {
		name        string
		resourceARN string
		describeLBs []describeLBCalls
		want        []string
		wantErr     string
	}{
		{
			name:        "non-NLB ARN is returned unchanged",
			resourceARN: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-alb/abc123",
			want:        []string{"arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-alb/abc123"},
		},
		{
			name:        "NLB ARN resolves to EIP allocation ARNs, deduped across AZs",
			resourceARN: nlbARN,
			describeLBs: []describeLBCalls{
				{
					req: &elbv2sdk.DescribeLoadBalancersInput{LoadBalancerArns: []string{nlbARN}},
					resp: []elbv2types.LoadBalancer{
						{
							AvailabilityZones: []elbv2types.AvailabilityZone{
								{
									LoadBalancerAddresses: []elbv2types.LoadBalancerAddress{
										{AllocationId: awssdk.String("eipalloc-1")},
										{AllocationId: awssdk.String("eipalloc-2")},
										{IpAddress: awssdk.String("2001:db8::1")}, // IPv6: no allocation ID
									},
								},
								{
									LoadBalancerAddresses: []elbv2types.LoadBalancerAddress{
										{AllocationId: awssdk.String("eipalloc-1")}, // duplicate across AZs
									},
								},
							},
						},
					},
				},
			},
			want: []string{
				"arn:aws:ec2:us-west-2:123456789012:eip-allocation/eipalloc-1",
				"arn:aws:ec2:us-west-2:123456789012:eip-allocation/eipalloc-2",
			},
		},
		{
			name:        "NLB without EIPs resolves to an empty target set",
			resourceARN: nlbARN,
			describeLBs: []describeLBCalls{
				{
					req: &elbv2sdk.DescribeLoadBalancersInput{LoadBalancerArns: []string{nlbARN}},
					resp: []elbv2types.LoadBalancer{
						{
							AvailabilityZones: []elbv2types.AvailabilityZone{
								{LoadBalancerAddresses: []elbv2types.LoadBalancerAddress{{IpAddress: awssdk.String("52.1.2.3")}}},
							},
						},
					},
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.describeLBs {
				elbv2Client.EXPECT().DescribeLoadBalancersAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			m := &defaultProtectionManager{
				elbv2Client: elbv2Client,
				logger:      logr.New(&log.NullLogSink{}),
			}
			got, err := m.ResolveProtectionResourceARNs(context.Background(), tt.resourceARN)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
