package services

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
)

type ELBV2 interface {
	// wrapper to DescribeLoadBalancersPagesWithContext API, which aggregates paged results into list.
	DescribeLoadBalancersAsList(ctx context.Context, input *elasticloadbalancingv2.DescribeLoadBalancersInput) ([]types.LoadBalancer, error)

	// wrapper to DescribeTargetGroupsPagesWithContext API, which aggregates paged results into list.
	DescribeTargetGroupsAsList(ctx context.Context, input *elasticloadbalancingv2.DescribeTargetGroupsInput) ([]types.TargetGroup, error)

	// wrapper to DescribeListenersPagesWithContext API, which aggregates paged results into list.
	DescribeListenersAsList(ctx context.Context, input *elasticloadbalancingv2.DescribeListenersInput) ([]types.Listener, error)

	// wrapper to DescribeListenerCertificatesWithContext API, which aggregates paged results into list.
	DescribeListenerCertificatesAsList(ctx context.Context, input *elasticloadbalancingv2.DescribeListenerCertificatesInput) ([]types.Certificate, error)

	// wrapper to DescribeRulesWithContext API, which aggregates paged results into list.
	DescribeRulesAsList(ctx context.Context, input *elasticloadbalancingv2.DescribeRulesInput) ([]types.Rule, error)

	AddTagsWithContext(ctx context.Context, input *elasticloadbalancingv2.AddTagsInput) (*elasticloadbalancingv2.AddTagsOutput, error)
	RemoveTagsWithContext(ctx context.Context, input *elasticloadbalancingv2.RemoveTagsInput) (*elasticloadbalancingv2.RemoveTagsOutput, error)
	DescribeTagsWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTagsInput) (*elasticloadbalancingv2.DescribeTagsOutput, error)
	CreateListenerWithContext(ctx context.Context, input *elasticloadbalancingv2.CreateListenerInput) (*elasticloadbalancingv2.CreateListenerOutput, error)
	DeleteListenerWithContext(ctx context.Context, input *elasticloadbalancingv2.DeleteListenerInput) (*elasticloadbalancingv2.DeleteListenerOutput, error)
	ModifyListenerWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyListenerInput) (*elasticloadbalancingv2.ModifyListenerOutput, error)
	ModifyLoadBalancerAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyLoadBalancerAttributesInput) (*elasticloadbalancingv2.ModifyLoadBalancerAttributesOutput, error)
	DescribeLoadBalancerAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeLoadBalancerAttributesInput) (*elasticloadbalancingv2.DescribeLoadBalancerAttributesOutput, error)
	CreateLoadBalancerWithContext(ctx context.Context, input *elasticloadbalancingv2.CreateLoadBalancerInput) (*elasticloadbalancingv2.CreateLoadBalancerOutput, error)
	DeleteLoadBalancerWithContext(ctx context.Context, input *elasticloadbalancingv2.DeleteLoadBalancerInput) (*elasticloadbalancingv2.DeleteLoadBalancerOutput, error)
	SetIpAddressTypeWithContext(ctx context.Context, input *elasticloadbalancingv2.SetIpAddressTypeInput) (*elasticloadbalancingv2.SetIpAddressTypeOutput, error)
	SetSubnetsWithContext(ctx context.Context, input *elasticloadbalancingv2.SetSubnetsInput) (*elasticloadbalancingv2.SetSubnetsOutput, error)
	SetSecurityGroupsWithContext(ctx context.Context, input *elasticloadbalancingv2.SetSecurityGroupsInput) (*elasticloadbalancingv2.SetSecurityGroupsOutput, error)
	ModifyTargetGroupAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyTargetGroupAttributesInput) (*elasticloadbalancingv2.ModifyTargetGroupAttributesOutput, error)
	DescribeTargetGroupAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTargetGroupAttributesInput) (*elasticloadbalancingv2.DescribeTargetGroupAttributesOutput, error)
	CreateTargetGroupWithContext(ctx context.Context, input *elasticloadbalancingv2.CreateTargetGroupInput) (*elasticloadbalancingv2.CreateTargetGroupOutput, error)
	ModifyTargetGroupWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyTargetGroupInput) (*elasticloadbalancingv2.ModifyTargetGroupOutput, error)
	DeleteTargetGroupWithContext(ctx context.Context, input *elasticloadbalancingv2.DeleteTargetGroupInput) (*elasticloadbalancingv2.DeleteTargetGroupOutput, error)
	DescribeTargetGroupsWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTargetGroupsInput) (*elasticloadbalancingv2.DescribeTargetGroupsOutput, error)
	DescribeTargetHealthWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTargetHealthInput) (*elasticloadbalancingv2.DescribeTargetHealthOutput, error)
	DescribeLoadBalancersWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeLoadBalancersInput) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error)
	WaitUntilLoadBalancerAvailableWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeLoadBalancersInput) error
	DescribeListenersWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeListenersInput) (*elasticloadbalancingv2.DescribeListenersOutput, error)
	DescribeRulesWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeRulesInput) (*elasticloadbalancingv2.DescribeRulesOutput, error)
	CreateRuleWithContext(ctx context.Context, input *elasticloadbalancingv2.CreateRuleInput) (*elasticloadbalancingv2.CreateRuleOutput, error)
	DeleteRuleWithContext(ctx context.Context, input *elasticloadbalancingv2.DeleteRuleInput) (*elasticloadbalancingv2.DeleteRuleOutput, error)
	ModifyRuleWithContext(ctx context.Context, inout *elasticloadbalancingv2.ModifyRuleInput) (*elasticloadbalancingv2.ModifyRuleOutput, error)
	RegisterTargetsWithContext(ctx context.Context, input *elasticloadbalancingv2.RegisterTargetsInput) (*elasticloadbalancingv2.RegisterTargetsOutput, error)
	DeregisterTargetsWithContext(ctx context.Context, input *elasticloadbalancingv2.DeregisterTargetsInput) (*elasticloadbalancingv2.DeregisterTargetsOutput, error)
	DescribeTrustStoresWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTrustStoresInput) (*elasticloadbalancingv2.DescribeTrustStoresOutput, error)
	RemoveListenerCertificatesWithContext(ctx context.Context, input *elasticloadbalancingv2.RemoveListenerCertificatesInput) (*elasticloadbalancingv2.RemoveListenerCertificatesOutput, error)
	AddListenerCertificatesWithContext(ctx context.Context, input *elasticloadbalancingv2.AddListenerCertificatesInput) (*elasticloadbalancingv2.AddListenerCertificatesOutput, error)
	DescribeListenerAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeListenerAttributesInput) (*elasticloadbalancingv2.DescribeListenerAttributesOutput, error)
	ModifyListenerAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyListenerAttributesInput) (*elasticloadbalancingv2.ModifyListenerAttributesOutput, error)
}

func NewELBV2(cfg aws.Config, endpointsResolver *endpoints.Resolver) ELBV2 {
	customEndpoint := endpointsResolver.EndpointFor(elasticloadbalancingv2.ServiceID)
	client := elasticloadbalancingv2.NewFromConfig(cfg, func(o *elasticloadbalancingv2.Options) {
		if customEndpoint != nil {
			o.BaseEndpoint = customEndpoint
		}
	})
	return &elbv2Client{elbv2Client: client}
}

// default implementation for ELBV2.
type elbv2Client struct {
	elbv2Client *elasticloadbalancingv2.Client
}

func (c *elbv2Client) AddListenerCertificatesWithContext(ctx context.Context, input *elasticloadbalancingv2.AddListenerCertificatesInput) (*elasticloadbalancingv2.AddListenerCertificatesOutput, error) {
	return c.elbv2Client.AddListenerCertificates(ctx, input)
}

func (c *elbv2Client) RemoveListenerCertificatesWithContext(ctx context.Context, input *elasticloadbalancingv2.RemoveListenerCertificatesInput) (*elasticloadbalancingv2.RemoveListenerCertificatesOutput, error) {
	return c.elbv2Client.RemoveListenerCertificates(ctx, input)
}

func (c *elbv2Client) DescribeListenersWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeListenersInput) (*elasticloadbalancingv2.DescribeListenersOutput, error) {
	return c.elbv2Client.DescribeListeners(ctx, input)
}

func (c *elbv2Client) DescribeRulesWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeRulesInput) (*elasticloadbalancingv2.DescribeRulesOutput, error) {
	return c.elbv2Client.DescribeRules(ctx, input)
}

func (c *elbv2Client) RegisterTargetsWithContext(ctx context.Context, input *elasticloadbalancingv2.RegisterTargetsInput) (*elasticloadbalancingv2.RegisterTargetsOutput, error) {
	return c.elbv2Client.RegisterTargets(ctx, input)
}

func (c *elbv2Client) DeregisterTargetsWithContext(ctx context.Context, input *elasticloadbalancingv2.DeregisterTargetsInput) (*elasticloadbalancingv2.DeregisterTargetsOutput, error) {
	return c.elbv2Client.DeregisterTargets(ctx, input)
}

func (c *elbv2Client) DescribeTrustStoresWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTrustStoresInput) (*elasticloadbalancingv2.DescribeTrustStoresOutput, error) {
	return c.elbv2Client.DescribeTrustStores(ctx, input)
}

func (c *elbv2Client) ModifyRuleWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyRuleInput) (*elasticloadbalancingv2.ModifyRuleOutput, error) {
	return c.elbv2Client.ModifyRule(ctx, input)
}

func (c *elbv2Client) DeleteRuleWithContext(ctx context.Context, input *elasticloadbalancingv2.DeleteRuleInput) (*elasticloadbalancingv2.DeleteRuleOutput, error) {
	return c.elbv2Client.DeleteRule(ctx, input)
}

func (c *elbv2Client) CreateRuleWithContext(ctx context.Context, input *elasticloadbalancingv2.CreateRuleInput) (*elasticloadbalancingv2.CreateRuleOutput, error) {
	return c.elbv2Client.CreateRule(ctx, input)
}

func (c *elbv2Client) WaitUntilLoadBalancerAvailableWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeLoadBalancersInput) error {
	waiter := elasticloadbalancingv2.NewLoadBalancerAvailableWaiter(c.elbv2Client)
	err := waiter.Wait(ctx, input, 5*time.Minute)
	return err
}

func (c *elbv2Client) DescribeLoadBalancersWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeLoadBalancersInput) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
	return c.elbv2Client.DescribeLoadBalancers(ctx, input)
}

func (c *elbv2Client) DescribeTargetHealthWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTargetHealthInput) (*elasticloadbalancingv2.DescribeTargetHealthOutput, error) {
	return c.elbv2Client.DescribeTargetHealth(ctx, input)
}

func (c *elbv2Client) DescribeTargetGroupsWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTargetGroupsInput) (*elasticloadbalancingv2.DescribeTargetGroupsOutput, error) {
	return c.elbv2Client.DescribeTargetGroups(ctx, input)
}

func (c *elbv2Client) DeleteTargetGroupWithContext(ctx context.Context, input *elasticloadbalancingv2.DeleteTargetGroupInput) (*elasticloadbalancingv2.DeleteTargetGroupOutput, error) {
	return c.elbv2Client.DeleteTargetGroup(ctx, input)
}

func (c *elbv2Client) ModifyTargetGroupWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyTargetGroupInput) (*elasticloadbalancingv2.ModifyTargetGroupOutput, error) {
	return c.elbv2Client.ModifyTargetGroup(ctx, input)
}

func (c *elbv2Client) CreateTargetGroupWithContext(ctx context.Context, input *elasticloadbalancingv2.CreateTargetGroupInput) (*elasticloadbalancingv2.CreateTargetGroupOutput, error) {
	return c.elbv2Client.CreateTargetGroup(ctx, input)
}

func (c *elbv2Client) DescribeTargetGroupAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTargetGroupAttributesInput) (*elasticloadbalancingv2.DescribeTargetGroupAttributesOutput, error) {
	return c.elbv2Client.DescribeTargetGroupAttributes(ctx, input)
}

func (c *elbv2Client) ModifyTargetGroupAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyTargetGroupAttributesInput) (*elasticloadbalancingv2.ModifyTargetGroupAttributesOutput, error) {
	return c.elbv2Client.ModifyTargetGroupAttributes(ctx, input)
}

func (c *elbv2Client) SetSecurityGroupsWithContext(ctx context.Context, input *elasticloadbalancingv2.SetSecurityGroupsInput) (*elasticloadbalancingv2.SetSecurityGroupsOutput, error) {
	return c.elbv2Client.SetSecurityGroups(ctx, input)
}

func (c *elbv2Client) SetSubnetsWithContext(ctx context.Context, input *elasticloadbalancingv2.SetSubnetsInput) (*elasticloadbalancingv2.SetSubnetsOutput, error) {
	return c.elbv2Client.SetSubnets(ctx, input)
}

func (c *elbv2Client) SetIpAddressTypeWithContext(ctx context.Context, input *elasticloadbalancingv2.SetIpAddressTypeInput) (*elasticloadbalancingv2.SetIpAddressTypeOutput, error) {
	return c.elbv2Client.SetIpAddressType(ctx, input)
}

func (c *elbv2Client) DeleteLoadBalancerWithContext(ctx context.Context, input *elasticloadbalancingv2.DeleteLoadBalancerInput) (*elasticloadbalancingv2.DeleteLoadBalancerOutput, error) {
	return c.elbv2Client.DeleteLoadBalancer(ctx, input)
}

func (c *elbv2Client) CreateLoadBalancerWithContext(ctx context.Context, input *elasticloadbalancingv2.CreateLoadBalancerInput) (*elasticloadbalancingv2.CreateLoadBalancerOutput, error) {
	return c.elbv2Client.CreateLoadBalancer(ctx, input)
}

func (c *elbv2Client) DescribeLoadBalancerAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeLoadBalancerAttributesInput) (*elasticloadbalancingv2.DescribeLoadBalancerAttributesOutput, error) {
	return c.elbv2Client.DescribeLoadBalancerAttributes(ctx, input)
}

func (c *elbv2Client) ModifyLoadBalancerAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyLoadBalancerAttributesInput) (*elasticloadbalancingv2.ModifyLoadBalancerAttributesOutput, error) {
	return c.elbv2Client.ModifyLoadBalancerAttributes(ctx, input)
}

func (c *elbv2Client) ModifyListenerWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyListenerInput) (*elasticloadbalancingv2.ModifyListenerOutput, error) {
	return c.elbv2Client.ModifyListener(ctx, input)
}

func (c *elbv2Client) DeleteListenerWithContext(ctx context.Context, input *elasticloadbalancingv2.DeleteListenerInput) (*elasticloadbalancingv2.DeleteListenerOutput, error) {
	return c.elbv2Client.DeleteListener(ctx, input)
}

func (c *elbv2Client) CreateListenerWithContext(ctx context.Context, input *elasticloadbalancingv2.CreateListenerInput) (*elasticloadbalancingv2.CreateListenerOutput, error) {
	return c.elbv2Client.CreateListener(ctx, input)
}

func (c *elbv2Client) DescribeTagsWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTagsInput) (*elasticloadbalancingv2.DescribeTagsOutput, error) {
	return c.elbv2Client.DescribeTags(ctx, input)
}

func (c *elbv2Client) AddTagsWithContext(ctx context.Context, input *elasticloadbalancingv2.AddTagsInput) (*elasticloadbalancingv2.AddTagsOutput, error) {
	return c.elbv2Client.AddTags(ctx, input)
}

func (c *elbv2Client) RemoveTagsWithContext(ctx context.Context, input *elasticloadbalancingv2.RemoveTagsInput) (*elasticloadbalancingv2.RemoveTagsOutput, error) {
	return c.elbv2Client.RemoveTags(ctx, input)
}

func (c *elbv2Client) DescribeLoadBalancersAsList(ctx context.Context, input *elasticloadbalancingv2.DescribeLoadBalancersInput) ([]types.LoadBalancer, error) {
	var result []types.LoadBalancer
	paginator := elasticloadbalancingv2.NewDescribeLoadBalancersPaginator(c.elbv2Client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.LoadBalancers...)
	}
	return result, nil
}

func (c *elbv2Client) DescribeTargetGroupsAsList(ctx context.Context, input *elasticloadbalancingv2.DescribeTargetGroupsInput) ([]types.TargetGroup, error) {
	var result []types.TargetGroup
	paginator := elasticloadbalancingv2.NewDescribeTargetGroupsPaginator(c.elbv2Client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.TargetGroups...)
	}
	return result, nil
}

func (c *elbv2Client) DescribeListenersAsList(ctx context.Context, input *elasticloadbalancingv2.DescribeListenersInput) ([]types.Listener, error) {
	var result []types.Listener
	paginator := elasticloadbalancingv2.NewDescribeListenersPaginator(c.elbv2Client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.Listeners...)
	}
	return result, nil
}

func (c *elbv2Client) DescribeListenerCertificatesAsList(ctx context.Context, input *elasticloadbalancingv2.DescribeListenerCertificatesInput) ([]types.Certificate, error) {
	var result []types.Certificate
	paginator := elasticloadbalancingv2.NewDescribeListenerCertificatesPaginator(c.elbv2Client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.Certificates...)
	}
	return result, nil
}

func (c *elbv2Client) DescribeRulesAsList(ctx context.Context, input *elasticloadbalancingv2.DescribeRulesInput) ([]types.Rule, error) {
	var result []types.Rule
	paginator := elasticloadbalancingv2.NewDescribeRulesPaginator(c.elbv2Client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.Rules...)
	}
	return result, nil
}

func (c *elbv2Client) DescribeListenerAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeListenerAttributesInput) (*elasticloadbalancingv2.DescribeListenerAttributesOutput, error) {
	return c.elbv2Client.DescribeListenerAttributes(ctx, input)
}

func (c *elbv2Client) ModifyListenerAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyListenerAttributesInput) (*elasticloadbalancingv2.ModifyListenerAttributesOutput, error) {
	return c.elbv2Client.ModifyListenerAttributes(ctx, input)
}
