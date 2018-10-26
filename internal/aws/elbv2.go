package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

const (
	// Amount of time between each deletion attempt (or reattempt) for a target group
	deleteTargetGroupReattemptSleep int = 10
	// Maximum attempts should be made to delete a target group
	deleteTargetGroupReattemptMax int = 10
)

type ELBV2API interface {
	StatusELBV2() func() error

	GetRules(context.Context, string) ([]*elbv2.Rule, error)

	// ListListenersByLoadBalancer gets all listeners for loadbalancer.
	ListListenersByLoadBalancer(lbArn string) ([]*elbv2.Listener, error)

	// DeleteListenersByArn deletes listener
	DeleteListenersByArn(lsArn string) error

	// GetLoadBalancerByName retrieve LoadBalancer instance by arn
	GetLoadBalancerByArn(string) (*elbv2.LoadBalancer, error)

	// GetLoadBalancerByName retrieve LoadBalancer instance by name
	GetLoadBalancerByName(string) (*elbv2.LoadBalancer, error)

	// DeleteLoadBalancerByArn deletes LoadBalancer instance by arn
	DeleteLoadBalancerByArn(string) error

	// GetTargetGroupByArn retrieve TargetGroup instance by arn
	GetTargetGroupByArn(string) (*elbv2.TargetGroup, error)

	// GetTargetGroupByName retrieve TargetGroup instance by name
	GetTargetGroupByName(string) (*elbv2.TargetGroup, error)

	// DeleteTargetGroupByArn deletes TargetGroup instance by arn
	DeleteTargetGroupByArn(string) error

	DescribeTargetGroupAttributesWithContext(context.Context, *elbv2.DescribeTargetGroupAttributesInput) (*elbv2.DescribeTargetGroupAttributesOutput, error)
	ModifyTargetGroupAttributesWithContext(context.Context, *elbv2.ModifyTargetGroupAttributesInput) (*elbv2.ModifyTargetGroupAttributesOutput, error)
	CreateTargetGroupWithContext(context.Context, *elbv2.CreateTargetGroupInput) (*elbv2.CreateTargetGroupOutput, error)
	ModifyTargetGroupWithContext(context.Context, *elbv2.ModifyTargetGroupInput) (*elbv2.ModifyTargetGroupOutput, error)
	RegisterTargetsWithContext(context.Context, *elbv2.RegisterTargetsInput) (*elbv2.RegisterTargetsOutput, error)
	DeregisterTargetsWithContext(context.Context, *elbv2.DeregisterTargetsInput) (*elbv2.DeregisterTargetsOutput, error)
	DescribeTargetHealthWithContext(context.Context, *elbv2.DescribeTargetHealthInput) (*elbv2.DescribeTargetHealthOutput, error)
	CreateRuleWithContext(context.Context, *elbv2.CreateRuleInput) (*elbv2.CreateRuleOutput, error)
	ModifyRuleWithContext(context.Context, *elbv2.ModifyRuleInput) (*elbv2.ModifyRuleOutput, error)
	DeleteRuleWithContext(context.Context, *elbv2.DeleteRuleInput) (*elbv2.DeleteRuleOutput, error)
	SetSecurityGroupsWithContext(context.Context, *elbv2.SetSecurityGroupsInput) (*elbv2.SetSecurityGroupsOutput, error)
	CreateListenerWithContext(context.Context, *elbv2.CreateListenerInput) (*elbv2.CreateListenerOutput, error)
	ModifyListenerWithContext(context.Context, *elbv2.ModifyListenerInput) (*elbv2.ModifyListenerOutput, error)
	DescribeLoadBalancerAttributesWithContext(context.Context, *elbv2.DescribeLoadBalancerAttributesInput) (*elbv2.DescribeLoadBalancerAttributesOutput, error)
	ModifyLoadBalancerAttributesWithContext(context.Context, *elbv2.ModifyLoadBalancerAttributesInput) (*elbv2.ModifyLoadBalancerAttributesOutput, error)
	CreateLoadBalancerWithContext(context.Context, *elbv2.CreateLoadBalancerInput) (*elbv2.CreateLoadBalancerOutput, error)
	SetIpAddressTypeWithContext(context.Context, *elbv2.SetIpAddressTypeInput) (*elbv2.SetIpAddressTypeOutput, error)
	SetSubnetsWithContext(context.Context, *elbv2.SetSubnetsInput) (*elbv2.SetSubnetsOutput, error)
	DescribeELBV2TagsWithContext(context.Context, *elbv2.DescribeTagsInput) (*elbv2.DescribeTagsOutput, error)
}

func (c *Cloud) DescribeTargetGroupAttributesWithContext(ctx context.Context, i *elbv2.DescribeTargetGroupAttributesInput) (*elbv2.DescribeTargetGroupAttributesOutput, error) {
	return c.elbv2.DescribeTargetGroupAttributesWithContext(ctx, i)
}

func (c *Cloud) ModifyTargetGroupAttributesWithContext(ctx context.Context, i *elbv2.ModifyTargetGroupAttributesInput) (*elbv2.ModifyTargetGroupAttributesOutput, error) {
	return c.elbv2.ModifyTargetGroupAttributesWithContext(ctx, i)
}
func (c *Cloud) CreateTargetGroupWithContext(ctx context.Context, i *elbv2.CreateTargetGroupInput) (*elbv2.CreateTargetGroupOutput, error) {
	return c.elbv2.CreateTargetGroupWithContext(ctx, i)
}
func (c *Cloud) ModifyTargetGroupWithContext(ctx context.Context, i *elbv2.ModifyTargetGroupInput) (*elbv2.ModifyTargetGroupOutput, error) {
	return c.elbv2.ModifyTargetGroupWithContext(ctx, i)
}

func (c *Cloud) RegisterTargetsWithContext(ctx context.Context, i *elbv2.RegisterTargetsInput) (*elbv2.RegisterTargetsOutput, error) {
	return c.elbv2.RegisterTargetsWithContext(ctx, i)
}
func (c *Cloud) DeregisterTargetsWithContext(ctx context.Context, i *elbv2.DeregisterTargetsInput) (*elbv2.DeregisterTargetsOutput, error) {
	return c.elbv2.DeregisterTargetsWithContext(ctx, i)
}
func (c *Cloud) DescribeTargetHealthWithContext(ctx context.Context, i *elbv2.DescribeTargetHealthInput) (*elbv2.DescribeTargetHealthOutput, error) {
	return c.elbv2.DescribeTargetHealthWithContext(ctx, i)
}
func (c *Cloud) CreateRuleWithContext(ctx context.Context, i *elbv2.CreateRuleInput) (*elbv2.CreateRuleOutput, error) {
	return c.elbv2.CreateRuleWithContext(ctx, i)
}
func (c *Cloud) ModifyRuleWithContext(ctx context.Context, i *elbv2.ModifyRuleInput) (*elbv2.ModifyRuleOutput, error) {
	return c.elbv2.ModifyRuleWithContext(ctx, i)
}
func (c *Cloud) DeleteRuleWithContext(ctx context.Context, i *elbv2.DeleteRuleInput) (*elbv2.DeleteRuleOutput, error) {
	return c.elbv2.DeleteRuleWithContext(ctx, i)
}
func (c *Cloud) SetSecurityGroupsWithContext(ctx context.Context, i *elbv2.SetSecurityGroupsInput) (*elbv2.SetSecurityGroupsOutput, error) {
	return c.elbv2.SetSecurityGroupsWithContext(ctx, i)
}
func (c *Cloud) CreateListenerWithContext(ctx context.Context, i *elbv2.CreateListenerInput) (*elbv2.CreateListenerOutput, error) {
	return c.elbv2.CreateListenerWithContext(ctx, i)
}
func (c *Cloud) ModifyListenerWithContext(ctx context.Context, i *elbv2.ModifyListenerInput) (*elbv2.ModifyListenerOutput, error) {
	return c.elbv2.ModifyListenerWithContext(ctx, i)
}
func (c *Cloud) DescribeLoadBalancerAttributesWithContext(ctx context.Context, i *elbv2.DescribeLoadBalancerAttributesInput) (*elbv2.DescribeLoadBalancerAttributesOutput, error) {
	return c.elbv2.DescribeLoadBalancerAttributesWithContext(ctx, i)
}
func (c *Cloud) ModifyLoadBalancerAttributesWithContext(ctx context.Context, i *elbv2.ModifyLoadBalancerAttributesInput) (*elbv2.ModifyLoadBalancerAttributesOutput, error) {
	return c.elbv2.ModifyLoadBalancerAttributesWithContext(ctx, i)
}
func (c *Cloud) CreateLoadBalancerWithContext(ctx context.Context, i *elbv2.CreateLoadBalancerInput) (*elbv2.CreateLoadBalancerOutput, error) {
	return c.elbv2.CreateLoadBalancerWithContext(ctx, i)
}
func (c *Cloud) SetIpAddressTypeWithContext(ctx context.Context, i *elbv2.SetIpAddressTypeInput) (*elbv2.SetIpAddressTypeOutput, error) {
	return c.elbv2.SetIpAddressTypeWithContext(ctx, i)
}
func (c *Cloud) SetSubnetsWithContext(ctx context.Context, i *elbv2.SetSubnetsInput) (*elbv2.SetSubnetsOutput, error) {
	return c.elbv2.SetSubnetsWithContext(ctx, i)
}
func (c *Cloud) DescribeELBV2TagsWithContext(ctx context.Context, i *elbv2.DescribeTagsInput) (*elbv2.DescribeTagsOutput, error) {
	return c.elbv2.DescribeTagsWithContext(ctx, i)
}

func (c *Cloud) GetRules(ctx context.Context, listenerArn string) ([]*elbv2.Rule, error) {
	var rules []*elbv2.Rule

	p := request.Pagination{
		EndPageOnSameToken: true,
		NewRequest: func() (*request.Request, error) {
			req, _ := c.elbv2.DescribeRulesRequest(&elbv2.DescribeRulesInput{ListenerArn: aws.String(listenerArn)})
			req.SetContext(ctx)
			return req, nil
		},
	}
	for p.Next() {
		page := p.Page().(*elbv2.DescribeRulesOutput)
		for _, rule := range page.Rules {
			rules = append(rules, rule)
		}
	}
	return rules, p.Err()
}

// StatusELBV2 validates ELBV2 connectivity
func (c *Cloud) StatusELBV2() func() error {
	return func() error {
		in := &elbv2.DescribeLoadBalancersInput{PageSize: aws.Int64(1)}

		if _, err := c.elbv2.DescribeLoadBalancersWithContext(context.TODO(), in); err != nil {
			return fmt.Errorf("[elbv2.DescribeLoadBalancersWithContext]: %v", err)
		}
		return nil
	}
}

func (c *Cloud) ListListenersByLoadBalancer(lbArn string) ([]*elbv2.Listener, error) {
	var listeners []*elbv2.Listener
	err := c.elbv2.DescribeListenersPagesWithContext(context.Background(),
		&elbv2.DescribeListenersInput{LoadBalancerArn: aws.String(lbArn)},
		func(p *elbv2.DescribeListenersOutput, lastPage bool) bool {
			for _, listener := range p.Listeners {
				listeners = append(listeners, listener)
			}
			return true
		})
	if err != nil {
		return nil, err
	}

	return listeners, nil
}

func (c *Cloud) DeleteListenersByArn(lsArn string) error {
	_, err := c.elbv2.DeleteListener(&elbv2.DeleteListenerInput{
		ListenerArn: aws.String(lsArn),
	})
	return err
}

func (c *Cloud) GetLoadBalancerByArn(arn string) (*elbv2.LoadBalancer, error) {
	loadBalancers, err := c.describeLoadBalancersHelper(&elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []*string{aws.String(arn)},
	})
	if err != nil {
		return nil, err
	}
	if len(loadBalancers) == 0 {
		return nil, nil
	}
	return loadBalancers[0], nil
}

func (c *Cloud) GetLoadBalancerByName(name string) (*elbv2.LoadBalancer, error) {
	loadBalancers, err := c.describeLoadBalancersHelper(&elbv2.DescribeLoadBalancersInput{
		Names: []*string{aws.String(name)},
	})
	if err != nil {
		if awsError, ok := err.(awserr.Error); ok {
			if awsError.Code() == "LoadBalancerNotFound" {
				return nil, nil
			}
		}
		return nil, err
	}
	if len(loadBalancers) == 0 {
		return nil, nil
	}
	return loadBalancers[0], nil
}

func (c *Cloud) DeleteLoadBalancerByArn(arn string) error {
	_, err := c.elbv2.DeleteLoadBalancer(&elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: aws.String(arn),
	})
	return err
}

func (c *Cloud) GetTargetGroupByArn(arn string) (*elbv2.TargetGroup, error) {
	targetGroups, err := c.describeTargetGroupsHelper(&elbv2.DescribeTargetGroupsInput{
		TargetGroupArns: []*string{aws.String(arn)},
	})
	if err != nil {
		return nil, err
	}
	if len(targetGroups) == 0 {
		return nil, nil
	}
	return targetGroups[0], nil
}

// GetTargetGroupByName retrieve TargetGroup instance by name
func (c *Cloud) GetTargetGroupByName(name string) (*elbv2.TargetGroup, error) {
	targetGroups, err := c.describeTargetGroupsHelper(&elbv2.DescribeTargetGroupsInput{
		Names: []*string{aws.String(name)},
	})
	if err != nil {
		if awsError, ok := err.(awserr.Error); ok {
			if awsError.Code() == "TargetGroupNotFound" {
				return nil, nil
			}
		}
		return nil, err
	}
	if len(targetGroups) == 0 {
		return nil, nil
	}
	return targetGroups[0], nil
}

// DeleteTargetGroupByArn deletes TargetGroup instance by arn
func (c *Cloud) DeleteTargetGroupByArn(arn string) error {
	_, err := c.elbv2.DeleteTargetGroup(&elbv2.DeleteTargetGroupInput{
		TargetGroupArn: aws.String(arn),
	})
	return err
}

// describeLoadBalancersHelper is an helper to handle pagination in describeLoadBalancers call
func (c *Cloud) describeLoadBalancersHelper(input *elbv2.DescribeLoadBalancersInput) (result []*elbv2.LoadBalancer, err error) {
	err = c.elbv2.DescribeLoadBalancersPages(input, func(output *elbv2.DescribeLoadBalancersOutput, _ bool) bool {
		result = append(result, output.LoadBalancers...)
		return true
	})
	return result, err
}

// describeTargetGroupsHelper is an helper t handle pagination in describeTargetGroups call
func (c *Cloud) describeTargetGroupsHelper(input *elbv2.DescribeTargetGroupsInput) (result []*elbv2.TargetGroup, err error) {
	err = c.elbv2.DescribeTargetGroupsPages(input, func(output *elbv2.DescribeTargetGroupsOutput, _ bool) bool {
		result = append(result, output.TargetGroups...)
		return true
	})
	return result, err
}
