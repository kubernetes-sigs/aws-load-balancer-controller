package services

import (
	"context"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
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
	ModifyCapacityReservationWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyCapacityReservationInput) (*elasticloadbalancingv2.ModifyCapacityReservationOutput, error)
	DescribeCapacityReservationWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeCapacityReservationInput) (*elasticloadbalancingv2.DescribeCapacityReservationOutput, error)
}

func NewELBV2(awsClientsProvider provider.AWSClientsProvider) ELBV2 {
	return &elbv2Client{
		awsClientsProvider: awsClientsProvider,
	}
}

// default implementation for ELBV2.
type elbv2Client struct {
	awsClientsProvider provider.AWSClientsProvider
}

func (c *elbv2Client) AddListenerCertificatesWithContext(ctx context.Context, input *elasticloadbalancingv2.AddListenerCertificatesInput) (*elasticloadbalancingv2.AddListenerCertificatesOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "AddListenerCertificates")
	if err != nil {
		return nil, err
	}
	return client.AddListenerCertificates(ctx, input)
}

func (c *elbv2Client) RemoveListenerCertificatesWithContext(ctx context.Context, input *elasticloadbalancingv2.RemoveListenerCertificatesInput) (*elasticloadbalancingv2.RemoveListenerCertificatesOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "RemoveListenerCertificates")
	if err != nil {
		return nil, err
	}
	return client.RemoveListenerCertificates(ctx, input)
}

func (c *elbv2Client) DescribeListenersWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeListenersInput) (*elasticloadbalancingv2.DescribeListenersOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DescribeListeners")
	if err != nil {
		return nil, err
	}
	return client.DescribeListeners(ctx, input)
}

func (c *elbv2Client) DescribeRulesWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeRulesInput) (*elasticloadbalancingv2.DescribeRulesOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DescribeRules")
	if err != nil {
		return nil, err
	}
	return client.DescribeRules(ctx, input)
}

func (c *elbv2Client) RegisterTargetsWithContext(ctx context.Context, input *elasticloadbalancingv2.RegisterTargetsInput) (*elasticloadbalancingv2.RegisterTargetsOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "RegisterTargets")
	if err != nil {
		return nil, err
	}
	return client.RegisterTargets(ctx, input)
}

func (c *elbv2Client) DeregisterTargetsWithContext(ctx context.Context, input *elasticloadbalancingv2.DeregisterTargetsInput) (*elasticloadbalancingv2.DeregisterTargetsOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DeregisterTargets")
	if err != nil {
		return nil, err
	}
	return client.DeregisterTargets(ctx, input)
}

func (c *elbv2Client) DescribeTrustStoresWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTrustStoresInput) (*elasticloadbalancingv2.DescribeTrustStoresOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DescribeTrustStores")
	if err != nil {
		return nil, err
	}
	return client.DescribeTrustStores(ctx, input)
}

func (c *elbv2Client) ModifyRuleWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyRuleInput) (*elasticloadbalancingv2.ModifyRuleOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "ModifyRule")
	if err != nil {
		return nil, err
	}
	return client.ModifyRule(ctx, input)
}

func (c *elbv2Client) DeleteRuleWithContext(ctx context.Context, input *elasticloadbalancingv2.DeleteRuleInput) (*elasticloadbalancingv2.DeleteRuleOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DeleteRule")
	if err != nil {
		return nil, err
	}
	return client.DeleteRule(ctx, input)
}

func (c *elbv2Client) CreateRuleWithContext(ctx context.Context, input *elasticloadbalancingv2.CreateRuleInput) (*elasticloadbalancingv2.CreateRuleOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "CreateRule")
	if err != nil {
		return nil, err
	}
	return client.CreateRule(ctx, input)
}

func (c *elbv2Client) WaitUntilLoadBalancerAvailableWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeLoadBalancersInput) error {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DescribeLoadBalancers")
	if err != nil {
		return err
	}
	waiter := elasticloadbalancingv2.NewLoadBalancerAvailableWaiter(client)
	err = waiter.Wait(ctx, input, 5*time.Minute)
	return err
}

func (c *elbv2Client) DescribeLoadBalancersWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeLoadBalancersInput) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DescribeLoadBalancers")
	if err != nil {
		return nil, err
	}
	return client.DescribeLoadBalancers(ctx, input)
}

func (c *elbv2Client) DescribeTargetHealthWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTargetHealthInput) (*elasticloadbalancingv2.DescribeTargetHealthOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DescribeTargetHealth")
	if err != nil {
		return nil, err
	}
	return client.DescribeTargetHealth(ctx, input)
}

func (c *elbv2Client) DescribeTargetGroupsWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTargetGroupsInput) (*elasticloadbalancingv2.DescribeTargetGroupsOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DescribeTargetGroups")
	if err != nil {
		return nil, err
	}
	return client.DescribeTargetGroups(ctx, input)
}

func (c *elbv2Client) DeleteTargetGroupWithContext(ctx context.Context, input *elasticloadbalancingv2.DeleteTargetGroupInput) (*elasticloadbalancingv2.DeleteTargetGroupOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DeleteTargetGroup")
	if err != nil {
		return nil, err
	}
	return client.DeleteTargetGroup(ctx, input)
}

func (c *elbv2Client) ModifyTargetGroupWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyTargetGroupInput) (*elasticloadbalancingv2.ModifyTargetGroupOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "ModifyTargetGroup")
	if err != nil {
		return nil, err
	}
	return client.ModifyTargetGroup(ctx, input)
}

func (c *elbv2Client) CreateTargetGroupWithContext(ctx context.Context, input *elasticloadbalancingv2.CreateTargetGroupInput) (*elasticloadbalancingv2.CreateTargetGroupOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "CreateTargetGroup")
	if err != nil {
		return nil, err
	}
	return client.CreateTargetGroup(ctx, input)
}

func (c *elbv2Client) DescribeTargetGroupAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTargetGroupAttributesInput) (*elasticloadbalancingv2.DescribeTargetGroupAttributesOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DescribeTargetGroupAttributes")
	if err != nil {
		return nil, err
	}
	return client.DescribeTargetGroupAttributes(ctx, input)
}

func (c *elbv2Client) ModifyTargetGroupAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyTargetGroupAttributesInput) (*elasticloadbalancingv2.ModifyTargetGroupAttributesOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "ModifyTargetGroupAttributes")
	if err != nil {
		return nil, err
	}
	return client.ModifyTargetGroupAttributes(ctx, input)
}

func (c *elbv2Client) SetSecurityGroupsWithContext(ctx context.Context, input *elasticloadbalancingv2.SetSecurityGroupsInput) (*elasticloadbalancingv2.SetSecurityGroupsOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "SetSecurityGroups")
	if err != nil {
		return nil, err
	}
	return client.SetSecurityGroups(ctx, input)
}

func (c *elbv2Client) SetSubnetsWithContext(ctx context.Context, input *elasticloadbalancingv2.SetSubnetsInput) (*elasticloadbalancingv2.SetSubnetsOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "SetSubnets")
	if err != nil {
		return nil, err
	}
	return client.SetSubnets(ctx, input)
}

func (c *elbv2Client) SetIpAddressTypeWithContext(ctx context.Context, input *elasticloadbalancingv2.SetIpAddressTypeInput) (*elasticloadbalancingv2.SetIpAddressTypeOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "SetIpAddressType")
	if err != nil {
		return nil, err
	}
	return client.SetIpAddressType(ctx, input)
}

func (c *elbv2Client) DeleteLoadBalancerWithContext(ctx context.Context, input *elasticloadbalancingv2.DeleteLoadBalancerInput) (*elasticloadbalancingv2.DeleteLoadBalancerOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DeleteLoadBalancer")
	if err != nil {
		return nil, err
	}
	return client.DeleteLoadBalancer(ctx, input)
}

func (c *elbv2Client) CreateLoadBalancerWithContext(ctx context.Context, input *elasticloadbalancingv2.CreateLoadBalancerInput) (*elasticloadbalancingv2.CreateLoadBalancerOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "CreateLoadBalancer")
	if err != nil {
		return nil, err
	}
	return client.CreateLoadBalancer(ctx, input)
}

func (c *elbv2Client) DescribeLoadBalancerAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeLoadBalancerAttributesInput) (*elasticloadbalancingv2.DescribeLoadBalancerAttributesOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DescribeLoadBalancerAttributes")
	if err != nil {
		return nil, err
	}
	return client.DescribeLoadBalancerAttributes(ctx, input)
}

func (c *elbv2Client) ModifyLoadBalancerAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyLoadBalancerAttributesInput) (*elasticloadbalancingv2.ModifyLoadBalancerAttributesOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "ModifyLoadBalancerAttributes")
	if err != nil {
		return nil, err
	}
	return client.ModifyLoadBalancerAttributes(ctx, input)
}

func (c *elbv2Client) ModifyListenerWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyListenerInput) (*elasticloadbalancingv2.ModifyListenerOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "ModifyListener")
	if err != nil {
		return nil, err
	}
	return client.ModifyListener(ctx, input)
}

func (c *elbv2Client) DeleteListenerWithContext(ctx context.Context, input *elasticloadbalancingv2.DeleteListenerInput) (*elasticloadbalancingv2.DeleteListenerOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DeleteListener")
	if err != nil {
		return nil, err
	}
	return client.DeleteListener(ctx, input)
}

func (c *elbv2Client) CreateListenerWithContext(ctx context.Context, input *elasticloadbalancingv2.CreateListenerInput) (*elasticloadbalancingv2.CreateListenerOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "CreateListener")
	if err != nil {
		return nil, err
	}
	return client.CreateListener(ctx, input)
}

func (c *elbv2Client) DescribeTagsWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeTagsInput) (*elasticloadbalancingv2.DescribeTagsOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DescribeTags")
	if err != nil {
		return nil, err
	}
	return client.DescribeTags(ctx, input)
}

func (c *elbv2Client) AddTagsWithContext(ctx context.Context, input *elasticloadbalancingv2.AddTagsInput) (*elasticloadbalancingv2.AddTagsOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "AddTags")
	if err != nil {
		return nil, err
	}
	return client.AddTags(ctx, input)
}

func (c *elbv2Client) RemoveTagsWithContext(ctx context.Context, input *elasticloadbalancingv2.RemoveTagsInput) (*elasticloadbalancingv2.RemoveTagsOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "RemoveTags")
	if err != nil {
		return nil, err
	}
	return client.RemoveTags(ctx, input)
}

func (c *elbv2Client) DescribeLoadBalancersAsList(ctx context.Context, input *elasticloadbalancingv2.DescribeLoadBalancersInput) ([]types.LoadBalancer, error) {
	var result []types.LoadBalancer
	var client *elasticloadbalancingv2.Client
	var err error
	client, err = c.awsClientsProvider.GetELBv2Client(ctx, "DescribeLoadBalancers")
	if err != nil {
		return nil, err
	}
	paginator := elasticloadbalancingv2.NewDescribeLoadBalancersPaginator(client, input)
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
	var client *elasticloadbalancingv2.Client
	var err error
	client, err = c.awsClientsProvider.GetELBv2Client(ctx, "DescribeTargetGroups")
	if err != nil {
		return nil, err
	}
	paginator := elasticloadbalancingv2.NewDescribeTargetGroupsPaginator(client, input)
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
	var client *elasticloadbalancingv2.Client
	var err error
	client, err = c.awsClientsProvider.GetELBv2Client(ctx, "DescribeListeners")
	if err != nil {
		return nil, err
	}
	paginator := elasticloadbalancingv2.NewDescribeListenersPaginator(client, input)
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
	var client *elasticloadbalancingv2.Client
	var err error
	client, err = c.awsClientsProvider.GetELBv2Client(ctx, "DescribeListenerCertificates")
	if err != nil {
		return nil, err
	}
	paginator := elasticloadbalancingv2.NewDescribeListenerCertificatesPaginator(client, input)
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
	var client *elasticloadbalancingv2.Client
	var err error
	client, err = c.awsClientsProvider.GetELBv2Client(ctx, "DescribeRules")
	if err != nil {
		return nil, err
	}
	paginator := elasticloadbalancingv2.NewDescribeRulesPaginator(client, input)
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
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DescribeListenerAttributes")
	if err != nil {
		return nil, err
	}
	return client.DescribeListenerAttributes(ctx, input)
}

func (c *elbv2Client) ModifyListenerAttributesWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyListenerAttributesInput) (*elasticloadbalancingv2.ModifyListenerAttributesOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "ModifyListenerAttributes")
	if err != nil {
		return nil, err
	}
	return client.ModifyListenerAttributes(ctx, input)
}

func (c *elbv2Client) ModifyCapacityReservationWithContext(ctx context.Context, input *elasticloadbalancingv2.ModifyCapacityReservationInput) (*elasticloadbalancingv2.ModifyCapacityReservationOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "ModifyCapacityReservation")
	if err != nil {
		return nil, err
	}
	return client.ModifyCapacityReservation(ctx, input)
}

func (c *elbv2Client) DescribeCapacityReservationWithContext(ctx context.Context, input *elasticloadbalancingv2.DescribeCapacityReservationInput) (*elasticloadbalancingv2.DescribeCapacityReservationOutput, error) {
	client, err := c.awsClientsProvider.GetELBv2Client(ctx, "DescribeCapacityReservation")
	if err != nil {
		return nil, err
	}
	return client.DescribeCapacityReservation(ctx, input)
}
