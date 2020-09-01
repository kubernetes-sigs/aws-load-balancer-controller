package services

import (
	"context"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
)

type ELBV2 interface {
	elbv2iface.ELBV2API

	// wrapper to DescribeLoadBalancersPagesWithContext API, which aggregates paged results into list.
	DescribeLoadBalancersAsList(ctx context.Context, input *elbv2.DescribeLoadBalancersInput) ([]*elbv2.LoadBalancer, error)

	// wrapper to DescribeTargetGroupsPagesWithContext API, which aggregates paged results into list.
	DescribeTargetGroupsAsList(ctx context.Context, input *elbv2.DescribeTargetGroupsInput) ([]*elbv2.TargetGroup, error)
}

// NewELBV2 constructs new ELBV2 implementation.
func NewELBV2(session *session.Session) ELBV2 {
	return &defaultELBV2{
		ELBV2API: elbv2.New(session),
	}
}

// default implementation for ELBV2.
type defaultELBV2 struct {
	elbv2iface.ELBV2API
}

func (c *defaultELBV2) DescribeLoadBalancersAsList(ctx context.Context, input *elbv2.DescribeLoadBalancersInput) ([]*elbv2.LoadBalancer, error) {
	var result []*elbv2.LoadBalancer
	if err := c.DescribeLoadBalancersPagesWithContext(ctx, input, func(output *elbv2.DescribeLoadBalancersOutput, _ bool) bool {
		result = append(result, output.LoadBalancers...)
		return true
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *defaultELBV2) DescribeTargetGroupsAsList(ctx context.Context, input *elbv2.DescribeTargetGroupsInput) ([]*elbv2.TargetGroup, error) {
	var result []*elbv2.TargetGroup
	if err := c.DescribeTargetGroupsPagesWithContext(ctx, input, func(output *elbv2.DescribeTargetGroupsOutput, _ bool) bool {
		result = append(result, output.TargetGroups...)
		return true
	}); err != nil {
		return nil, err
	}
	return result, nil
}
