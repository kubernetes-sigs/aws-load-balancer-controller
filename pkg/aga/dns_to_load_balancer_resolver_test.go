package aga

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"testing"
	"time"
)

func TestDNSToLoadBalancerResolver_ResolveDNSToLoadBalancerARN(t *testing.T) {
	type describeLoadBalancersAsListCall struct {
		req  *elbv2sdk.DescribeLoadBalancersInput
		resp []types.LoadBalancer
		err  error
	}

	type fields struct {
		elbv2Client                *services.MockELBV2
		describeLoadBalancersCalls []describeLoadBalancersAsListCall
	}

	tests := []struct {
		name        string
		fields      fields
		dnsName     string
		wantARN     string
		wantErr     bool
		setupFields func(fields fields)
	}{
		{
			name: "successfully resolves DNS to ARN",
			fields: fields{
				elbv2Client: services.NewMockELBV2(gomock.NewController(t)),
				describeLoadBalancersCalls: []describeLoadBalancersAsListCall{
					{
						req: &elbv2sdk.DescribeLoadBalancersInput{},
						resp: []types.LoadBalancer{
							{
								DNSName:         awssdk.String("test-lb.us-west-2.elb.amazonaws.com"),
								LoadBalancerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-lb/1234567890abcdef"),
							},
							{
								DNSName:         awssdk.String("another-lb.us-west-2.elb.amazonaws.com"),
								LoadBalancerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/another-lb/0987654321fedcba"),
							},
						},
						err: nil,
					},
				},
			},
			dnsName: "test-lb.us-west-2.elb.amazonaws.com",
			wantARN: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-lb/1234567890abcdef",
			wantErr: false,
			setupFields: func(fields fields) {
				gomock.InOrder(
					fields.elbv2Client.EXPECT().
						DescribeLoadBalancersAsList(gomock.Any(), fields.describeLoadBalancersCalls[0].req).
						Return(fields.describeLoadBalancersCalls[0].resp, fields.describeLoadBalancersCalls[0].err),
				)
			},
		},
		{
			name: "uses cached ARN on second call",
			fields: fields{
				elbv2Client: services.NewMockELBV2(gomock.NewController(t)),
				describeLoadBalancersCalls: []describeLoadBalancersAsListCall{
					{
						req: &elbv2sdk.DescribeLoadBalancersInput{},
						resp: []types.LoadBalancer{
							{
								DNSName:         awssdk.String("test-lb.us-west-2.elb.amazonaws.com"),
								LoadBalancerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-lb/1234567890abcdef"),
							},
						},
						err: nil,
					},
				},
			},
			dnsName: "test-lb.us-west-2.elb.amazonaws.com",
			wantARN: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-lb/1234567890abcdef",
			wantErr: false,
			setupFields: func(fields fields) {
				gomock.InOrder(
					fields.elbv2Client.EXPECT().
						DescribeLoadBalancersAsList(gomock.Any(), fields.describeLoadBalancersCalls[0].req).
						Return(fields.describeLoadBalancersCalls[0].resp, fields.describeLoadBalancersCalls[0].err).
						Times(1),
				)
			},
		},
		{
			name: "returns error for empty DNS name",
			fields: fields{
				elbv2Client:                services.NewMockELBV2(gomock.NewController(t)),
				describeLoadBalancersCalls: []describeLoadBalancersAsListCall{},
			},
			dnsName: "",
			wantARN: "",
			wantErr: true,
			setupFields: func(fields fields) {
				// No calls expected for empty DNS name
			},
		},
		{
			name: "returns error when no load balancers found",
			fields: fields{
				elbv2Client: services.NewMockELBV2(gomock.NewController(t)),
				describeLoadBalancersCalls: []describeLoadBalancersAsListCall{
					{
						req:  &elbv2sdk.DescribeLoadBalancersInput{},
						resp: []types.LoadBalancer{},
						err:  nil,
					},
				},
			},
			dnsName: "test-lb.us-west-2.elb.amazonaws.com",
			wantARN: "",
			wantErr: true,
			setupFields: func(fields fields) {
				gomock.InOrder(
					fields.elbv2Client.EXPECT().
						DescribeLoadBalancersAsList(gomock.Any(), fields.describeLoadBalancersCalls[0].req).
						Return(fields.describeLoadBalancersCalls[0].resp, fields.describeLoadBalancersCalls[0].err),
				)
			},
		},
		{
			name: "returns error when no matching load balancer found",
			fields: fields{
				elbv2Client: services.NewMockELBV2(gomock.NewController(t)),
				describeLoadBalancersCalls: []describeLoadBalancersAsListCall{
					{
						req: &elbv2sdk.DescribeLoadBalancersInput{},
						resp: []types.LoadBalancer{
							{
								DNSName:         awssdk.String("another-lb.us-west-2.elb.amazonaws.com"),
								LoadBalancerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/another-lb/0987654321fedcba"),
							},
						},
						err: nil,
					},
				},
			},
			dnsName: "test-lb.us-west-2.elb.amazonaws.com",
			wantARN: "",
			wantErr: true,
			setupFields: func(fields fields) {
				gomock.InOrder(
					fields.elbv2Client.EXPECT().
						DescribeLoadBalancersAsList(gomock.Any(), fields.describeLoadBalancersCalls[0].req).
						Return(fields.describeLoadBalancersCalls[0].resp, fields.describeLoadBalancersCalls[0].err),
				)
			},
		},
		{
			name: "returns error when API call fails",
			fields: fields{
				elbv2Client: services.NewMockELBV2(gomock.NewController(t)),
				describeLoadBalancersCalls: []describeLoadBalancersAsListCall{
					{
						req:  &elbv2sdk.DescribeLoadBalancersInput{},
						resp: nil,
						err:  errors.New("API error"),
					},
				},
			},
			dnsName: "test-lb.us-west-2.elb.amazonaws.com",
			wantARN: "",
			wantErr: true,
			setupFields: func(fields fields) {
				gomock.InOrder(
					fields.elbv2Client.EXPECT().
						DescribeLoadBalancersAsList(gomock.Any(), fields.describeLoadBalancersCalls[0].req).
						Return(fields.describeLoadBalancersCalls[0].resp, fields.describeLoadBalancersCalls[0].err),
				)
			},
		},
		{
			name: "successfully resolves NLB DNS to ARN",
			fields: fields{
				elbv2Client: services.NewMockELBV2(gomock.NewController(t)),
				describeLoadBalancersCalls: []describeLoadBalancersAsListCall{
					{
						req: &elbv2sdk.DescribeLoadBalancersInput{},
						resp: []types.LoadBalancer{
							{
								DNSName:         awssdk.String("test-nlb.us-west-2.elb.us-west-2.amazonaws.aws"),
								LoadBalancerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/test-nlb/1234567890abcdef"),
							},
							{
								DNSName:         awssdk.String("another-lb.us-west-2.elb.amazonaws.com"),
								LoadBalancerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/another-lb/0987654321fedcba"),
							},
						},
						err: nil,
					},
				},
			},
			dnsName: "test-nlb.us-west-2.elb.us-west-2.amazonaws.aws",
			wantARN: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/test-nlb/1234567890abcdef",
			wantErr: false,
			setupFields: func(fields fields) {
				gomock.InOrder(
					fields.elbv2Client.EXPECT().
						DescribeLoadBalancersAsList(gomock.Any(), fields.describeLoadBalancersCalls[0].req).
						Return(fields.describeLoadBalancersCalls[0].resp, fields.describeLoadBalancersCalls[0].err),
				)
			},
		},
	}

	// Add a test case for cache expiration
	t.Run("cache expiration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		elbv2Client := services.NewMockELBV2(ctrl)
		dnsName := "expired-lb.us-west-2.elb.amazonaws.com"
		originalARN := "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/expired-lb/original"
		updatedARN := "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/expired-lb/updated"

		// Create resolver with a small TTL for testing
		resolver, err := NewDNSToLoadBalancerResolver(elbv2Client)
		assert.NoError(t, err)

		// Override the TTL for testing
		resolver.ttl = 10 * time.Millisecond

		// First call, should resolve through API
		elbv2Client.EXPECT().
			DescribeLoadBalancersAsList(gomock.Any(), &elbv2sdk.DescribeLoadBalancersInput{}).
			Return([]types.LoadBalancer{
				{
					DNSName:         awssdk.String(dnsName),
					LoadBalancerArn: awssdk.String(originalARN),
				},
			}, nil).
			Times(1)

		gotARN1, err := resolver.ResolveDNSToLoadBalancerARN(context.Background(), dnsName)
		assert.NoError(t, err)
		assert.Equal(t, originalARN, gotARN1)

		// Wait for cache to expire
		time.Sleep(15 * time.Millisecond)

		// Second call after cache expiry, should resolve through API again
		elbv2Client.EXPECT().
			DescribeLoadBalancersAsList(gomock.Any(), &elbv2sdk.DescribeLoadBalancersInput{}).
			Return([]types.LoadBalancer{
				{
					DNSName:         awssdk.String(dnsName),
					LoadBalancerArn: awssdk.String(updatedARN), // Different ARN to verify re-resolution
				},
			}, nil).
			Times(1)

		gotARN2, err := resolver.ResolveDNSToLoadBalancerARN(context.Background(), dnsName)
		assert.NoError(t, err)
		assert.Equal(t, updatedARN, gotARN2, "ARN should be updated after cache expiry")
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupFields(tt.fields)

			resolver, err := NewDNSToLoadBalancerResolver(tt.fields.elbv2Client)
			assert.NoError(t, err)

			// For cache test, we need to call it twice
			if tt.name == "uses cached ARN on second call" {
				// First call
				gotARN, err := resolver.ResolveDNSToLoadBalancerARN(context.Background(), tt.dnsName)
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.wantARN, gotARN)
				}

				// Second call - should use cache
				gotARN, err = resolver.ResolveDNSToLoadBalancerARN(context.Background(), tt.dnsName)
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.wantARN, gotARN)
				}
			} else {
				// Regular test
				gotARN, err := resolver.ResolveDNSToLoadBalancerARN(context.Background(), tt.dnsName)
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.wantARN, gotARN)
				}
			}
		})
	}
}
