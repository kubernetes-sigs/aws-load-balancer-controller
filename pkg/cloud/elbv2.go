package cloud

import (
	"context"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
)

// ELBV2 is an wrapper around original ELBV2API with additional convenient APIs.
type ELBV2 interface {
	elbv2iface.ELBV2API

	DescribeLoadBalancersAsList(ctx context.Context, input *elbv2.DescribeLoadBalancersInput) ([]*elbv2.LoadBalancer, error)
	DescribeListenersAsList(ctx context.Context, input *elbv2.DescribeListenersInput) ([]*elbv2.Listener, error)
	DescribeListenerCertificatesAsList(ctx context.Context, input *elbv2.DescribeListenerCertificatesInput) ([]*elbv2.Certificate, error)
	DescribeRulesAsList(ctx context.Context, input *elbv2.DescribeRulesInput) ([]*elbv2.Rule, error)
	DescribeTargetGroupsAsList(ctx context.Context, input *elbv2.DescribeTargetGroupsInput) ([]*elbv2.TargetGroup, error)
}

func NewELBV2(session *session.Session) ELBV2 {
	return &defaultELBV2{
		elbv2.New(session),
	}
}

var _ ELBV2 = (*defaultELBV2)(nil)

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

func (c *defaultELBV2) DescribeListenersAsList(ctx context.Context, input *elbv2.DescribeListenersInput) ([]*elbv2.Listener, error) {
	var result []*elbv2.Listener
	if err := c.DescribeListenersPagesWithContext(ctx, input, func(output *elbv2.DescribeListenersOutput, _ bool) bool {
		result = append(result, output.Listeners...)
		return true
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *defaultELBV2) DescribeListenerCertificatesAsList(ctx context.Context, input *elbv2.DescribeListenerCertificatesInput) ([]*elbv2.Certificate, error) {
	var result []*elbv2.Certificate
	p := request.Pagination{
		EndPageOnSameToken: true,
		NewRequest: func() (*request.Request, error) {
			req, _ := c.DescribeListenerCertificatesRequest(input)
			req.SetContext(ctx)
			return req, nil
		},
	}
	for p.Next() {
		page := p.Page().(*elbv2.DescribeListenerCertificatesOutput)
		result = append(result, page.Certificates...)
	}
	return result, p.Err()
}

func (c *defaultELBV2) DescribeRulesAsList(ctx context.Context, input *elbv2.DescribeRulesInput) ([]*elbv2.Rule, error) {
	var rules []*elbv2.Rule

	p := request.Pagination{
		EndPageOnSameToken: true,
		NewRequest: func() (*request.Request, error) {
			req, _ := c.DescribeRulesRequest(input)
			req.SetContext(ctx)
			return req, nil
		},
	}
	for p.Next() {
		page := p.Page().(*elbv2.DescribeRulesOutput)
		rules = append(rules, page.Rules...)
	}
	return rules, p.Err()
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
