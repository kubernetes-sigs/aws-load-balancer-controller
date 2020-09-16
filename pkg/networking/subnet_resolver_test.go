package networking

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	mock_services "sigs.k8s.io/aws-alb-ingress-controller/mocks/aws/services"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sort"
	"testing"
)

func Test_subnetResolver_DiscoverSubnets(t *testing.T) {
	type fields struct {
		input  *ec2.DescribeSubnetsInput
		output []*ec2.Subnet
		err    error
	}
	tests := []struct {
		name        string
		scheme      elbv2.LoadBalancerScheme
		apiParams   fields
		want        []*ec2.Subnet
		wantErr     error
		vpcID       string
		clusterName string
	}{
		{
			name:   "internet facing",
			scheme: elbv2.LoadBalancerSchemeInternetFacing,
			apiParams: fields{
				input: &ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{
					{
						Name:   aws.String("tag:kubernetes.io/cluster/kube-kluster"),
						Values: aws.StringSlice([]string{"owned", "shared"}),
					},
					{
						Name:   aws.String("tag:kubernetes.io/role/elb"),
						Values: aws.StringSlice([]string{"", "1"}),
					},
					{
						Name:   aws.String("vpc-id"),
						Values: aws.StringSlice([]string{"vpc-1"}),
					},
				}},
				output: []*ec2.Subnet{
					{
						SubnetId:         aws.String("subnet-1"),
						AvailabilityZone: aws.String("az-1"),
						VpcId:            aws.String("vpc-1"),
					},
					{
						SubnetId:         aws.String("subnet-2"),
						AvailabilityZone: aws.String("az-2"),
						VpcId:            aws.String("vpc-1"),
					},
				},
			},
			vpcID:       "vpc-1",
			clusterName: "kube-kluster",
			want: []*ec2.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("az-1"),
					VpcId:            aws.String("vpc-1"),
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("az-2"),
					VpcId:            aws.String("vpc-1"),
				},
			},
		},
		{
			name:   "internal",
			scheme: elbv2.LoadBalancerSchemeInternal,
			apiParams: fields{
				input: &ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{
					{
						Name:   aws.String("tag:kubernetes.io/cluster/kube-cl"),
						Values: aws.StringSlice([]string{"owned", "shared"}),
					},
					{
						Name:   aws.String("tag:kubernetes.io/role/internal-elb"),
						Values: aws.StringSlice([]string{"", "1"}),
					},
					{
						Name:   aws.String("vpc-id"),
						Values: aws.StringSlice([]string{"vpc-xx"}),
					},
				}},
				output: []*ec2.Subnet{
					{
						SubnetId:         aws.String("subnet-ab1"),
						AvailabilityZone: aws.String("az-133"),
						VpcId:            aws.String("vpc-xx"),
					},
					{
						SubnetId:         aws.String("subnet-bc1"),
						AvailabilityZone: aws.String("az-22"),
						VpcId:            aws.String("vpc-xx"),
					},
				},
			},
			vpcID:       "vpc-xx",
			clusterName: "kube-cl",
			want: []*ec2.Subnet{
				{
					SubnetId:         aws.String("subnet-ab1"),
					AvailabilityZone: aws.String("az-133"),
					VpcId:            aws.String("vpc-xx"),
				},
				{
					SubnetId:         aws.String("subnet-bc1"),
					AvailabilityZone: aws.String("az-22"),
					VpcId:            aws.String("vpc-xx"),
				},
			},
		},
		{
			name:   "no matching subnets",
			scheme: elbv2.LoadBalancerSchemeInternal,
			apiParams: fields{
				input: &ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{
					{
						Name:   aws.String("tag:kubernetes.io/cluster/kube-cl"),
						Values: aws.StringSlice([]string{"owned", "shared"}),
					},
					{
						Name:   aws.String("tag:kubernetes.io/role/internal-elb"),
						Values: aws.StringSlice([]string{"", "1"}),
					},
					{
						Name:   aws.String("vpc-id"),
						Values: aws.StringSlice([]string{"vpc-xx"}),
					},
				}},
				output: nil,
			},
			vpcID:       "vpc-xx",
			clusterName: "kube-cl",
			want:        []*ec2.Subnet{},
		},
		{
			name:   "describe returns error",
			scheme: elbv2.LoadBalancerSchemeInternal,
			apiParams: fields{
				input: &ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{
					{
						Name:   aws.String("tag:kubernetes.io/cluster/kube-cl"),
						Values: aws.StringSlice([]string{"owned", "shared"}),
					},
					{
						Name:   aws.String("tag:kubernetes.io/role/internal-elb"),
						Values: aws.StringSlice([]string{"", "1"}),
					},
					{
						Name:   aws.String("vpc-id"),
						Values: aws.StringSlice([]string{"vpc-xx"}),
					},
				}},
				err: errors.New("some error"),
			},
			vpcID:       "vpc-xx",
			clusterName: "kube-cl",
			wantErr:     errors.New("some error"),
		},
		{
			name:   "multiple subnets per az",
			scheme: elbv2.LoadBalancerSchemeInternetFacing,
			apiParams: fields{
				input: &ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{
					{
						Name:   aws.String("tag:kubernetes.io/cluster/kube-kluster"),
						Values: aws.StringSlice([]string{"owned", "shared"}),
					},
					{
						Name:   aws.String("tag:kubernetes.io/role/elb"),
						Values: aws.StringSlice([]string{"", "1"}),
					},
					{
						Name:   aws.String("vpc-id"),
						Values: aws.StringSlice([]string{"vpc-1"}),
					},
				}},
				output: []*ec2.Subnet{
					{
						SubnetId:         aws.String("fab"),
						AvailabilityZone: aws.String("az-1"),
						VpcId:            aws.String("vpc-1"),
					},
					{
						SubnetId:         aws.String("cd"),
						AvailabilityZone: aws.String("az-2"),
						VpcId:            aws.String("vpc-1"),
					},
					{
						SubnetId:         aws.String("ef"),
						AvailabilityZone: aws.String("az-1"),
						VpcId:            aws.String("vpc-1"),
					},
					{
						SubnetId:         aws.String("gh"),
						AvailabilityZone: aws.String("az-2"),
						VpcId:            aws.String("vpc-1"),
					},
				},
			},
			vpcID:       "vpc-1",
			clusterName: "kube-kluster",
			want: []*ec2.Subnet{
				{
					SubnetId:         aws.String("ef"),
					AvailabilityZone: aws.String("az-1"),
					VpcId:            aws.String("vpc-1"),
				},
				{
					SubnetId:         aws.String("cd"),
					AvailabilityZone: aws.String("az-2"),
					VpcId:            aws.String("vpc-1"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			ec2Client := mock_services.NewMockEC2(ctrl)
			ec2Client.EXPECT().DescribeSubnetsAsList(gomock.Any(), tt.apiParams.input).Return(tt.apiParams.output, tt.apiParams.err)
			resolver := NewSubnetsResolver(ec2Client, tt.vpcID, tt.clusterName, &log.NullLogger{})

			got, err := resolver.DiscoverSubnets(context.Background(), tt.scheme)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				sort.Slice(tt.want, func(i, j int) bool {
					return aws.StringValue(tt.want[i].SubnetId) < aws.StringValue(tt.want[j].SubnetId)
				})
				sort.Slice(got, func(i, j int) bool {
					return aws.StringValue(got[i].SubnetId) < aws.StringValue(got[j].SubnetId)
				})
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
