package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
)

func TestCloud_StatusEC2(t *testing.T) {
	for _, tc := range []struct {
		Name          string
		Error         error
		ExpectedError error
	}{
		{
			Name:          "No error from API call",
			Error:         nil,
			ExpectedError: nil,
		},
		{
			Name:          "Error from API call",
			Error:         errors.New("Some API error"),
			ExpectedError: errors.New("[ec2.DescribeTagsWithContext]: Some API error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ec2svc := &mocks.EC2API{}
			ec2svc.On("DescribeTagsWithContext", context.TODO(), &ec2.DescribeTagsInput{MaxResults: aws.Int64(5)}).Return(nil, tc.Error)

			cloud := &Cloud{
				ec2: ec2svc,
			}

			err := cloud.StatusEC2()()
			assert.Equal(t, tc.ExpectedError, err)
			ec2svc.AssertExpectations(t)
		})
	}
}

func TestCloud_ModifyNetworkInterfaceAttributeWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.EC2API{}

		i := &ec2.ModifyNetworkInterfaceAttributeInput{}
		o := &ec2.ModifyNetworkInterfaceAttributeOutput{}
		var e error

		svc.On("ModifyNetworkInterfaceAttributeWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			ec2: svc,
		}

		a, b := cloud.ModifyNetworkInterfaceAttributeWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_CreateSecurityGroupWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.EC2API{}

		i := &ec2.CreateSecurityGroupInput{}
		o := &ec2.CreateSecurityGroupOutput{}
		var e error

		svc.On("CreateSecurityGroupWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			ec2: svc,
		}

		a, b := cloud.CreateSecurityGroupWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_AuthorizeSecurityGroupIngressWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.EC2API{}

		i := &ec2.AuthorizeSecurityGroupIngressInput{}
		o := &ec2.AuthorizeSecurityGroupIngressOutput{}
		var e error

		svc.On("AuthorizeSecurityGroupIngressWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			ec2: svc,
		}

		a, b := cloud.AuthorizeSecurityGroupIngressWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_CreateTagsWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.EC2API{}

		i := &ec2.CreateTagsInput{}
		o := &ec2.CreateTagsOutput{}
		var e error

		svc.On("CreateTagsWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			ec2: svc,
		}

		a, b := cloud.CreateEC2TagsWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_RevokeSecurityGroupIngressWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.EC2API{}

		i := &ec2.RevokeSecurityGroupIngressInput{}
		o := &ec2.RevokeSecurityGroupIngressOutput{}
		var e error

		svc.On("RevokeSecurityGroupIngressWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			ec2: svc,
		}

		a, b := cloud.RevokeSecurityGroupIngressWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_GetClusterSubnets(t *testing.T) {
	clusterName := "clusterName"
	internalSubnet1 := &ec2.Subnet{
		SubnetId: aws.String("arn:aws:ec2:region:account-id:subnet/subnet-id1"),
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("kubernetes.io/cluster/" + clusterName),
				Value: aws.String("owned"),
			},
			{
				Key:   aws.String("kubernetes.io/role/internal-elb"),
				Value: aws.String("1"),
			},
		},
	}
	internalSubnet2 := &ec2.Subnet{
		SubnetId: aws.String("arn:aws:ec2:region:account-id:subnet/subnet-id2"),
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("kubernetes.io/cluster/" + clusterName),
				Value: aws.String("owned"),
			},
			{
				Key:   aws.String("kubernetes.io/role/internal-elb"),
				Value: aws.String(""),
			},
		},
	}
	publicSubnet := &ec2.Subnet{
		SubnetId: aws.String("arn:aws:ec2:region:account-id:subnet/subnet-id3"),
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("kubernetes.io/cluster/" + clusterName),
				Value: aws.String("shared"),
			},
			{
				Key:   aws.String("kubernetes.io/role/elb"),
				Value: aws.String("1"),
			},
		},
	}

	for _, tc := range []struct {
		Name                  string
		DescribeSubnetsOutput *ec2.DescribeSubnetsOutput
		DescribeSubnetsError  error
		TagSubnetType         string
		ExpectedResult        []*ec2.Subnet
		ExpectedError         error
	}{
		{
			Name:          "No subnets returned",
			TagSubnetType: TagNameSubnetInternalELB,
			DescribeSubnetsOutput: &ec2.DescribeSubnetsOutput{
				NextToken: nil,
				Subnets:   []*ec2.Subnet{},
			},
		},
		{
			Name:          "Two internal subnets returned",
			TagSubnetType: TagNameSubnetInternalELB,
			DescribeSubnetsOutput: &ec2.DescribeSubnetsOutput{
				NextToken: nil,
				Subnets:   []*ec2.Subnet{internalSubnet1, internalSubnet2},
			},
			ExpectedResult: []*ec2.Subnet{internalSubnet1, internalSubnet2},
		},
		{
			Name:          "One public subnet returned",
			TagSubnetType: TagNameSubnetPublicELB,
			DescribeSubnetsOutput: &ec2.DescribeSubnetsOutput{
				NextToken: nil,
				Subnets:   []*ec2.Subnet{publicSubnet},
			},
			ExpectedResult: []*ec2.Subnet{publicSubnet},
		},
		{
			Name:          "Error from API call",
			TagSubnetType: TagNameSubnetPublicELB,
			DescribeSubnetsOutput: &ec2.DescribeSubnetsOutput{
				NextToken: nil,
				Subnets:   []*ec2.Subnet{},
			},
			DescribeSubnetsError: errors.New("Some API error"),
			ExpectedError:        errors.New("Some API error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			svc := &mocks.EC2API{}

			svc.On("DescribeSubnetsPages",
				&ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{
					{
						Name:   aws.String("tag:kubernetes.io/cluster/" + clusterName),
						Values: aws.StringSlice([]string{"owned", "shared"}),
					},
					{
						Name:   aws.String("tag:" + tc.TagSubnetType),
						Values: aws.StringSlice([]string{"", "1"}),
					}},
				},
				mock.AnythingOfType("func(*ec2.DescribeSubnetsOutput, bool) bool"),
			).Return(tc.DescribeSubnetsError).Run(func(args mock.Arguments) {
				arg := args.Get(1).(func(*ec2.DescribeSubnetsOutput, bool) bool)
				arg(tc.DescribeSubnetsOutput, false)
			})

			cloud := &Cloud{
				clusterName: clusterName,
				ec2:         svc,
			}
			subnets, err := cloud.GetClusterSubnets(tc.TagSubnetType)
			assert.Equal(t, tc.ExpectedResult, subnets)
			assert.Equal(t, tc.ExpectedError, err)
			svc.AssertExpectations(t)
		})
	}
}
