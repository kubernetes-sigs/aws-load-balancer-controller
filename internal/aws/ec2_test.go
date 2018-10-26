package aws

import (
	"context"
	"errors"
	"os"
	"sort"
	"testing"

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
			ec2svc.On("DescribeTagsWithContext", context.TODO(), &ec2.DescribeTagsInput{MaxResults: aws.Int64(1)}).Return(nil, tc.Error)

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

		a, b := cloud.CreateTagsWithContext(ctx, i)
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

func ec2Filter(k string, v ...string) *ec2.Filter {
	return &ec2.Filter{
		Name:   aws.String(k),
		Values: aws.StringSlice(v),
	}
}

func TestCloud_ResolveSecurityGroupNames(t *testing.T) {
	vpcid := "vpc-123456"
	os.Setenv("AWS_VPC_ID", vpcid)
	vpcFilter := &ec2.Filter{Name: aws.String("vpc-id"), Values: []*string{aws.String(vpcid)}}

	for _, tc := range []struct {
		Name   string
		Input  []string
		Output []string

		DescribeSecurityGroupsInput  *ec2.DescribeSecurityGroupsInput
		DescribeSecurityGroupsOutput *ec2.DescribeSecurityGroupsOutput

		Error         error
		ExpectedError error
	}{
		{
			Name:          "empty input, empty output",
			Error:         nil,
			ExpectedError: nil,
		},
		{
			Name:   "single resolved 'sg-' input",
			Input:  []string{"sg-123456"},
			Output: []string{"sg-123456"},
		},
		{
			Name:   "single named 'sg1' input",
			Input:  []string{"sg1"},
			Output: []string{"sg-123456"},
			DescribeSecurityGroupsInput: &ec2.DescribeSecurityGroupsInput{
				Filters: []*ec2.Filter{
					ec2Filter("tag:Name", "sg1"),
					vpcFilter,
				},
			},
			DescribeSecurityGroupsOutput: &ec2.DescribeSecurityGroupsOutput{
				SecurityGroups: []*ec2.SecurityGroup{
					{GroupId: aws.String("sg-123456")},
				},
			},
		},
		{
			Name:   "mixed named and unnamed input",
			Input:  []string{"sg1", "sg-567234"},
			Output: []string{"sg-123456", "sg-567234"},
			DescribeSecurityGroupsInput: &ec2.DescribeSecurityGroupsInput{
				Filters: []*ec2.Filter{
					ec2Filter("tag:Name", "sg1"),
					vpcFilter,
				},
			},
			DescribeSecurityGroupsOutput: &ec2.DescribeSecurityGroupsOutput{
				SecurityGroups: []*ec2.SecurityGroup{
					{GroupId: aws.String("sg-123456")},
				},
			},
		},
		{
			Name:   "a sg name that doesn't resolve",
			Input:  []string{"sg1", "sg-567234"},
			Output: []string{"sg-567234"},
			DescribeSecurityGroupsInput: &ec2.DescribeSecurityGroupsInput{
				Filters: []*ec2.Filter{
					ec2Filter("tag:Name", "sg1"),
					vpcFilter,
				},
			},
			DescribeSecurityGroupsOutput: &ec2.DescribeSecurityGroupsOutput{
				SecurityGroups: []*ec2.SecurityGroup{},
			},
			ExpectedError: errors.New("not all security groups were resolvable, (sg1,sg-567234 != sg-567234)"),
		},
		{
			Name:   "Error from API call",
			Input:  []string{"sg1", "sg-567234"},
			Output: []string{"sg-567234"},
			DescribeSecurityGroupsInput: &ec2.DescribeSecurityGroupsInput{
				Filters: []*ec2.Filter{
					ec2Filter("tag:Name", "sg1"),
					vpcFilter,
				},
			},
			Error:         errors.New("Some API error"),
			ExpectedError: errors.New("Unable to fetch security groups [{\n  Name: \"tag:Name\",\n  Values: [\"sg1\"]\n} {\n  Name: \"vpc-id\",\n  Values: [\"vpc-123456\"]\n}]: Some API error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			ec2svc := &mocks.EC2API{}
			if tc.DescribeSecurityGroupsInput != nil {
				ec2svc.On("DescribeSecurityGroupsWithContext",
					context.TODO(),
					tc.DescribeSecurityGroupsInput).Return(
					tc.DescribeSecurityGroupsOutput,
					tc.Error,
				)
			}

			cloud := &Cloud{
				ec2: ec2svc,
			}

			out, err := cloud.ResolveSecurityGroupNames(ctx, tc.Input)
			sort.Strings(tc.Output)
			sort.Strings(out)
			assert.Equal(t, tc.Output, out)
			assert.Equal(t, tc.ExpectedError, err)
			ec2svc.AssertExpectations(t)
		})
	}
}
