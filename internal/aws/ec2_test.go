package aws

import (
	"context"
	"errors"
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
