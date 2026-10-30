package networking

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
)

func Test_defaultEIPResolver_ResolveForSubnets(t *testing.T) {
	tagFilters := map[string]string{
		"pod":        "pod998",
		"service":    "zorg",
		"visibility": "external",
	}

	subnets := []ec2types.Subnet{
		{
			SubnetId:         aws.String("subnet-1"),
			AvailabilityZone: aws.String("us-west-2a"),
		},
		{
			SubnetId:         aws.String("subnet-2"),
			AvailabilityZone: aws.String("us-west-2b"),
		},
	}

	tests := []struct {
		name        string
		tagFilters  map[string]string
		subnets     []ec2types.Subnet
		addresses   []ec2types.Address
		describeErr error
		want        []string
		wantErr     string
	}{
		{
			name:       "resolves one EIP per subnet AZ",
			tagFilters: tagFilters,
			subnets:    subnets,
			addresses: []ec2types.Address{
				{
					AllocationId:       aws.String("eipalloc-aaa"),
					NetworkBorderGroup: aws.String("us-west-2a"),
				},
				{
					AllocationId:       aws.String("eipalloc-bbb"),
					NetworkBorderGroup: aws.String("us-west-2b"),
				},
			},
			want: []string{"eipalloc-aaa", "eipalloc-bbb"},
		},
		{
			name:       "allows EIPs associated with NLB",
			tagFilters: tagFilters,
			subnets:    subnets,
			addresses: []ec2types.Address{
				{
					AllocationId:            aws.String("eipalloc-aaa"),
					NetworkBorderGroup:      aws.String("us-west-2a"),
					AssociationId:           aws.String("eipassoc-1"),
					NetworkInterfaceOwnerId: aws.String("amazon-elb"),
				},
				{
					AllocationId:            aws.String("eipalloc-bbb"),
					NetworkBorderGroup:      aws.String("us-west-2b"),
					AssociationId:           aws.String("eipassoc-2"),
					NetworkInterfaceOwnerId: aws.String("amazon-elb"),
				},
			},
			want: []string{"eipalloc-aaa", "eipalloc-bbb"},
		},
		{
			name:       "empty discovery tags",
			tagFilters: map[string]string{},
			subnets:    subnets,
			wantErr:    "EIP discovery tags must not be empty",
		},
		{
			name:       "empty subnets",
			tagFilters: tagFilters,
			subnets:    nil,
			wantErr:    "subnets must not be empty for EIP discovery",
		},
		{
			name:       "missing EIP for subnet AZ",
			tagFilters: tagFilters,
			subnets:    subnets,
			addresses: []ec2types.Address{
				{
					AllocationId:       aws.String("eipalloc-aaa"),
					NetworkBorderGroup: aws.String("us-west-2a"),
				},
			},
			wantErr: "no EIP found for subnet subnet-2 in availability zone us-west-2b matching discovery tags",
		},
		{
			name:       "multiple EIPs in same AZ",
			tagFilters: tagFilters,
			subnets:    subnets[:1],
			addresses: []ec2types.Address{
				{
					AllocationId:       aws.String("eipalloc-aaa"),
					NetworkBorderGroup: aws.String("us-west-2a"),
				},
				{
					AllocationId:       aws.String("eipalloc-aab"),
					NetworkBorderGroup: aws.String("us-west-2a"),
				},
			},
			wantErr: "multiple EIPs found for availability zone us-west-2a matching discovery tags",
		},
		{
			name:       "EIP associated with another resource",
			tagFilters: tagFilters,
			subnets:    subnets[:1],
			addresses: []ec2types.Address{
				{
					AllocationId:            aws.String("eipalloc-aaa"),
					NetworkBorderGroup:      aws.String("us-west-2a"),
					AssociationId:           aws.String("eipassoc-1"),
					NetworkInterfaceOwnerId: aws.String("amazon-aws"),
				},
			},
			wantErr: "EIP eipalloc-aaa is associated with another resource",
		},
		{
			name:        "describe addresses error",
			tagFilters:  tagFilters,
			subnets:     subnets,
			describeErr: errors.New("boom"),
			wantErr:     "failed to list EIPs by discovery tags: boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			if tt.tagFilters != nil && len(tt.tagFilters) > 0 && len(tt.subnets) > 0 {
				ec2Client.EXPECT().DescribeAddressesAsList(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, input *ec2sdk.DescribeAddressesInput) ([]ec2types.Address, error) {
						if tt.describeErr != nil {
							return nil, tt.describeErr
						}
						assert.Equal(t, "domain", aws.ToString(input.Filters[0].Name))
						assert.Equal(t, []string{"vpc"}, input.Filters[0].Values)
						return tt.addresses, nil
					},
				)
			}

			resolver := NewDefaultEIPResolver(ec2Client)
			got, err := resolver.ResolveForSubnets(context.Background(), tt.tagFilters, tt.subnets)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
