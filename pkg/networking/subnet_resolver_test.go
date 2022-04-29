package networking

import (
	"context"
	"errors"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultSubnetsResolver_ResolveViaDiscovery(t *testing.T) {
	type describeSubnetsAsListCall struct {
		input  *ec2sdk.DescribeSubnetsInput
		output []*ec2sdk.Subnet
		err    error
	}
	type fetchAZInfosCall struct {
		availabilityZoneIDs []string
		azInfoByAZID        map[string]ec2sdk.AvailabilityZone
		err                 error
	}
	type fields struct {
		vpcID                      string
		clusterName                string
		describeSubnetsAsListCalls []describeSubnetsAsListCall
		fetchAZInfosCalls          []fetchAZInfosCall
	}
	type args struct {
		opts []SubnetsResolveOption
	}
	const (
		minimalAvailableIPAddressCount = int64(8)
		defaultSubnetsClusterTagCheck  = true
	)
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []*ec2sdk.Subnet
		wantErr error
	}{
		{
			name: "ALB internet facing",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
							{
								SubnetId:           awssdk.String("subnet-2"),
								AvailabilityZone:   awssdk.String("us-west-2b"),
								AvailabilityZoneId: awssdk.String("usw2-az2"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
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
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
				{
					SubnetId:           awssdk.String("subnet-2"),
					AvailabilityZone:   awssdk.String("us-west-2b"),
					AvailabilityZoneId: awssdk.String("usw2-az2"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
		},
		{
			name: "ALB internal",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/internal-elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
							{
								SubnetId:           awssdk.String("subnet-2"),
								AvailabilityZone:   awssdk.String("us-west-2b"),
								AvailabilityZoneId: awssdk.String("usw2-az2"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
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
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
				{
					SubnetId:           awssdk.String("subnet-2"),
					AvailabilityZone:   awssdk.String("us-west-2b"),
					AvailabilityZoneId: awssdk.String("usw2-az2"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
		},
		{
			name: "ALB with no matching subnets",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/internal-elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
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
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			wantErr: errors.New("unable to discover at least one subnet"),
		},
		{
			name: "NLB with one matching subnet",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/internal-elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
		},
		{
			name: "ALB with one matching availability-zone subnet",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/internal-elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
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
			wantErr: errors.New("subnets count less than minimal required count: 1 < 2"),
		},
		{
			name: "ALB with one matching local-zone subnet",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/internal-elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2-lax-1a"),
								AvailabilityZoneId: awssdk.String("usw2-lax1-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-lax1-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-lax1-az1": {
								ZoneId:   awssdk.String("usw2-lax1-az1"),
								ZoneType: awssdk.String("local-zone"),
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
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2-lax-1a"),
					AvailabilityZoneId: awssdk.String("usw2-lax1-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
		},
		{
			name: "ALB with one matching outpost subnet",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/internal-elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								OutpostArn:         awssdk.String("outpost-xxx"),
								VpcId:              awssdk.String("vpc-1"),
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
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					OutpostArn:         awssdk.String("outpost-xxx"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
		},
		{
			name: "multiple subnets per az",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/internal-elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-3"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
							{
								SubnetId:           awssdk.String("subnet-4"),
								AvailabilityZone:   awssdk.String("us-west-2b"),
								AvailabilityZoneId: awssdk.String("usw2-az2"),
								VpcId:              awssdk.String("vpc-1"),
							},
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
							{
								SubnetId:           awssdk.String("subnet-2"),
								AvailabilityZone:   awssdk.String("us-west-2b"),
								AvailabilityZoneId: awssdk.String("usw2-az2"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
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
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
				{
					SubnetId:           awssdk.String("subnet-2"),
					AvailabilityZone:   awssdk.String("us-west-2b"),
					AvailabilityZoneId: awssdk.String("usw2-az2"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
		},
		{
			name: "multiple subnet locales",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/internal-elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
							{
								SubnetId:           awssdk.String("subnet-2"),
								AvailabilityZone:   awssdk.String("us-west-2b"),
								AvailabilityZoneId: awssdk.String("usw2-az2"),
								VpcId:              awssdk.String("vpc-1"),
								OutpostArn:         awssdk.String("outpost-xxx"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
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
			wantErr: errors.New("subnets in multiple locales: [availability-zone outpost]"),
		},
		{
			name: "describeSubnetsAsList returns error",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/internal-elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
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
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			wantErr: errors.New("some error"),
		},
		{
			name: "subnet with cluster tag gets precedence",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2b"),
								AvailabilityZoneId: awssdk.String("usw2-az2"),
								VpcId:              awssdk.String("vpc-1"),
							},
							{
								SubnetId:           awssdk.String("subnet-2"),
								AvailabilityZone:   awssdk.String("us-west-2b"),
								AvailabilityZoneId: awssdk.String("usw2-az2"),
								VpcId:              awssdk.String("vpc-1"),
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("kubernetes.io/cluster/kube-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
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
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-2"),
					AvailabilityZone:   awssdk.String("us-west-2b"),
					AvailabilityZoneId: awssdk.String("usw2-az2"),
					VpcId:              awssdk.String("vpc-1"),
					Tags: []*ec2sdk.Tag{
						{
							Key:   awssdk.String("kubernetes.io/cluster/kube-cluster"),
							Value: awssdk.String("owned"),
						},
					},
				},
			},
		},
		{
			name: "subnets tagged for some other clusters get ignored, with SubnetsClusterTagCheck enabled",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("kubernetes.io/cluster/some-other-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:           awssdk.String("subnet-3"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("kubernetes.io/cluster/kube-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:           awssdk.String("subnet-4"),
								AvailabilityZone:   awssdk.String("us-west-2b"),
								AvailabilityZoneId: awssdk.String("usw2-az2"),
								VpcId:              awssdk.String("vpc-1"),
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("kubernetes.io/cluster/no-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:           awssdk.String("subnet-2"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("kubernetes.io/cluster/kube-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:           awssdk.String("subnet-5"),
								AvailabilityZone:   awssdk.String("us-west-2c"),
								AvailabilityZoneId: awssdk.String("usw2-az3"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az3"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az3": {
								ZoneId:   awssdk.String("usw2-az3"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
					WithSubnetsClusterTagCheck(defaultSubnetsClusterTagCheck),
				},
			},
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-2"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
					Tags: []*ec2sdk.Tag{
						{
							Key:   awssdk.String("kubernetes.io/cluster/kube-cluster"),
							Value: awssdk.String("owned"),
						},
					},
				},
				{
					SubnetId:           awssdk.String("subnet-5"),
					AvailabilityZone:   awssdk.String("us-west-2c"),
					AvailabilityZoneId: awssdk.String("usw2-az3"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
		},
		{
			name: "subnets tagged for some other clusters doesn't get ignored, with SubnetsClusterTagCheck disabled",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-3"),
								AvailabilityZone:   awssdk.String("us-west-2c"),
								AvailabilityZoneId: awssdk.String("usw2-az3"),
								VpcId:              awssdk.String("vpc-1"),
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("kubernetes.io/cluster/some-other-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("kubernetes.io/cluster/some-other-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
							{
								SubnetId:           awssdk.String("subnet-2"),
								AvailabilityZone:   awssdk.String("us-west-2c"),
								AvailabilityZoneId: awssdk.String("usw2-az3"),
								VpcId:              awssdk.String("vpc-1"),
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("kubernetes.io/cluster/kube-cluster"),
										Value: awssdk.String("owned"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az3"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az3": {
								ZoneId:   awssdk.String("usw2-az3"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
					WithSubnetsClusterTagCheck(false),
				},
			},
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
					Tags: []*ec2sdk.Tag{
						{
							Key:   awssdk.String("kubernetes.io/cluster/some-other-cluster"),
							Value: awssdk.String("owned"),
						},
					},
				},
				{
					SubnetId:           awssdk.String("subnet-2"),
					AvailabilityZone:   awssdk.String("us-west-2c"),
					AvailabilityZoneId: awssdk.String("usw2-az3"),
					VpcId:              awssdk.String("vpc-1"),
					Tags: []*ec2sdk.Tag{
						{
							Key:   awssdk.String("kubernetes.io/cluster/kube-cluster"),
							Value: awssdk.String("owned"),
						},
					},
				},
			},
		},
		{
			name: "subnets with multiple cluster tags",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("kubernetes.io/cluster/some-other-cluster"),
										Value: awssdk.String("owned"),
									},
									{
										Key:   awssdk.String("kubernetes.io/cluster/kube-cluster"),
										Value: awssdk.String("shared"),
									},
								},
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
					Tags: []*ec2sdk.Tag{
						{
							Key:   awssdk.String("kubernetes.io/cluster/some-other-cluster"),
							Value: awssdk.String("owned"),
						},
						{
							Key:   awssdk.String("kubernetes.io/cluster/kube-cluster"),
							Value: awssdk.String("shared"),
						},
					},
				},
			},
		},
		{
			name: "subnets with insufficient available ip addresses get ignored",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/role/elb"),
									Values: awssdk.StringSlice([]string{"", "1"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:                awssdk.String("subnet-1"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								VpcId:                   awssdk.String("vpc-1"),
								AvailableIpAddressCount: awssdk.Int64(0),
							},
							{
								SubnetId:                awssdk.String("subnet-3"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								VpcId:                   awssdk.String("vpc-1"),
								AvailableIpAddressCount: awssdk.Int64(8),
							},
							{
								SubnetId:                awssdk.String("subnet-4"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								VpcId:                   awssdk.String("vpc-1"),
								AvailableIpAddressCount: awssdk.Int64(25),
							},
							{
								SubnetId:                awssdk.String("subnet-2"),
								AvailabilityZone:        awssdk.String("us-west-2a"),
								AvailabilityZoneId:      awssdk.String("usw2-az1"),
								VpcId:                   awssdk.String("vpc-1"),
								AvailableIpAddressCount: awssdk.Int64(2),
							},
							{
								SubnetId:                awssdk.String("subnet-5"),
								AvailabilityZone:        awssdk.String("us-west-2b"),
								AvailabilityZoneId:      awssdk.String("usw2-az2"),
								VpcId:                   awssdk.String("vpc-1"),
								AvailableIpAddressCount: awssdk.Int64(10),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
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
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
					WithSubnetsResolveAvailableIPAddressCount(minimalAvailableIPAddressCount),
				},
			},
			want: []*ec2sdk.Subnet{
				{
					SubnetId:                awssdk.String("subnet-3"),
					AvailabilityZone:        awssdk.String("us-west-2a"),
					AvailabilityZoneId:      awssdk.String("usw2-az1"),
					VpcId:                   awssdk.String("vpc-1"),
					AvailableIpAddressCount: awssdk.Int64(8),
				},
				{
					SubnetId:                awssdk.String("subnet-4"),
					AvailabilityZone:        awssdk.String("us-west-2b"),
					AvailabilityZoneId:      awssdk.String("usw2-az2"),
					VpcId:                   awssdk.String("vpc-1"),
					AvailableIpAddressCount: awssdk.Int64(25),
				},
			},
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

			r := &defaultSubnetsResolver{
				azInfoProvider: azInfoProvider,
				ec2Client:      ec2Client,
				vpcID:          tt.fields.vpcID,
				clusterName:    tt.fields.clusterName,
				logger:         &log.NullLogger{},
			}

			got, err := r.ResolveViaDiscovery(context.Background(), tt.args.opts...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opts := cmpopts.SortSlices(func(lhs *ec2sdk.Subnet, rhs *ec2sdk.Subnet) bool {
					return awssdk.StringValue(lhs.SubnetId) < awssdk.StringValue(rhs.SubnetId)
				})
				assert.True(t, cmp.Equal(tt.want, got, opts), "diff", cmp.Diff(tt.want, got, opts))
			}
		})
	}
}

func Test_defaultSubnetsResolver_ResolveViaNameOrIDSlice(t *testing.T) {
	type describeSubnetsAsListCall struct {
		input  *ec2sdk.DescribeSubnetsInput
		output []*ec2sdk.Subnet
		err    error
	}
	type fetchAZInfosCall struct {
		availabilityZoneIDs []string
		azInfoByAZID        map[string]ec2sdk.AvailabilityZone
		err                 error
	}
	type fields struct {
		vpcID                      string
		clusterName                string
		describeSubnetsAsListCalls []describeSubnetsAsListCall
		fetchAZInfosCalls          []fetchAZInfosCall
	}
	type args struct {
		subnetNameOrIDs []string
		opts            []SubnetsResolveOption
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []*ec2sdk.Subnet
		wantErr error
	}{
		{
			name: "ALB with subnetID only",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: awssdk.StringSlice([]string{"subnet-1", "subnet-2"}),
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
							{
								SubnetId:           awssdk.String("subnet-2"),
								AvailabilityZone:   awssdk.String("us-west-2b"),
								AvailabilityZoneId: awssdk.String("usw2-az2"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnetNameOrIDs: []string{"subnet-1", "subnet-2"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
				{
					SubnetId:           awssdk.String("subnet-2"),
					AvailabilityZone:   awssdk.String("us-west-2b"),
					AvailabilityZoneId: awssdk.String("usw2-az2"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
		},
		{
			name: "ALB with subnet Name only",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:Name"),
									Values: awssdk.StringSlice([]string{"my-name-1", "my-name-2"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
							{
								SubnetId:           awssdk.String("subnet-2"),
								AvailabilityZone:   awssdk.String("us-west-2b"),
								AvailabilityZoneId: awssdk.String("usw2-az2"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnetNameOrIDs: []string{"my-name-1", "my-name-2"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
				{
					SubnetId:           awssdk.String("subnet-2"),
					AvailabilityZone:   awssdk.String("us-west-2b"),
					AvailabilityZoneId: awssdk.String("usw2-az2"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
		},
		{
			name: "ALB with subnetID and subnet Name",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: awssdk.StringSlice([]string{"subnet-1"}),
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:Name"),
									Values: awssdk.StringSlice([]string{"my-name-2"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-2"),
								AvailabilityZone:   awssdk.String("us-west-2b"),
								AvailabilityZoneId: awssdk.String("usw2-az2"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
					{
						availabilityZoneIDs: []string{"usw2-az2"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az2": {
								ZoneId:   awssdk.String("usw2-az2"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnetNameOrIDs: []string{"subnet-1", "my-name-2"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
				{
					SubnetId:           awssdk.String("subnet-2"),
					AvailabilityZone:   awssdk.String("us-west-2b"),
					AvailabilityZoneId: awssdk.String("usw2-az2"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
		},
		{
			name: "cannot resolve all subnet names",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:Name"),
									Values: awssdk.StringSlice([]string{"my-name-1", "my-name-2", "my-name-3"}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-1"}),
								},
							},
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:         awssdk.String("subnet-1"),
								AvailabilityZone: awssdk.String("us-west-2a"),
								VpcId:            awssdk.String("vpc-1"),
							},
							{
								SubnetId:         awssdk.String("subnet-2"),
								AvailabilityZone: awssdk.String("us-west-2b"),
								VpcId:            awssdk.String("vpc-1"),
							},
						},
					},
				},
			},
			args: args{
				subnetNameOrIDs: []string{"my-name-1", "my-name-2", "my-name-3"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			wantErr: errors.New("couldn't find all subnets, nameOrIDs: [my-name-1 my-name-2 my-name-3], found: 2"),
		},
		{
			name: "empty subnet name or IDs",
			fields: fields{
				vpcID:                      "vpc-1",
				clusterName:                "kube-cluster",
				describeSubnetsAsListCalls: nil,
			},
			args: args{
				subnetNameOrIDs: nil,
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			wantErr: errors.New("unable to resolve at least one subnet"),
		},
		{
			name: "multiple subnet in same AZ",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: awssdk.StringSlice([]string{"subnet-1", "subnet-2", "subnet-3"}),
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:         awssdk.String("subnet-1"),
								AvailabilityZone: awssdk.String("us-west-2a"),
								VpcId:            awssdk.String("vpc-1"),
							},
							{
								SubnetId:         awssdk.String("subnet-2"),
								AvailabilityZone: awssdk.String("us-west-2b"),
								VpcId:            awssdk.String("vpc-1"),
							},
							{
								SubnetId:         awssdk.String("subnet-3"),
								AvailabilityZone: awssdk.String("us-west-2a"),
								VpcId:            awssdk.String("vpc-1"),
							},
						},
					},
				},
			},
			args: args{
				subnetNameOrIDs: []string{"subnet-1", "subnet-2", "subnet-3"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			wantErr: errors.New("multiple subnets in same Availability Zone us-west-2a: [subnet-1 subnet-3]"),
		},
		{
			name: "multiple subnet locales",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: awssdk.StringSlice([]string{"subnet-1", "subnet-2"}),
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
							{
								SubnetId:           awssdk.String("subnet-2"),
								AvailabilityZone:   awssdk.String("us-west-2b"),
								AvailabilityZoneId: awssdk.String("usw2-az2"),
								VpcId:              awssdk.String("vpc-1"),
								OutpostArn:         awssdk.String("outpost-xxx"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnetNameOrIDs: []string{"subnet-1", "subnet-2"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			wantErr: errors.New("subnets in multiple locales: [availability-zone outpost]"),
		},
		{
			name: "ALB with one availability-zone subnet",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: awssdk.StringSlice([]string{"subnet-1"}),
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnetNameOrIDs: []string{"subnet-1"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			wantErr: errors.New("subnets count less than minimal required count: 1 < 2"),
		},
		{
			name: "ALB with one local-zone subnet",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: awssdk.StringSlice([]string{"subnet-1"}),
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2-lax-1a"),
								AvailabilityZoneId: awssdk.String("usw2-lax1-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-lax1-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-lax1-az1": {
								ZoneId:   awssdk.String("usw2-lax1-az1"),
								ZoneType: awssdk.String("local-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnetNameOrIDs: []string{"subnet-1"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2-lax-1a"),
					AvailabilityZoneId: awssdk.String("usw2-lax1-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
		},
		{
			name: "ALB with one outpost subnet",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: awssdk.StringSlice([]string{"subnet-1"}),
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
								OutpostArn:         awssdk.String("outpost-xxx"),
							},
						},
					},
				},
			},
			args: args{
				subnetNameOrIDs: []string{"subnet-1"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
					OutpostArn:         awssdk.String("outpost-xxx"),
				},
			},
		},
		{
			name: "NLB with one availabilityZone subnet",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: awssdk.StringSlice([]string{"subnet-1"}),
						},
						output: []*ec2sdk.Subnet{
							{
								SubnetId:           awssdk.String("subnet-1"),
								AvailabilityZone:   awssdk.String("us-west-2a"),
								AvailabilityZoneId: awssdk.String("usw2-az1"),
								VpcId:              awssdk.String("vpc-1"),
							},
						},
					},
				},
				fetchAZInfosCalls: []fetchAZInfosCall{
					{
						availabilityZoneIDs: []string{"usw2-az1"},
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnetNameOrIDs: []string{"subnet-1"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []*ec2sdk.Subnet{
				{
					SubnetId:           awssdk.String("subnet-1"),
					AvailabilityZone:   awssdk.String("us-west-2a"),
					AvailabilityZoneId: awssdk.String("usw2-az1"),
					VpcId:              awssdk.String("vpc-1"),
				},
			},
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

			r := &defaultSubnetsResolver{
				azInfoProvider: azInfoProvider,
				ec2Client:      ec2Client,
				vpcID:          tt.fields.vpcID,
				clusterName:    tt.fields.clusterName,
				logger:         &log.NullLogger{},
			}
			got, err := r.ResolveViaNameOrIDSlice(context.Background(), tt.args.subnetNameOrIDs, tt.args.opts...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opts := cmpopts.SortSlices(func(lhs *ec2sdk.Subnet, rhs *ec2sdk.Subnet) bool {
					return awssdk.StringValue(lhs.SubnetId) < awssdk.StringValue(rhs.SubnetId)
				})
				assert.True(t, cmp.Equal(tt.want, got, opts), "diff", cmp.Diff(tt.want, got, opts))
			}
		})
	}
}

func Test_defaultSubnetsResolver_buildSDKSubnetLocaleType(t *testing.T) {
	type fetchAZInfosCall struct {
		availabilityZoneIDs []string
		azInfoByAZID        map[string]ec2sdk.AvailabilityZone
		err                 error
	}
	type fields struct {
		fetchAZInfosCalls []fetchAZInfosCall
	}
	type args struct {
		subnet *ec2sdk.Subnet
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
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("availability-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnet: &ec2sdk.Subnet{
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
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-lax1-az1": {
								ZoneId:   awssdk.String("usw2-lax1-az1"),
								ZoneType: awssdk.String("local-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnet: &ec2sdk.Subnet{
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
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-wl1-las-wlz1": {
								ZoneId:   awssdk.String("usw2-lax1-az1"),
								ZoneType: awssdk.String("wavelength-zone"),
							},
						},
					},
				},
			},
			args: args{
				subnet: &ec2sdk.Subnet{
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
				subnet: &ec2sdk.Subnet{
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
				subnet: &ec2sdk.Subnet{
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
						azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
							"usw2-az1": {
								ZoneId:   awssdk.String("usw2-az1"),
								ZoneType: awssdk.String("unknown"),
							},
						},
					},
				},
			},
			args: args{
				subnet: &ec2sdk.Subnet{
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
		subnets []*ec2sdk.Subnet
	}
	tests := []struct {
		name        string
		args        args
		wantSubnets []*ec2sdk.Subnet
	}{
		{
			name: "subnets already sorted",
			args: args{
				subnets: []*ec2sdk.Subnet{
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
			wantSubnets: []*ec2sdk.Subnet{
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
				subnets: []*ec2sdk.Subnet{
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
			wantSubnets: []*ec2sdk.Subnet{
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
