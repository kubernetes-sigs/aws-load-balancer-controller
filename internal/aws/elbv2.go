package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
)

const (
	// Amount of time between each deletion attempt (or reattempt) for a target group
	deleteTargetGroupReattemptSleep int = 10
	// Maximum attempts should be made to delete a target group
	deleteTargetGroupReattemptMax int = 10
)

type ELBV2API interface {
	ClusterLoadBalancers() ([]*elbv2.LoadBalancer, error)
	ClusterTargetGroups() (map[string][]*elbv2.TargetGroup, error)
	RemoveTargetGroup(arn *string) error
	RemoveListener(arn *string) error

	StatusELBV2() func() error
	SetField(string, interface{})

	GetRules(string) ([]*elbv2.Rule, error)

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

	DescribeTargetGroupAttributes(*elbv2.DescribeTargetGroupAttributesInput) (*elbv2.DescribeTargetGroupAttributesOutput, error)
	ModifyTargetGroupAttributes(*elbv2.ModifyTargetGroupAttributesInput) (*elbv2.ModifyTargetGroupAttributesOutput, error)
	CreateTargetGroup(*elbv2.CreateTargetGroupInput) (*elbv2.CreateTargetGroupOutput, error)
	ModifyTargetGroup(*elbv2.ModifyTargetGroupInput) (*elbv2.ModifyTargetGroupOutput, error)
	RegisterTargets(*elbv2.RegisterTargetsInput) (*elbv2.RegisterTargetsOutput, error)
	DeregisterTargets(*elbv2.DeregisterTargetsInput) (*elbv2.DeregisterTargetsOutput, error)
	DescribeTargetHealth(*elbv2.DescribeTargetHealthInput) (*elbv2.DescribeTargetHealthOutput, error)
	CreateRule(*elbv2.CreateRuleInput) (*elbv2.CreateRuleOutput, error)
	ModifyRule(*elbv2.ModifyRuleInput) (*elbv2.ModifyRuleOutput, error)
	DeleteRule(*elbv2.DeleteRuleInput) (*elbv2.DeleteRuleOutput, error)
	SetSecurityGroups(*elbv2.SetSecurityGroupsInput) (*elbv2.SetSecurityGroupsOutput, error)
	CreateListener(*elbv2.CreateListenerInput) (*elbv2.CreateListenerOutput, error)
	ModifyListener(*elbv2.ModifyListenerInput) (*elbv2.ModifyListenerOutput, error)
	DescribeLoadBalancerAttributes(*elbv2.DescribeLoadBalancerAttributesInput) (*elbv2.DescribeLoadBalancerAttributesOutput, error)
	ModifyLoadBalancerAttributes(*elbv2.ModifyLoadBalancerAttributesInput) (*elbv2.ModifyLoadBalancerAttributesOutput, error)
	CreateLoadBalancer(*elbv2.CreateLoadBalancerInput) (*elbv2.CreateLoadBalancerOutput, error)
	SetIpAddressType(*elbv2.SetIpAddressTypeInput) (*elbv2.SetIpAddressTypeOutput, error)
	SetSubnets(*elbv2.SetSubnetsInput) (*elbv2.SetSubnetsOutput, error)
}

func (c *Cloud) DescribeTargetGroupAttributes(i *elbv2.DescribeTargetGroupAttributesInput) (*elbv2.DescribeTargetGroupAttributesOutput, error) {
	return c.elbv2.DescribeTargetGroupAttributes(i)
}

func (c *Cloud) ModifyTargetGroupAttributes(i *elbv2.ModifyTargetGroupAttributesInput) (*elbv2.ModifyTargetGroupAttributesOutput, error) {
	return c.elbv2.ModifyTargetGroupAttributes(i)
}
func (c *Cloud) CreateTargetGroup(i *elbv2.CreateTargetGroupInput) (*elbv2.CreateTargetGroupOutput, error) {
	return c.elbv2.CreateTargetGroup(i)
}
func (c *Cloud) ModifyTargetGroup(i *elbv2.ModifyTargetGroupInput) (*elbv2.ModifyTargetGroupOutput, error) {
	return c.elbv2.ModifyTargetGroup(i)
}

func (c *Cloud) RegisterTargets(i *elbv2.RegisterTargetsInput) (*elbv2.RegisterTargetsOutput, error) {
	return c.elbv2.RegisterTargets(i)
}
func (c *Cloud) DeregisterTargets(i *elbv2.DeregisterTargetsInput) (*elbv2.DeregisterTargetsOutput, error) {
	return c.elbv2.DeregisterTargets(i)
}
func (c *Cloud) DescribeTargetHealth(i *elbv2.DescribeTargetHealthInput) (*elbv2.DescribeTargetHealthOutput, error) {
	return c.elbv2.DescribeTargetHealth(i)
}
func (c *Cloud) CreateRule(i *elbv2.CreateRuleInput) (*elbv2.CreateRuleOutput, error) {
	return c.elbv2.CreateRule(i)
}
func (c *Cloud) ModifyRule(i *elbv2.ModifyRuleInput) (*elbv2.ModifyRuleOutput, error) {
	return c.elbv2.ModifyRule(i)
}
func (c *Cloud) DeleteRule(i *elbv2.DeleteRuleInput) (*elbv2.DeleteRuleOutput, error) {
	return c.elbv2.DeleteRule(i)
}
func (c *Cloud) SetSecurityGroups(i *elbv2.SetSecurityGroupsInput) (*elbv2.SetSecurityGroupsOutput, error) {
	return c.elbv2.SetSecurityGroups(i)
}
func (c *Cloud) CreateListener(i *elbv2.CreateListenerInput) (*elbv2.CreateListenerOutput, error) {
	return c.elbv2.CreateListener(i)
}
func (c *Cloud) ModifyListener(i *elbv2.ModifyListenerInput) (*elbv2.ModifyListenerOutput, error) {
	return c.elbv2.ModifyListener(i)
}
func (c *Cloud) DescribeLoadBalancerAttributes(i *elbv2.DescribeLoadBalancerAttributesInput) (*elbv2.DescribeLoadBalancerAttributesOutput, error) {
	return c.elbv2.DescribeLoadBalancerAttributes(i)
}
func (c *Cloud) ModifyLoadBalancerAttributes(i *elbv2.ModifyLoadBalancerAttributesInput) (*elbv2.ModifyLoadBalancerAttributesOutput, error) {
	return c.elbv2.ModifyLoadBalancerAttributes(i)
}
func (c *Cloud) CreateLoadBalancer(i *elbv2.CreateLoadBalancerInput) (*elbv2.CreateLoadBalancerOutput, error) {
	return c.elbv2.CreateLoadBalancer(i)
}
func (c *Cloud) SetIpAddressType(i *elbv2.SetIpAddressTypeInput) (*elbv2.SetIpAddressTypeOutput, error) {
	return c.elbv2.SetIpAddressType(i)
}
func (c *Cloud) SetSubnets(i *elbv2.SetSubnetsInput) (*elbv2.SetSubnetsOutput, error) {
	return c.elbv2.SetSubnets(i)
}

// RemoveListener removes a Listener from an ELBV2 (ALB) by deleting it in AWS. If the deletion
// attempt returns a elbv2.ErrCodeListenerNotFoundException, it's considered a success as the
// listener has already been removed. If removal fails for another reason, an error is returned.
func (c *Cloud) RemoveListener(arn *string) error {
	in := elbv2.DeleteListenerInput{
		ListenerArn: arn,
	}

	if _, err := c.elbv2.DeleteListener(&in); err != nil {
		awsErr := err.(awserr.Error)
		if awsErr.Code() != elbv2.ErrCodeListenerNotFoundException {
			return err
		}
	}

	return nil
}

// RemoveTargetGroup removes a Target Group from AWS by deleting it. If the deletion fails, an error
// is returned. Often, a Listener that references the Target Group is still being deleted when this
// method is accessed. Thus, this method makes multiple attempts to delete the Target Group when it
// receives an elbv2.ErrCodeResourceInUseException.
func (c *Cloud) RemoveTargetGroup(arn *string) error {
	in := &elbv2.DeleteTargetGroupInput{
		TargetGroupArn: arn,
	}
	for i := 0; i < deleteTargetGroupReattemptMax; i++ {
		_, err := c.elbv2.DeleteTargetGroup(in)
		if err == nil {
			return nil
		}

		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case elbv2.ErrCodeResourceInUseException:
				time.Sleep(time.Duration(deleteTargetGroupReattemptSleep) * time.Second)
			default:
				return aerr
			}
		} else {
			return aerr
		}
	}

	return fmt.Errorf("Timed out trying to delete target group %s", *arn)
}

// ClusterLoadBalancers looks up all ELBV2 (ALB) instances in AWS that are part of the cluster.
func (c *Cloud) ClusterLoadBalancers() ([]*elbv2.LoadBalancer, error) {
	var loadbalancers []*elbv2.LoadBalancer

	// BUG?: Does not filter based on ingress-class, should it?
	rgt, err := albrgt.RGTsvc.GetClusterResources()
	if err != nil {
		return nil, fmt.Errorf("Failed to get AWS tags. Error: %s", err.Error())
	}

	err = c.elbv2.DescribeLoadBalancersPages(&elbv2.DescribeLoadBalancersInput{}, func(page *elbv2.DescribeLoadBalancersOutput, _ bool) bool {
		for _, loadBalancer := range page.LoadBalancers {
			if _, ok := rgt.LoadBalancers[*loadBalancer.LoadBalancerArn]; ok {
				loadbalancers = append(loadbalancers, loadBalancer)
			}
		}
		return true
	})

	return loadbalancers, err
}

// ClusterTargetGroups fetches all target groups that are part of the cluster.
func (c *Cloud) ClusterTargetGroups() (map[string][]*elbv2.TargetGroup, error) {
	output := make(map[string][]*elbv2.TargetGroup)

	rgt, err := albrgt.RGTsvc.GetClusterResources()
	if err != nil {
		return nil, fmt.Errorf("Failed to get AWS tags. Error: %s", err.Error())
	}

	err = c.elbv2.DescribeTargetGroupsPages(&elbv2.DescribeTargetGroupsInput{}, func(page *elbv2.DescribeTargetGroupsOutput, _ bool) bool {
		for _, targetGroup := range page.TargetGroups {
			for _, lbarn := range targetGroup.LoadBalancerArns {
				if _, ok := rgt.LoadBalancers[*lbarn]; ok {
					output[*lbarn] = append(output[*lbarn], targetGroup)
				}
			}
		}
		return true
	})

	return output, err
}

func (c *Cloud) GetRules(listenerArn string) ([]*elbv2.Rule, error) {
	var rules []*elbv2.Rule

	p := request.Pagination{
		EndPageOnSameToken: true,
		NewRequest: func() (*request.Request, error) {
			req, _ := c.elbv2.DescribeRulesRequest(&elbv2.DescribeRulesInput{ListenerArn: aws.String(listenerArn)})
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
		in := &elbv2.DescribeLoadBalancersInput{}
		in.SetPageSize(1)

		if _, err := c.elbv2.DescribeLoadBalancers(in); err != nil {
			return fmt.Errorf("[elasticloadbalancer.DescribeLoadBalancers]: %v", err)
		}
		return nil
	}
}

func (c *Cloud) SetField(field string, v interface{}) {
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
