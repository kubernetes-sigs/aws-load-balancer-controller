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
	mock_services "sigs.k8s.io/aws-load-balancer-controller/mocks/aws/services"
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
	type fields struct {
		vpcID                      string
		clusterName                string
		describeSubnetsAsListCalls []describeSubnetsAsListCall
	}
	type args struct {
		opts []SubnetsResolveOption
	}
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
									Name:   awssdk.String("tag:kubernetes.io/cluster/kube-cluster"),
									Values: awssdk.StringSlice([]string{"owned", "shared"}),
								},
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
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternetFacing),
				},
			},
			want: []*ec2sdk.Subnet{
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
									Name:   awssdk.String("tag:kubernetes.io/cluster/kube-cluster"),
									Values: awssdk.StringSlice([]string{"owned", "shared"}),
								},
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
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []*ec2sdk.Subnet{
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
									Name:   awssdk.String("tag:kubernetes.io/cluster/kube-cluster"),
									Values: awssdk.StringSlice([]string{"owned", "shared"}),
								},
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
									Name:   awssdk.String("tag:kubernetes.io/cluster/kube-cluster"),
									Values: awssdk.StringSlice([]string{"owned", "shared"}),
								},
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
								SubnetId:         awssdk.String("subnet-1"),
								AvailabilityZone: awssdk.String("us-west-2a"),
								VpcId:            awssdk.String("vpc-1"),
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
					SubnetId:         awssdk.String("subnet-1"),
					AvailabilityZone: awssdk.String("us-west-2a"),
					VpcId:            awssdk.String("vpc-1"),
				},
			},
		},
		{
			name: "ALB with one matching subnet",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/cluster/kube-cluster"),
									Values: awssdk.StringSlice([]string{"owned", "shared"}),
								},
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
								SubnetId:         awssdk.String("subnet-1"),
								AvailabilityZone: awssdk.String("us-west-2a"),
								VpcId:            awssdk.String("vpc-1"),
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
			name: "multiple subnets per az",
			fields: fields{
				vpcID:       "vpc-1",
				clusterName: "kube-cluster",
				describeSubnetsAsListCalls: []describeSubnetsAsListCall{
					{
						input: &ec2sdk.DescribeSubnetsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("tag:kubernetes.io/cluster/kube-cluster"),
									Values: awssdk.StringSlice([]string{"owned", "shared"}),
								},
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
								SubnetId:         awssdk.String("subnet-3"),
								AvailabilityZone: awssdk.String("us-west-2a"),
								VpcId:            awssdk.String("vpc-1"),
							},
							{
								SubnetId:         awssdk.String("subnet-4"),
								AvailabilityZone: awssdk.String("us-west-2b"),
								VpcId:            awssdk.String("vpc-1"),
							},
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
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []*ec2sdk.Subnet{
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
									Name:   awssdk.String("tag:kubernetes.io/cluster/kube-cluster"),
									Values: awssdk.StringSlice([]string{"owned", "shared"}),
								},
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
								SubnetId:         awssdk.String("subnet-1"),
								AvailabilityZone: awssdk.String("us-west-2a"),
								VpcId:            awssdk.String("vpc-1"),
							},
							{
								SubnetId:         awssdk.String("subnet-2"),
								AvailabilityZone: awssdk.String("us-west-2b"),
								VpcId:            awssdk.String("vpc-1"),
								OutpostArn:       awssdk.String("outpost-xxx"),
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
			wantErr: errors.New("subnets in multiple locales: [availabilityZone outpost]"),
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
									Name:   awssdk.String("tag:kubernetes.io/cluster/kube-cluster"),
									Values: awssdk.StringSlice([]string{"owned", "shared"}),
								},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := mock_services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeSubnetsAsListCalls {
				ec2Client.EXPECT().DescribeSubnetsAsList(gomock.Any(), call.input).Return(call.output, call.err)
			}

			r := &defaultSubnetsResolver{
				ec2Client:   ec2Client,
				vpcID:       tt.fields.vpcID,
				clusterName: tt.fields.clusterName,
				logger:      &log.NullLogger{},
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
	type fields struct {
		vpcID                      string
		clusterName                string
		describeSubnetsAsListCalls []describeSubnetsAsListCall
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
				subnetNameOrIDs: []string{"subnet-1", "subnet-2"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []*ec2sdk.Subnet{
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
				subnetNameOrIDs: []string{"my-name-1", "my-name-2"},
				opts: []SubnetsResolveOption{
					WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
					WithSubnetsResolveLBScheme(elbv2model.LoadBalancerSchemeInternal),
				},
			},
			want: []*ec2sdk.Subnet{
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
								SubnetId:         awssdk.String("subnet-1"),
								AvailabilityZone: awssdk.String("us-west-2a"),
								VpcId:            awssdk.String("vpc-1"),
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
								SubnetId:         awssdk.String("subnet-2"),
								AvailabilityZone: awssdk.String("us-west-2b"),
								VpcId:            awssdk.String("vpc-1"),
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
								SubnetId:         awssdk.String("subnet-1"),
								AvailabilityZone: awssdk.String("us-west-2a"),
								VpcId:            awssdk.String("vpc-1"),
							},
							{
								SubnetId:         awssdk.String("subnet-2"),
								AvailabilityZone: awssdk.String("us-west-2b"),
								VpcId:            awssdk.String("vpc-1"),
								OutpostArn:       awssdk.String("outpost-xxx"),
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
			wantErr: errors.New("subnets in multiple locales: [availabilityZone outpost]"),
		},
		{
			name: "ALB with one availabilityZone subnet",
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
								SubnetId:         awssdk.String("subnet-1"),
								AvailabilityZone: awssdk.String("us-west-2a"),
								VpcId:            awssdk.String("vpc-1"),
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
			name: "ALB with one Outpost subnet",
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
								SubnetId:         awssdk.String("subnet-1"),
								AvailabilityZone: awssdk.String("us-west-2a"),
								VpcId:            awssdk.String("vpc-1"),
								OutpostArn:       awssdk.String("outpost-xxx"),
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
					SubnetId:         awssdk.String("subnet-1"),
					AvailabilityZone: awssdk.String("us-west-2a"),
					VpcId:            awssdk.String("vpc-1"),
					OutpostArn:       awssdk.String("outpost-xxx"),
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
								SubnetId:         awssdk.String("subnet-1"),
								AvailabilityZone: awssdk.String("us-west-2a"),
								VpcId:            awssdk.String("vpc-1"),
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
					SubnetId:         awssdk.String("subnet-1"),
					AvailabilityZone: awssdk.String("us-west-2a"),
					VpcId:            awssdk.String("vpc-1"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := mock_services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeSubnetsAsListCalls {
				ec2Client.EXPECT().DescribeSubnetsAsList(gomock.Any(), call.input).Return(call.output, call.err)
			}

			r := &defaultSubnetsResolver{
				ec2Client:   ec2Client,
				vpcID:       tt.fields.vpcID,
				clusterName: tt.fields.clusterName,
				logger:      &log.NullLogger{},
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

func Test_buildSDKSubnetLocaleType(t *testing.T) {
	type args struct {
		subnet *ec2sdk.Subnet
	}
	tests := []struct {
		name string
		args args
		want subnetLocaleType
	}{
		{
			name: "availbilityZone subnet",
			args: args{
				subnet: &ec2sdk.Subnet{
					SubnetId:         awssdk.String("subnet-1"),
					AvailabilityZone: awssdk.String("us-west-2a"),
					VpcId:            awssdk.String("vpc-1"),
				},
			},
			want: subnetLocaleTypeAvailabilityZone,
		},
		{
			name: "outpost subnet",
			args: args{
				subnet: &ec2sdk.Subnet{
					SubnetId:         awssdk.String("subnet-1"),
					AvailabilityZone: awssdk.String("us-west-2a"),
					VpcId:            awssdk.String("vpc-1"),
					OutpostArn:       awssdk.String("outpost-xxx"),
				},
			},
			want: subnetLocaleTypeOutpost,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildSDKSubnetLocaleType(tt.args.subnet); got != tt.want {
				t.Errorf("buildSDKSubnetLocaleType() = %v, want %v", got, tt.want)
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
