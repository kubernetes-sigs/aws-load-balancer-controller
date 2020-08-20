package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
			Error:         awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
			ExpectedError: errors.New("[elbv2.DescribeLoadBalancersWithContext]: ResponseTimeout: timeout"),
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
			Name:               "DescribeRules has an API timeout",
			ListenerArn:        "arn",
			DescribeRulesError: awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
			ExpectedError:      awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			elbv2svc := &mocks.ELBV2API{}

			elbv2svc.On("DescribeRulesRequest",
				&elbv2.DescribeRulesInput{
					ListenerArn: aws.String(tc.ListenerArn),
				},
			).Return(
				newReq(tc.DescribeRulesOutput, tc.DescribeRulesError),
				nil,
			)
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

func TestCloud_ListListenersByLoadBalancer(t *testing.T) {
	for _, tc := range []struct {
		Name                    string
		LbArn                   string
		DescribeListenersOutput *elbv2.DescribeListenersOutput
		DescribeListenersError  error
		ExpectedListeners       []*elbv2.Listener
		ExpectedError           error
	}{
		{
			Name:  "Listeners are returned",
			LbArn: "arn",
			DescribeListenersOutput: &elbv2.DescribeListenersOutput{
				Listeners: []*elbv2.Listener{
					{ListenerArn: aws.String("some arn")},
					{ListenerArn: aws.String("some other arn")},
				},
			},
			ExpectedListeners: []*elbv2.Listener{
				{ListenerArn: aws.String("some arn")},
				{ListenerArn: aws.String("some other arn")},
			},
		},
		{
			Name:                   "DescribeListeners has an API timeout",
			LbArn:                  "arn",
			DescribeListenersError: awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
			ExpectedError:          awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			elbv2svc := &mocks.ELBV2API{}

			elbv2svc.On("DescribeListenersPagesWithContext",
				ctx,
				&elbv2.DescribeListenersInput{LoadBalancerArn: aws.String(tc.LbArn)},
				mock.AnythingOfType("func(*elbv2.DescribeListenersOutput, bool) bool"),
			).Return(tc.DescribeListenersError).Run(func(args mock.Arguments) {
				arg := args.Get(2).(func(*elbv2.DescribeListenersOutput, bool) bool)
				arg(tc.DescribeListenersOutput, false)
			})
			cloud := &Cloud{
				elbv2: elbv2svc,
			}
			listeners, err := cloud.ListListenersByLoadBalancer(ctx, tc.LbArn)
			assert.Equal(t, tc.ExpectedListeners, listeners)
			assert.Equal(t, tc.ExpectedError, err)
			elbv2svc.AssertExpectations(t)
		})
	}
}

func TestCloud_DeleteListenersByArn(t *testing.T) {
	t.Run("Delete a listener", func(t *testing.T) {
		lsArn := "listenerArn"
		expectedError := awserr.New(request.ErrCodeResponseTimeout, "timeout", nil)
		ctx := context.Background()
		svc := &mocks.ELBV2API{}
		svc.On("DeleteListenerWithContext",
			ctx,
			&elbv2.DeleteListenerInput{ListenerArn: aws.String(lsArn)},
		).Return(
			&elbv2.DeleteListenerOutput{},
			expectedError,
		)
		cloud := &Cloud{elbv2: svc}
		err := cloud.DeleteListenersByArn(ctx, lsArn)
		assert.Equal(t, expectedError, err)
		svc.AssertExpectations(t)
	})
}

func TestCloud_GetLoadBalancerByArn(t *testing.T) {
	lb1 := &elbv2.LoadBalancer{LoadBalancerArn: aws.String("lbArn1")}
	lb2 := &elbv2.LoadBalancer{LoadBalancerArn: aws.String("lbArn2")}
	for _, tc := range []struct {
		Name                             string
		LbArn                            string
		DescribeLoadBalancersPagesOutput *elbv2.DescribeLoadBalancersOutput
		DescribeLoadBalancersPagesError  error
		ExpectedLoadBalancer             *elbv2.LoadBalancer
		ExpectedError                    error
	}{
		{
			Name:  "The requested load balancer is returned",
			LbArn: "lbArn1",
			DescribeLoadBalancersPagesOutput: &elbv2.DescribeLoadBalancersOutput{
				LoadBalancers: []*elbv2.LoadBalancer{lb1},
			},
			ExpectedLoadBalancer: lb1,
		},
		{
			Name:  "More than the requested load balancer is returned",
			LbArn: "lbArn1",
			DescribeLoadBalancersPagesOutput: &elbv2.DescribeLoadBalancersOutput{
				LoadBalancers: []*elbv2.LoadBalancer{lb1, lb2},
			},
			ExpectedLoadBalancer: lb1,
		},
		{
			Name:  "Load balancer doesn't exist",
			LbArn: "lbArn1",
			DescribeLoadBalancersPagesOutput: &elbv2.DescribeLoadBalancersOutput{
				LoadBalancers: []*elbv2.LoadBalancer{},
			},
		},
		{
			Name:                            "API timeout",
			LbArn:                           "lbArn1",
			DescribeLoadBalancersPagesError: awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
			ExpectedError:                   awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			elbv2svc := &mocks.ELBV2API{}

			elbv2svc.On("DescribeLoadBalancersPages",
				&elbv2.DescribeLoadBalancersInput{
					LoadBalancerArns: []*string{aws.String(tc.LbArn)},
				},
				mock.AnythingOfType("func(*elbv2.DescribeLoadBalancersOutput, bool) bool"),
			).Return(tc.DescribeLoadBalancersPagesError).Run(func(args mock.Arguments) {
				arg := args.Get(1).(func(*elbv2.DescribeLoadBalancersOutput, bool) bool)
				arg(tc.DescribeLoadBalancersPagesOutput, false)
			})
			// })
			cloud := &Cloud{
				elbv2: elbv2svc,
			}
			loadbalancer, err := cloud.GetLoadBalancerByArn(ctx, tc.LbArn)
			assert.Equal(t, tc.ExpectedLoadBalancer, loadbalancer)
			assert.Equal(t, tc.ExpectedError, err)
			elbv2svc.AssertExpectations(t)
		})
	}
}

func TestCloud_GetLoadBalancerByName(t *testing.T) {
	lb1 := &elbv2.LoadBalancer{LoadBalancerName: aws.String("name1")}
	lb2 := &elbv2.LoadBalancer{LoadBalancerName: aws.String("name2")}
	for _, tc := range []struct {
		Name                             string
		LbName                           string
		DescribeLoadBalancersPagesOutput *elbv2.DescribeLoadBalancersOutput
		DescribeLoadBalancersPagesError  error
		ExpectedLoadBalancer             *elbv2.LoadBalancer
		ExpectedError                    error
	}{
		{
			Name:   "The requested load balancer is returned",
			LbName: "name1",
			DescribeLoadBalancersPagesOutput: &elbv2.DescribeLoadBalancersOutput{
				LoadBalancers: []*elbv2.LoadBalancer{lb1},
			},
			ExpectedLoadBalancer: lb1,
		},
		{
			Name:   "More than the requested load balancer is returned",
			LbName: "name1",
			DescribeLoadBalancersPagesOutput: &elbv2.DescribeLoadBalancersOutput{
				LoadBalancers: []*elbv2.LoadBalancer{lb1, lb2},
			},
			ExpectedLoadBalancer: lb1,
		},
		{
			Name:   "Load balancer doesn't exist",
			LbName: "name1",
			DescribeLoadBalancersPagesOutput: &elbv2.DescribeLoadBalancersOutput{
				LoadBalancers: []*elbv2.LoadBalancer{},
			},
		},
		{
			Name:                            "API throws an error",
			LbName:                          "lbArn1",
			DescribeLoadBalancersPagesError: awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
			ExpectedError:                   awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
		},
		{
			Name:                            "API throws an error that is LB not found",
			LbName:                          "lbArn1",
			DescribeLoadBalancersPagesError: awserr.New(elbv2.ErrCodeLoadBalancerNotFoundException, "not found!", nil),
			ExpectedError:                   nil,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			elbv2svc := &mocks.ELBV2API{}

			elbv2svc.On("DescribeLoadBalancersPages",
				&elbv2.DescribeLoadBalancersInput{
					Names: []*string{aws.String(tc.LbName)},
				},
				mock.AnythingOfType("func(*elbv2.DescribeLoadBalancersOutput, bool) bool"),
			).Return(tc.DescribeLoadBalancersPagesError).Run(func(args mock.Arguments) {
				arg := args.Get(1).(func(*elbv2.DescribeLoadBalancersOutput, bool) bool)
				arg(tc.DescribeLoadBalancersPagesOutput, false)
			})
			// })
			cloud := &Cloud{
				elbv2: elbv2svc,
			}
			loadbalancer, err := cloud.GetLoadBalancerByName(ctx, tc.LbName)
			assert.Equal(t, tc.ExpectedLoadBalancer, loadbalancer)
			assert.Equal(t, tc.ExpectedError, err)
			elbv2svc.AssertExpectations(t)
		})
	}
}

func TestCloud_DeleteLoadBalancerByArn(t *testing.T) {
	t.Run("Delete a load balancer", func(t *testing.T) {
		lbArn := "loadbalancerArn"
		expectedError := awserr.New(request.ErrCodeResponseTimeout, "timeout", nil)
		ctx := context.Background()
		svc := &mocks.ELBV2API{}
		svc.On("DeleteLoadBalancerWithContext",
			ctx,
			&elbv2.DeleteLoadBalancerInput{LoadBalancerArn: aws.String(lbArn)},
		).Return(
			&elbv2.DeleteLoadBalancerOutput{},
			expectedError,
		)
		cloud := &Cloud{elbv2: svc}
		err := cloud.DeleteLoadBalancerByArn(ctx, lbArn)
		assert.Equal(t, expectedError, err)
		svc.AssertExpectations(t)
	})
}

func TestCloud_GetTargetGroupByArn(t *testing.T) {
	tg1 := &elbv2.TargetGroup{TargetGroupArn: aws.String("tgArn1")}
	tg2 := &elbv2.TargetGroup{TargetGroupArn: aws.String("tgArn2")}
	for _, tc := range []struct {
		Name                            string
		TgArn                           string
		DescribeTargetGroupsPagesOutput *elbv2.DescribeTargetGroupsOutput
		DescribeTargetGroupsPagesError  error
		ExpectedTargetGroup             *elbv2.TargetGroup
		ExpectedError                   error
	}{
		{
			Name:  "The requested target group is returned",
			TgArn: "tgArn1",
			DescribeTargetGroupsPagesOutput: &elbv2.DescribeTargetGroupsOutput{
				TargetGroups: []*elbv2.TargetGroup{tg1},
			},
			ExpectedTargetGroup: tg1,
		},
		{
			Name:  "More than the requested target group is returned",
			TgArn: "tgArn1",
			DescribeTargetGroupsPagesOutput: &elbv2.DescribeTargetGroupsOutput{
				TargetGroups: []*elbv2.TargetGroup{tg1, tg2},
			},
			ExpectedTargetGroup: tg1,
		},
		{
			Name:  "Target group doesn't exist",
			TgArn: "tgArn1",
			DescribeTargetGroupsPagesOutput: &elbv2.DescribeTargetGroupsOutput{
				TargetGroups: []*elbv2.TargetGroup{},
			},
		},
		{
			Name:                           "API timeout",
			TgArn:                          "tgArn1",
			DescribeTargetGroupsPagesError: awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
			ExpectedError:                  awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			elbv2svc := &mocks.ELBV2API{}

			elbv2svc.On("DescribeTargetGroupsPages",
				&elbv2.DescribeTargetGroupsInput{
					TargetGroupArns: []*string{aws.String(tc.TgArn)},
				},
				mock.AnythingOfType("func(*elbv2.DescribeTargetGroupsOutput, bool) bool"),
			).Return(tc.DescribeTargetGroupsPagesError).Run(func(args mock.Arguments) {
				arg := args.Get(1).(func(output *elbv2.DescribeTargetGroupsOutput, _ bool) bool)
				arg(tc.DescribeTargetGroupsPagesOutput, false)
			})
			// })
			cloud := &Cloud{
				elbv2: elbv2svc,
			}
			targetgroup, err := cloud.GetTargetGroupByArn(ctx, tc.TgArn)
			assert.Equal(t, tc.ExpectedTargetGroup, targetgroup)
			assert.Equal(t, tc.ExpectedError, err)
			elbv2svc.AssertExpectations(t)
		})
	}
}

func TestCloud_GetTargetGroupByName(t *testing.T) {
	tg1 := &elbv2.TargetGroup{TargetGroupName: aws.String("name1")}
	tg2 := &elbv2.TargetGroup{TargetGroupName: aws.String("name2")}
	for _, tc := range []struct {
		Name                            string
		TgName                          string
		DescribeTargetGroupsPagesOutput *elbv2.DescribeTargetGroupsOutput
		DescribeTargetGroupsPagesError  error
		ExpectedTargetGroup             *elbv2.TargetGroup
		ExpectedError                   error
	}{
		{
			Name:   "The requested target group is returned",
			TgName: "name1",
			DescribeTargetGroupsPagesOutput: &elbv2.DescribeTargetGroupsOutput{
				TargetGroups: []*elbv2.TargetGroup{tg1},
			},
			ExpectedTargetGroup: tg1,
		},
		{
			Name:   "More than the requested target group is returned",
			TgName: "name1",
			DescribeTargetGroupsPagesOutput: &elbv2.DescribeTargetGroupsOutput{
				TargetGroups: []*elbv2.TargetGroup{tg1, tg2},
			},
			ExpectedTargetGroup: tg1,
		},
		{
			Name:   "Target Group doesn't exist",
			TgName: "name1",
			DescribeTargetGroupsPagesOutput: &elbv2.DescribeTargetGroupsOutput{
				TargetGroups: []*elbv2.TargetGroup{},
			},
		},
		{
			Name:                           "API throws an error",
			TgName:                         "lbArn1",
			DescribeTargetGroupsPagesError: awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
			ExpectedError:                  awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
		},
		{
			Name:                           "API throws an error that is LB not found",
			TgName:                         "lbArn1",
			DescribeTargetGroupsPagesError: awserr.New(elbv2.ErrCodeTargetGroupNotFoundException, "not found!", nil),
			ExpectedError:                  nil,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			elbv2svc := &mocks.ELBV2API{}

			elbv2svc.On("DescribeTargetGroupsPages",
				&elbv2.DescribeTargetGroupsInput{
					Names: []*string{aws.String(tc.TgName)},
				},
				mock.AnythingOfType("func(*elbv2.DescribeTargetGroupsOutput, bool) bool"),
			).Return(tc.DescribeTargetGroupsPagesError).Run(func(args mock.Arguments) {
				arg := args.Get(1).(func(*elbv2.DescribeTargetGroupsOutput, bool) bool)
				arg(tc.DescribeTargetGroupsPagesOutput, false)
			})
			// })
			cloud := &Cloud{
				elbv2: elbv2svc,
			}
			targetgroup, err := cloud.GetTargetGroupByName(ctx, tc.TgName)
			assert.Equal(t, tc.ExpectedTargetGroup, targetgroup)
			assert.Equal(t, tc.ExpectedError, err)
			elbv2svc.AssertExpectations(t)
		})
	}
}

func TestCloud_GetTargetGroupsByLbArn(t *testing.T) {
	lbArn := "lbArn"
	tg1 := &elbv2.TargetGroup{TargetGroupArn: aws.String("tgArn1")}
	tg2 := &elbv2.TargetGroup{TargetGroupArn: aws.String("tgArn2")}
	for _, tc := range []struct {
		Name                            string
		LbArn                           string
		DescribeTargetGroupsPagesOutput *elbv2.DescribeTargetGroupsOutput
		DescribeTargetGroupsPagesError  error
		ExpectedTargetGroup             []*elbv2.TargetGroup
		ExpectedError                   error
	}{
		{
			Name:  "The requested target group is returned",
			LbArn: lbArn,
			DescribeTargetGroupsPagesOutput: &elbv2.DescribeTargetGroupsOutput{
				TargetGroups: []*elbv2.TargetGroup{tg1},
			},
			ExpectedTargetGroup: []*elbv2.TargetGroup{tg1},
		},
		{
			Name:  "More than the requested target group is returned",
			LbArn: lbArn,
			DescribeTargetGroupsPagesOutput: &elbv2.DescribeTargetGroupsOutput{
				TargetGroups: []*elbv2.TargetGroup{tg1, tg2},
			},
			ExpectedTargetGroup: []*elbv2.TargetGroup{tg1, tg2},
		},
		{
			Name:  "Target group doesn't exist",
			LbArn: lbArn,
			DescribeTargetGroupsPagesOutput: &elbv2.DescribeTargetGroupsOutput{
				TargetGroups: []*elbv2.TargetGroup{},
			},
		},
		{
			Name:                           "API timeout",
			LbArn:                          lbArn,
			DescribeTargetGroupsPagesError: awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
			ExpectedError:                  awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			elbv2svc := &mocks.ELBV2API{}

			elbv2svc.On("DescribeTargetGroupsPages",
				&elbv2.DescribeTargetGroupsInput{
					LoadBalancerArn: aws.String(tc.LbArn),
				},
				mock.AnythingOfType("func(*elbv2.DescribeTargetGroupsOutput, bool) bool"),
			).Return(tc.DescribeTargetGroupsPagesError).Run(func(args mock.Arguments) {
				arg := args.Get(1).(func(output *elbv2.DescribeTargetGroupsOutput, _ bool) bool)
				arg(tc.DescribeTargetGroupsPagesOutput, false)
			})

			cloud := &Cloud{
				elbv2: elbv2svc,
			}
			targetgroup, err := cloud.GetTargetGroupsByLbArn(ctx, tc.LbArn)
			assert.Equal(t, tc.ExpectedTargetGroup, targetgroup)
			assert.Equal(t, tc.ExpectedError, err)
			elbv2svc.AssertExpectations(t)
		})
	}
}

func TestCloud_DeleteTargetGroupByArn(t *testing.T) {
	t.Run("Delete a load balancer", func(t *testing.T) {
		tgArn := "targetgroupArn"
		expectedError := awserr.New(request.ErrCodeResponseTimeout, "timeout", nil)
		ctx := context.Background()
		svc := &mocks.ELBV2API{}
		svc.On("DeleteTargetGroupWithContext",
			ctx,
			&elbv2.DeleteTargetGroupInput{TargetGroupArn: aws.String(tgArn)},
		).Return(
			&elbv2.DeleteTargetGroupOutput{},
			expectedError,
		)
		cloud := &Cloud{elbv2: svc}
		err := cloud.DeleteTargetGroupByArn(ctx, tgArn)
		assert.Equal(t, expectedError, err)
		svc.AssertExpectations(t)
	})
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
