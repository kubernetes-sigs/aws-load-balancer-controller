package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
)

func TestCloud_StatusELBV2(t *testing.T) {
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
			ExpectedError: errors.New("[elbv2.DescribeLoadBalancersWithContext]: Some API error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			elbv2svc := &mocks.ELBV2API{}
			elbv2svc.On("DescribeLoadBalancersWithContext", context.TODO(), &elbv2.DescribeLoadBalancersInput{PageSize: aws.Int64(1)}).Return(nil, tc.Error)

			cloud := &Cloud{
				elbv2: elbv2svc,
			}

			err := cloud.StatusELBV2()()
			assert.Equal(t, tc.ExpectedError, err)
			elbv2svc.AssertExpectations(t)
		})
	}
}

func TestCloud_GetRules(t *testing.T) {
	for _, tc := range []struct {
		Name                string
		ListenerArn         string
		DescribeRulesOutput *elbv2.DescribeRulesOutput
		DescribeRulesError  error
		ExpectedRules       []*elbv2.Rule
		ExpectedError       error
	}{
		{
			Name:        "Rules are returned",
			ListenerArn: "arn",
			DescribeRulesOutput: &elbv2.DescribeRulesOutput{
				Rules: []*elbv2.Rule{
					{RuleArn: aws.String("some arn")},
					{RuleArn: aws.String("some other arn")},
				},
			},
			ExpectedRules: []*elbv2.Rule{
				{RuleArn: aws.String("some arn")},
				{RuleArn: aws.String("some other arn")},
			},
		},
		{
			Name:               "DescribeRules has an API error",
			ListenerArn:        "arn",
			DescribeRulesError: errors.New("some API error"),
			ExpectedError:      errors.New("some API error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			elbv2svc := &mocks.ELBV2API{}

			elbv2svc.On("DescribeRulesRequest",
				&elbv2.DescribeRulesInput{ListenerArn: aws.String(tc.ListenerArn)}).Return(newReq(tc.DescribeRulesOutput, tc.DescribeRulesError), nil)
			cloud := &Cloud{
				elbv2: elbv2svc,
			}
			rules, err := cloud.GetRules(ctx, tc.ListenerArn)
			assert.Equal(t, tc.ExpectedRules, rules)
			assert.Equal(t, tc.ExpectedError, err)
			elbv2svc.AssertExpectations(t)
		})
	}
}

func TestCloud_DescribeTargetGroupAttributesWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.DescribeTargetGroupAttributesInput{}
		o := &elbv2.DescribeTargetGroupAttributesOutput{}
		var e error

		svc.On("DescribeTargetGroupAttributesWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.DescribeTargetGroupAttributesWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_ModifyTargetGroupAttributesWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.ModifyTargetGroupAttributesInput{}
		o := &elbv2.ModifyTargetGroupAttributesOutput{}
		var e error

		svc.On("ModifyTargetGroupAttributesWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.ModifyTargetGroupAttributesWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_CreateTargetGroupWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.CreateTargetGroupInput{}
		o := &elbv2.CreateTargetGroupOutput{}
		var e error

		svc.On("CreateTargetGroupWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.CreateTargetGroupWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_ModifyTargetGroupWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.ModifyTargetGroupInput{}
		o := &elbv2.ModifyTargetGroupOutput{}
		var e error

		svc.On("ModifyTargetGroupWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.ModifyTargetGroupWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_RegisterTargetsWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.RegisterTargetsInput{}
		o := &elbv2.RegisterTargetsOutput{}
		var e error

		svc.On("RegisterTargetsWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.RegisterTargetsWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_DeregisterTargetsWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.DeregisterTargetsInput{}
		o := &elbv2.DeregisterTargetsOutput{}
		var e error

		svc.On("DeregisterTargetsWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.DeregisterTargetsWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_DescribeTargetHealthWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.DescribeTargetHealthInput{}
		o := &elbv2.DescribeTargetHealthOutput{}
		var e error

		svc.On("DescribeTargetHealthWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.DescribeTargetHealthWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_CreateRuleWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.CreateRuleInput{}
		o := &elbv2.CreateRuleOutput{}
		var e error

		svc.On("CreateRuleWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.CreateRuleWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_ModifyRuleWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.ModifyRuleInput{}
		o := &elbv2.ModifyRuleOutput{}
		var e error

		svc.On("ModifyRuleWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.ModifyRuleWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_DeleteRuleWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.DeleteRuleInput{}
		o := &elbv2.DeleteRuleOutput{}
		var e error

		svc.On("DeleteRuleWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.DeleteRuleWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_SetSecurityGroupsWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.SetSecurityGroupsInput{}
		o := &elbv2.SetSecurityGroupsOutput{}
		var e error

		svc.On("SetSecurityGroupsWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.SetSecurityGroupsWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_CreateListenerWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.CreateListenerInput{}
		o := &elbv2.CreateListenerOutput{}
		var e error

		svc.On("CreateListenerWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.CreateListenerWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_ModifyListenerWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.ModifyListenerInput{}
		o := &elbv2.ModifyListenerOutput{}
		var e error

		svc.On("ModifyListenerWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.ModifyListenerWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_DescribeLoadBalancerAttributesWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.DescribeLoadBalancerAttributesInput{}
		o := &elbv2.DescribeLoadBalancerAttributesOutput{}
		var e error

		svc.On("DescribeLoadBalancerAttributesWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.DescribeLoadBalancerAttributesWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_ModifyLoadBalancerAttributesWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.ModifyLoadBalancerAttributesInput{}
		o := &elbv2.ModifyLoadBalancerAttributesOutput{}
		var e error

		svc.On("ModifyLoadBalancerAttributesWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.ModifyLoadBalancerAttributesWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_CreateLoadBalancerWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.CreateLoadBalancerInput{}
		o := &elbv2.CreateLoadBalancerOutput{}
		var e error

		svc.On("CreateLoadBalancerWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.CreateLoadBalancerWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_SetIpAddressTypeWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.SetIpAddressTypeInput{}
		o := &elbv2.SetIpAddressTypeOutput{}
		var e error

		svc.On("SetIpAddressTypeWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.SetIpAddressTypeWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_SetSubnetsWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.SetSubnetsInput{}
		o := &elbv2.SetSubnetsOutput{}
		var e error

		svc.On("SetSubnetsWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.SetSubnetsWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_DescribeELBV2TagsWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ELBV2API{}

		i := &elbv2.DescribeTagsInput{}
		o := &elbv2.DescribeTagsOutput{}
		var e error

		svc.On("DescribeTagsWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			elbv2: svc,
		}

		a, b := cloud.DescribeELBV2TagsWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}
