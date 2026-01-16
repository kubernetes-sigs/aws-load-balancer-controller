package networking

import (
	"context"
	"errors"
	"testing"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_defaultSubnetsResolver_ResolveViaDiscovery(t *testing.T) {
	type describeSubnetsAsListCall struct {
		input  *ec2sdk.DescribeSubnetsInput
		output []ec2types.Subnet
		err    error
	}
	type describeRouteTablesAsListCall struct {
		input  *ec2sdk.DescribeRouteTablesInput
		output []ec2types.RouteTable
		err    error
	}
	type fetchAZInfosCall struct {
		availabilityZoneIDs []string
		azInfoByAZID        map[string]ec2types.AvailabilityZone
		err                 error
	}
	type fields struct {
		clusterTagCheckEnabled         bool
		albSingleSubnetEnabled         bool
		discoveryByReachabilityEnabled bool
		describeSubnetsAsListCalls     []describeSubnetsAsListCall
		describeRouteTablesAsListCalls []describeRouteTablesAsListCall
		fetchAZInfosCalls              []fetchAZInfosCall
	}
	type args struct {
		opts []SubnetsResolveOption
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []ec2types.Subnet
		wantErr error
	}{
		{
			name: "alb/internet-facing, discovered via role tag",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: []string{"", "1"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/role/elb"),
							Value: awssdk.String("1"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/role/elb"),
							Value: awssdk.String("1"),
						},
					},
				},
			},
		},
		{
			name: "alb/internal, discovered via role tag",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:kubernetes.io/role/internal-elb"),
									Values: []string{"", "1"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/internal-elb"),
										Value: awssdk.String("1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/internal-elb"),
										Value: awssdk.String("1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/role/internal-elb"),
							Value: awssdk.String("1"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/role/internal-elb"),
							Value: awssdk.String("1"),
						},
					},
				},
			},
		},
		{
			name: "alb/internet-facing, discovered via fallback to reachability",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: []string{"", "1"},
								},
							},
						},
						output: nil,
					},
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-3"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-4"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
						},
					},
				},
				describeRouteTablesAsListCalls: []describeRouteTablesAsListCall{
					{
						input: &ec2sdk.DescribeRouteTablesInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
							},
						},
						output: []ec2types.RouteTable{
							{
								RouteTableId: awssdk.String("rtb-main"),
								Associations: []ec2types.RouteTableAssociation{
									{
										Main: awssdk.Bool(true),
									},
								},
							},
							{
								RouteTableId: awssdk.String("rtb-public"),
								Associations: []ec2types.RouteTableAssociation{
									{
										Main:     awssdk.Bool(false),
										SubnetId: awssdk.String("subnet-1"),
									},
									{
										Main:     awssdk.Bool(false),
										SubnetId: awssdk.String("subnet-2"),
									},
								},
								Routes: []ec2types.Route{
									{
										GatewayId: awssdk.String("igw-xxx"),
									},
								},
							},
							{
								RouteTableId: awssdk.String("rtb-private"),
								Associations: []ec2types.RouteTableAssociation{
									{
										Main: awssdk.Bool(false),
									},
									{
										Main:     awssdk.Bool(false),
										SubnetId: awssdk.String("subnet-3"),
									},
									{
										Main:     awssdk.Bool(false),
										SubnetId: awssdk.String("subnet-4"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
			},
		},
		{
			name: "alb/internal, discovered via fallback to reachability",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:kubernetes.io/role/internal-elb"),
									Values: []string{"", "1"},
								},
							},
						},
						output: nil,
					},
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-3"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-4"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
						},
					},
				},
				describeRouteTablesAsListCalls: []describeRouteTablesAsListCall{
					{
						input: &ec2sdk.DescribeRouteTablesInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
							},
						},
						output: []ec2types.RouteTable{
							{
								RouteTableId: awssdk.String("rtb-main"),
								Associations: []ec2types.RouteTableAssociation{
									{
										Main: awssdk.Bool(true),
									},
								},
							},
							{
								RouteTableId: awssdk.String("rtb-public"),
								Associations: []ec2types.RouteTableAssociation{
									{
										Main:     awssdk.Bool(false),
										SubnetId: awssdk.String("subnet-1"),
									},
									{
										Main:     awssdk.Bool(false),
										SubnetId: awssdk.String("subnet-2"),
									},
								},
								Routes: []ec2types.Route{
									{
										GatewayId: awssdk.String("igw-xxx"),
									},
								},
							},
							{
								RouteTableId: awssdk.String("rtb-private"),
								Associations: []ec2types.RouteTableAssociation{
									{
										Main: awssdk.Bool(false),
									},
									{
										Main:     awssdk.Bool(false),
										SubnetId: awssdk.String("subnet-3"),
									},
									{
										Main:     awssdk.Bool(false),
										SubnetId: awssdk.String("subnet-4"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-3"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-4"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
			},
		},
		{
			name: "subnets tagged for other clusters shall be filtered out when clusterTagCheckEnabled enabled",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: []string{"", "1"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
									{
										Key:   awssdk.String("kubernetes.io/cluster/other-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-3"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/role/elb"),
							Value: awssdk.String("1"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-3"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/role/elb"),
							Value: awssdk.String("1"),
						},
					},
				},
			},
		},
		{
			name: "subnets tagged for other clusters shall not be filtered out when clusterTagCheckEnabled disabled",
			fields: fields{
				clusterTagCheckEnabled:         false,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: []string{"", "1"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
									{
										Key:   awssdk.String("kubernetes.io/cluster/other-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-3"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/role/elb"),
							Value: awssdk.String("1"),
						},
						{
							Key:   awssdk.String("kubernetes.io/cluster/other-cluster"),
							Value: awssdk.String("owned"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/role/elb"),
							Value: awssdk.String("1"),
						},
					},
				},
			},
		},
		{
			name: "subnets with insufficient available ip addresses shall be filtered out",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: []string{"", "1"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(2),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-3"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/role/elb"),
							Value: awssdk.String("1"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-3"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/role/elb"),
							Value: awssdk.String("1"),
						},
					},
				},
			},
		},
		{
			name: "subnets are either tagged for other clusters or with insufficient ip address",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: []string{"", "1"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
									{
										Key:   awssdk.String("kubernetes.io/cluster/other-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(2),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			wantErr: errors.New("unable to resolve at least one subnet. Evaluated 2 subnets: 1 are tagged for other clusters, and 1 have insufficient available IP addresses"),
		},
		{
			name: "multiple subnets found per AZ, pick one per az",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: []string{"", "1"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
									{
										Key:   awssdk.String("kubernetes.io/cluster/cluster-dummy"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-3"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-4"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/role/elb"),
							Value: awssdk.String("1"),
						},
						{
							Key:   awssdk.String("kubernetes.io/cluster/cluster-dummy"),
							Value: awssdk.String("owned"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-3"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/role/elb"),
							Value: awssdk.String("1"),
						},
					},
				},
			},
		},
		{
			name: "fallback to reachability were disabled",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: false,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: []string{"", "1"},
								},
							},
						},
						output: nil,
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			wantErr: errors.New("unable to resolve at least one subnet. Evaluated 0 subnets: 0 are tagged for other clusters, and 0 have insufficient available IP addresses"),
		},
		{
			name: "failed to list subnets by role tag",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: []string{"", "1"},
								},
							},
						},
						err: errors.New("some auth error"),
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			wantErr: errors.New("failed to list subnets by role tag: some auth error"),
		},
		{
			name: "failed to fallback to reachability",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: []string{"", "1"},
								},
							},
						},
						output: nil,
					},
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-3"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-4"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
						},
					},
				},
				describeRouteTablesAsListCalls: []describeRouteTablesAsListCall{
					{
						input: &ec2sdk.DescribeRouteTablesInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
							},
						},
						err: errors.New("some error"),
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			wantErr: errors.New("failed to list subnets by reachability: some error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeSubnetsAsListCalls {
				ec2Client.EXPECT().DescribeSubnetsAsList(gomock.Any(), call.input).Return(call.output, call.err)
			}
			for _, call := range tt.fields.describeRouteTablesAsListCalls {
				ec2Client.EXPECT().DescribeRouteTablesAsList(gomock.Any(), call.input).Return(call.output, call.err)
			}
			azInfoProvider := NewMockAZInfoProvider(ctrl)
			for _, call := range tt.fields.fetchAZInfosCalls {
				azInfoProvider.EXPECT().FetchAZInfos(gomock.Any(), call.availabilityZoneIDs).Return(call.azInfoByAZID, call.err)
			}
			r := NewDefaultSubnetsResolver(azInfoProvider, ec2Client, "vpc-dummy", "cluster-dummy",
				tt.fields.clusterTagCheckEnabled, tt.fields.albSingleSubnetEnabled, tt.fields.discoveryByReachabilityEnabled,
				logr.New(&log.NullLogSink{}))
			got, err := r.ResolveViaDiscovery(context.Background(), tt.args.opts...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opts := cmp.Options{
					cmpopts.IgnoreUnexported(ec2types.Subnet{}),
					cmpopts.IgnoreUnexported(ec2types.Tag{}),
				}
				assert.True(t, cmp.Equal(tt.want, got, opts), "diff", cmp.Diff(tt.want, got, opts))
			}
		})
	}
}

func Test_defaultSubnetsResolver_ResolveViaSelector(t *testing.T) {
	type describeSubnetsAsListCall struct {
		input  *ec2sdk.DescribeSubnetsInput
		output []ec2types.Subnet
		err    error
	}
	type fetchAZInfosCall struct {
		availabilityZoneIDs []string
		azInfoByAZID        map[string]ec2types.AvailabilityZone
		err                 error
	}
	type fields struct {
		clusterTagCheckEnabled         bool
		albSingleSubnetEnabled         bool
		discoveryByReachabilityEnabled bool
		describeSubnetsAsListCalls     []describeSubnetsAsListCall
		fetchAZInfosCalls              []fetchAZInfosCall
	}
	type args struct {
		selector elbv2api.SubnetSelector
		opts     []SubnetsResolveOption
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []ec2types.Subnet
		wantErr error
	}{
		{
			name: "resolved via subnetIDs",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-1", "subnet-2"},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				selector: elbv2api.SubnetSelector{
					IDs: []elbv2api.SubnetID{"subnet-1", "subnet-2"},
				},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
			},
		},
		{
			name: "subnets specified via ID are in same AZ",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-1", "subnet-2"},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
						},
					},
				},
			},
			args: args{
				selector: elbv2api.SubnetSelector{
					IDs: []elbv2api.SubnetID{"subnet-1", "subnet-2"},
				},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			wantErr: errors.New("multiple subnets in same Availability Zone us-west-2a: [subnet-1 subnet-2]"),
		},
		{
			name: "listSubnetsByIDs failed",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-1", "subnet-2"},
						},
						err: errors.New("some error"),
					},
				},
			},
			args: args{
				selector: elbv2api.SubnetSelector{
					IDs: []elbv2api.SubnetID{"subnet-1", "subnet-2"},
				},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			wantErr: errors.New("failed to list subnets by IDs: some error"),
		},
		{
			name: "resolved via tag selector",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:tagA"),
									Values: []string{"tagAVal1", "tagAVal2"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				selector: elbv2api.SubnetSelector{
					Tags: map[string][]string{
						"tagA": {"tagAVal1", "tagAVal2"},
					},
				},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("tagA"),
							Value: awssdk.String("tagAVal1"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("tagA"),
							Value: awssdk.String("tagAVal1"),
						},
					},
				},
			},
		},
		{
			name: "subnets tagged for other clusters shall be filtered out when clusterTagCheckEnabled enabled",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:tagA"),
									Values: []string{"tagAVal1", "tagAVal2"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
									{
										Key:   awssdk.String("kubernetes.io/cluster/other-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-3"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				selector: elbv2api.SubnetSelector{
					Tags: map[string][]string{
						"tagA": {"tagAVal1", "tagAVal2"},
					},
				},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("tagA"),
							Value: awssdk.String("tagAVal1"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-3"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("tagA"),
							Value: awssdk.String("tagAVal1"),
						},
					},
				},
			},
		},
		{
			name: "subnets tagged for other clusters shall not be filtered out when clusterTagCheckEnabled disabled",
			fields: fields{
				clusterTagCheckEnabled:         false,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:tagA"),
									Values: []string{"tagAVal1", "tagAVal2"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
									{
										Key:   awssdk.String("kubernetes.io/cluster/other-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-3"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				selector: elbv2api.SubnetSelector{
					Tags: map[string][]string{
						"tagA": {"tagAVal1", "tagAVal2"},
					},
				},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("tagA"),
							Value: awssdk.String("tagAVal1"),
						},
						{
							Key:   awssdk.String("kubernetes.io/cluster/other-cluster"),
							Value: awssdk.String("owned"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("tagA"),
							Value: awssdk.String("tagAVal1"),
						},
					},
				},
			},
		},
		{
			name: "subnets with insufficient available ip addresses shall be filtered out",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:tagA"),
									Values: []string{"tagAVal1", "tagAVal2"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(2),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-3"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				selector: elbv2api.SubnetSelector{
					Tags: map[string][]string{
						"tagA": {"tagAVal1", "tagAVal2"},
					},
				},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("tagA"),
							Value: awssdk.String("tagAVal1"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-3"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("tagA"),
							Value: awssdk.String("tagAVal1"),
						},
					},
				},
			},
		},
		{
			name: "subnets are either tagged for other clusters or with insufficient ip address",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:tagA"),
									Values: []string{"tagAVal1", "tagAVal2"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
									{
										Key:   awssdk.String("kubernetes.io/cluster/other-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(2),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("tagA"),
										Value: awssdk.String("tagAVal1"),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				selector: elbv2api.SubnetSelector{
					Tags: map[string][]string{
						"tagA": {"tagAVal1", "tagAVal2"},
					},
				},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			wantErr: errors.New("unable to resolve at least one subnet. Evaluated 2 subnets: 1 are tagged for other clusters, and 1 have insufficient available IP addresses"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeSubnetsAsListCalls {
				ec2Client.EXPECT().DescribeSubnetsAsList(gomock.Any(), call.input).Return(call.output, call.err)
			}
			azInfoProvider := NewMockAZInfoProvider(ctrl)
			for _, call := range tt.fields.fetchAZInfosCalls {
				azInfoProvider.EXPECT().FetchAZInfos(gomock.Any(), call.availabilityZoneIDs).Return(call.azInfoByAZID, call.err)
			}
			r := NewDefaultSubnetsResolver(azInfoProvider, ec2Client, "vpc-dummy", "cluster-dummy",
				tt.fields.clusterTagCheckEnabled, tt.fields.albSingleSubnetEnabled, tt.fields.discoveryByReachabilityEnabled,
				logr.New(&log.NullLogSink{}))
			got, err := r.ResolveViaSelector(context.Background(), tt.args.selector, tt.args.opts...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opts := cmp.Options{
					cmpopts.IgnoreUnexported(ec2types.Subnet{}),
					cmpopts.IgnoreUnexported(ec2types.Tag{}),
				}
				assert.True(t, cmp.Equal(tt.want, got, opts), "diff", cmp.Diff(tt.want, got, opts))
			}
		})
	}
}

func Test_defaultSubnetsResolver_ResolveViaNameOrIDSlice(t *testing.T) {
	type describeSubnetsAsListCall struct {
		input  *ec2sdk.DescribeSubnetsInput
		output []ec2types.Subnet
		err    error
	}
	type fetchAZInfosCall struct {
		availabilityZoneIDs []string
		azInfoByAZID        map[string]ec2types.AvailabilityZone
		err                 error
	}
	type fields struct {
		clusterTagCheckEnabled         bool
		albSingleSubnetEnabled         bool
		discoveryByReachabilityEnabled bool
		describeSubnetsAsListCalls     []describeSubnetsAsListCall
		fetchAZInfosCalls              []fetchAZInfosCall
	}
	type args struct {
		nameOrIDs []string
		opts      []SubnetsResolveOption
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []ec2types.Subnet
		wantErr error
	}{
		{
			name: "resolved via subnetIDs",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-1", "subnet-2"},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				nameOrIDs: []string{"subnet-1", "subnet-2"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
			},
		},
		{
			name: "resolved via subnetNames",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:Name"),
									Values: []string{"name-1", "name-2"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("Name"),
										Value: awssdk.String("name-1"),
									},
								},
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("Name"),
										Value: awssdk.String("name-2"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				nameOrIDs: []string{"name-1", "name-2"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("Name"),
							Value: awssdk.String("name-1"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("Name"),
							Value: awssdk.String("name-2"),
						},
					},
				},
			},
		},
		{
			name: "resolved via both id and name",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-1"},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
						},
					},
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{"vpc-dummy"},
								},
								{
									Name:   awssdk.String("tag:Name"),
									Values: []string{"name-2"},
								},
							},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("Name"),
										Value: awssdk.String("name-2"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				nameOrIDs: []string{"subnet-1", "name-2"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("Name"),
							Value: awssdk.String("name-2"),
						},
					},
				},
			},
		},
		{
			name: "order is preserved when AWS returns subnets in different order",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-3", "subnet-1", "subnet-2"},
						},
						// AWS returns in different order than requested
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-3"),
								AvailabilityZone:        awssdk.String("us-west-2c"),
								AvailabilityZoneId:      awssdk.String("usw2-az3"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az3"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az3": {
								ZoneId:   awssdk.String("usw2-az3"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				nameOrIDs: []string{"subnet-3", "subnet-1", "subnet-2"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			// Expected result must be in the requested order, not AWS's order
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-3"),
					AvailabilityZone:        awssdk.String("us-west-2c"),
					AvailabilityZoneId:      awssdk.String("usw2-az3"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
			},
		},
		{
			name: "subnets specified are in same AZ",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-1", "subnet-2"},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
							},
						},
					},
				},
			},
			args: args{
				nameOrIDs: []string{"subnet-1", "subnet-2"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			wantErr: errors.New("multiple subnets in same Availability Zone us-west-2a: [subnet-1 subnet-2]"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeSubnetsAsListCalls {
				ec2Client.EXPECT().DescribeSubnetsAsList(gomock.Any(), call.input).Return(call.output, call.err)
			}
			azInfoProvider := NewMockAZInfoProvider(ctrl)
			for _, call := range tt.fields.fetchAZInfosCalls {
				azInfoProvider.EXPECT().FetchAZInfos(gomock.Any(), call.availabilityZoneIDs).Return(call.azInfoByAZID, call.err)
			}
			r := NewDefaultSubnetsResolver(azInfoProvider, ec2Client, "vpc-dummy", "cluster-dummy",
				tt.fields.clusterTagCheckEnabled, tt.fields.albSingleSubnetEnabled, tt.fields.discoveryByReachabilityEnabled,
				logr.New(&log.NullLogSink{}))
			got, err := r.ResolveViaNameOrIDSlice(context.Background(), tt.args.nameOrIDs, tt.args.opts...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opts := cmp.Options{
					cmpopts.IgnoreUnexported(ec2types.Subnet{}),
					cmpopts.IgnoreUnexported(ec2types.Tag{}),
				}
				assert.True(t, cmp.Equal(tt.want, got, opts), "diff", cmp.Diff(tt.want, got, opts))
			}
		})
	}
}

func Test_defaultSubnetsResolver_chooseSubnetsPerAZ(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		subnets []ec2types.Subnet
		want    []ec2types.Subnet
	}{
		{
			name: "sort by lexlexicographical order of subnet-id by default",
			subnets: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-3"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-4"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-3"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
			},
		},
		{
			name: "subnets with current cluster tag gets priority",
			subnets: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-1"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/cluster/cluster-dummy"),
							Value: awssdk.String("owned"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-3"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
				{
					SubnetId:                awssdk.String("subnet-4"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:                awssdk.String("subnet-2"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
					Tags: []ec2types.Tag{
						{
							Key:   awssdk.String("kubernetes.io/cluster/cluster-dummy"),
							Value: awssdk.String("owned"),
						},
					},
				},
				{
					SubnetId:                awssdk.String("subnet-3"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					AvailableIpAddressCount: awssdk.Int32(8),
					VpcId:                   awssdk.String("vpc-dummy"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewDefaultSubnetsResolver(nil, nil, "vpc-dummy", "cluster-dummy", true, false, true,
				logr.New(&log.NullLogSink{}))
			got := r.chooseSubnetsPerAZ(tt.subnets)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultSubnetsResolver_computeSubnetsMinimalCount(t *testing.T) {
	type fields struct {
		albSingleSubnetEnabled bool
	}
	type args struct {
		subnetLocale subnetLocaleType
		resolveOpts  SubnetsResolveOptions
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   int
	}{
		{
			name: "ALB needs 2 subnet by default",
			fields: fields{
				albSingleSubnetEnabled: false,
			},
			args: args{
				subnetLocale: subnetLocaleTypeAvailabilityZone,
				resolveOpts: SubnetsResolveOptions{
					LBType: elbv2model.LoadBalancerTypeApplication,
				},
			},
			want: 2,
		},
		{
			name: "ALB needs 1 subnet in localZone",
			fields: fields{
				albSingleSubnetEnabled: false,
			},
			args: args{
				subnetLocale: subnetLocaleTypeLocalZone,
				resolveOpts: SubnetsResolveOptions{
					LBType: elbv2model.LoadBalancerTypeApplication,
				},
			},
			want: 1,
		},
		{
			name: "ALB needs 1 subnet when albSingleSubnet enabled",
			fields: fields{
				albSingleSubnetEnabled: true,
			},
			args: args{
				subnetLocale: subnetLocaleTypeAvailabilityZone,
				resolveOpts: SubnetsResolveOptions{
					LBType: elbv2model.LoadBalancerTypeApplication,
				},
			},
			want: 1,
		},
		{
			name: "NLB needs 1 subnet by default",
			fields: fields{
				albSingleSubnetEnabled: false,
			},
			args: args{
				subnetLocale: subnetLocaleTypeAvailabilityZone,
				resolveOpts: SubnetsResolveOptions{
					LBType: elbv2model.LoadBalancerTypeNetwork,
				},
			},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewDefaultSubnetsResolver(nil, nil, "vpc-dummy", "cluster-dummy", true, tt.fields.albSingleSubnetEnabled, false,
				logr.New(&log.NullLogSink{}))
			got := r.computeSubnetsMinimalCount(tt.args.subnetLocale, tt.args.resolveOpts)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultSubnetsResolver_buildSDKSubnetLocaleType(t *testing.T) {
	type fetchAZInfosCall struct {
		availabilityZoneIDs []string
		azInfoByAZID        map[string]ec2types.AvailabilityZone
		err                 error
	}
	type fields struct {
		fetchAZInfosCalls []fetchAZInfosCall
	}
	type args struct {
		subnet ec2types.Subnet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    subnetLocaleType
		wantErr error
	}{
		{
			name: "availabilityZone subnet",
			fields: fields{
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnet: ec2types.Subnet{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
			want: subnetLocaleTypeAvailabilityZone,
		},
		{
			name: "localZone subnet",
			fields: fields{
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-lax1-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-lax1-az1": {
								ZoneId:   awssdk.String("usw2-lax1-az1"),
								ZoneType: awssdk.String("local-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnet: ec2types.Subnet{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2-lax-1a"),
					AvailabilityZoneId: awssdk.String("usw2-lax1-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
			want: subnetLocaleTypeLocalZone,
		},
		{
			name: "wavelengthZone subnet",
			fields: fields{
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-wl1-las-wlz1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-wl1-las-wlz1": {
								ZoneId:   awssdk.String("usw2-lax1-az1"),
								ZoneType: awssdk.String("wavelength-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnet: ec2types.Subnet{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2-wl1-las-wlz-1"),
					AvailabilityZoneId: awssdk.String("usw2-wl1-las-wlz1"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
			want: subnetLocaleTypeWavelengthZone,
		},
		{
			name: "outpost subnet",
			fields: fields{
				fetchAZInfosCalls: nil,
			},
			args: args{
				subnet: ec2types.Subnet{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
					OutpostArn:         awssdk.String("outpost-xxx"),
				},
			},
			want: subnetLocaleTypeOutpost,
		},
		{
			name: "fetchAZInfos fails",
			fields: fields{
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"invalid-zone-id"},
						err:                 errors.New("invalid availability zone-id: invalid-zone-id"),
					},
				},
			},
			args: args{
				subnet: ec2types.Subnet{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("invalid-zone"),
					AvailabilityZoneId: awssdk.String("invalid-zone-id"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
			wantErr: errors.New("invalid availability zone-id: invalid-zone-id"),
		},
		{
			name: "fetchAZInfos returns unknown zoneType",
			fields: fields{
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("unknown"),
							},
						},
					},
				},
			},
			args: args{
				subnet: ec2types.Subnet{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
			wantErr: errors.New("unknown zone type for subnet subnet-1: unknown"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			azInfoProvider := NewMockAZInfoProvider(ctrl)
			for _, call := range tt.fields.fetchAZInfosCalls {
				azInfoProvider.EXPECT().FetchAZInfos(gomock.Any(), call.availabilityZoneIDs).Return(call.azInfoByAZID, call.err)
			}

			r := defaultSubnetsResolver{
				azInfoProvider: azInfoProvider,
			}
			got, err := r.buildSDKSubnetLocaleType(context.Background(), tt.args.subnet)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_sortSubnetsByID(t *testing.T) {
	type args struct {
		subnets []ec2types.Subnet
	}
	tests := []struct {
		name        string
		args        args
		wantSubnets []ec2types.Subnet
	}{
		{
			name: "subnets already sorted",
			args: args{
				subnets: []ec2types.Subnet{
					{
						SubnetId: awssdk.String("subnet-a"),
					},
					{
						SubnetId: awssdk.String("subnet-b"),
					}, {
						SubnetId: awssdk.String("subnet-c"),
					},
				},
			},
			wantSubnets: []ec2types.Subnet{
				{
					SubnetId: awssdk.String("subnet-a"),
				},
				{
					SubnetId: awssdk.String("subnet-b"),
				}, {
					SubnetId: awssdk.String("subnet-c"),
				},
			},
		},
		{
			name: "subnets not sorted",
			args: args{
				subnets: []ec2types.Subnet{
					{
						SubnetId: awssdk.String("subnet-c"),
					},
					{
						SubnetId: awssdk.String("subnet-b"),
					},
					{
						SubnetId: awssdk.String("subnet-a"),
					},
				},
			},
			wantSubnets: []ec2types.Subnet{
				{
					SubnetId: awssdk.String("subnet-a"),
				},
				{
					SubnetId: awssdk.String("subnet-b"),
				},
				{
					SubnetId: awssdk.String("subnet-c"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subnetsClone := append(tt.args.subnets[:0:0], tt.args.subnets...)
			sortSubnetsByID(subnetsClone)
			assert.Equal(t, tt.wantSubnets, subnetsClone)
		})
	}
}

func Test_extractSubnetIDs(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		subnets []ec2types.Subnet
		want    []string
	}{
		{
			name:    "0 subnets",
			subnets: nil,
			want:    []string{},
		},
		{
			name: "multiple subnets",
			subnets: []ec2types.Subnet{
				{
					SubnetId: awssdk.String("subnet-1"),
				},
				{
					SubnetId: awssdk.String("subnet-2"),
				},
			},
			want: []string{"subnet-1", "subnet-2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSubnetIDs(tt.subnets)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_IsSubnetInLocalZoneOrOutpost(t *testing.T) {
	type describeSubnetsAsListCall struct {
		input  *ec2sdk.DescribeSubnetsInput
		output []ec2types.Subnet
		err    error
	}
	type describeRouteTablesAsListCall struct {
		input  *ec2sdk.DescribeRouteTablesInput
		output []ec2types.RouteTable
		err    error
	}
	type fetchAZInfosCall struct {
		availabilityZoneIDs []string
		azInfoByAZID        map[string]ec2types.AvailabilityZone
		err                 error
	}
	type fields struct {
		clusterTagCheckEnabled         bool
		albSingleSubnetEnabled         bool
		discoveryByReachabilityEnabled bool
		describeSubnetsAsListCalls     []describeSubnetsAsListCall
		describeRouteTablesAsListCalls []describeRouteTablesAsListCall
		fetchAZInfosCalls              []fetchAZInfosCall
	}
	tests := []struct {
		name     string
		fields   fields
		subnetID string
		want     bool
		wantErr  bool
	}{
		{
			name:     "Subnet in Local Zone",
			subnetID: "subnet-1234567890",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-1234567890"},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1234567890"),
								AvailabilityZone:        awssdk.String("us-west-2-lax-1a"),
								AvailabilityZoneId:      awssdk.String("usw2-lax1-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								OutpostArn:              nil,
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-lax1-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-lax1-az1": {
								ZoneId:   awssdk.String("usw2-lax1-az1"),
								ZoneType: awssdk.String("local-zone"),
							},
						},
					},
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name:     "Subnet in Outpost",
			subnetID: "subnet-outpost123",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-outpost123"},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-outpost123"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								OutpostArn:              awssdk.String("arn:aws:outposts:us-west-2:123456789012:outpost/op-1234567890abcdef0"),
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
						},
					},
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name:     "Regular AZ subnet",
			subnetID: "subnet-regular123",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-regular123"},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-regular123"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								OutpostArn:              nil,
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			want:    false,
			wantErr: false,
		},
		{
			name:     "Subnet in Wavelength Zone",
			subnetID: "subnet-wavelength123",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-wavelength123"},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-wavelength123"),
								AvailabilityZone:        awssdk.String("us-west-2-wl1-sfo-wlz-1"),
								AvailabilityZoneId:      awssdk.String("usw2-wl1-sfo-wlz1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								OutpostArn:              nil,
								Tags: []ec2types.Tag{
									{
										Key:   awssdk.String("kubernetes.io/role/elb"),
										Value: awssdk.String("1"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-wl1-sfo-wlz1"},
						azInfoByAZID: map[string]ec2types.AvailabilityZone{
							"usw2-wl1-sfo-wlz1": {
								ZoneId:   awssdk.String("usw2-wl1-sfo-wlz1"),
								ZoneType: awssdk.String("wavelength-zone"),
							},
						},
					},
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name:     "Subnet not found",
			subnetID: "subnet-notfound",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-notfound"},
						},
						err: errors.New("InvalidSubnetID.NotFound: Subnet ID 'subnet-notfound' does not exist"),
					},
				},
			},
			want:    false,
			wantErr: true,
		},
		{
			name:     "DescribeSubnets API error",
			subnetID: "subnet-error",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-error"},
						},
						err: errors.New("API error"),
					},
				},
			},
			want:    false,
			wantErr: true,
		},
		{
			name:     "FetchAZInfos error",
			subnetID: "subnet-azinfo-error",
			fields: fields{
				clusterTagCheckEnabled:         true,
				albSingleSubnetEnabled:         false,
				discoveryByReachabilityEnabled: true,
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: []string{"subnet-azinfo-error"},
						},
						output: []ec2types.Subnet{
							{
								SubnetId:                awssdk.String("subnet-azinfo-error"),
								AvailabilityZone:        awssdk.String("us-west-2-lax-1a"),
								AvailabilityZoneId:      awssdk.String("usw2-lax1-az1"),
								AvailableIpAddressCount: awssdk.Int32(8),
								VpcId:                   awssdk.String("vpc-dummy"),
								OutpostArn:              nil,
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-lax1-az1"},
						err:                 errors.New("failed to fetch AZ info"),
					},
				},
			},
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeSubnetsAsListCalls {
				ec2Client.EXPECT().DescribeSubnetsAsList(gomock.Any(), call.input).Return(call.output, call.err)
			}
			for _, call := range tt.fields.describeRouteTablesAsListCalls {
				ec2Client.EXPECT().DescribeRouteTablesAsList(gomock.Any(), call.input).Return(call.output, call.err)
			}
			azInfoProvider := NewMockAZInfoProvider(ctrl)
			for _, call := range tt.fields.fetchAZInfosCalls {
				azInfoProvider.EXPECT().FetchAZInfos(gomock.Any(), call.availabilityZoneIDs).Return(call.azInfoByAZID, call.err)
			}
			r := NewDefaultSubnetsResolver(azInfoProvider, ec2Client, "vpc-dummy", "cluster-dummy",
				tt.fields.clusterTagCheckEnabled, tt.fields.albSingleSubnetEnabled, tt.fields.discoveryByReachabilityEnabled,
				logr.New(&log.NullLogSink{}))
			got, err := r.IsSubnetInLocalZoneOrOutpost(ctx, tt.subnetID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
