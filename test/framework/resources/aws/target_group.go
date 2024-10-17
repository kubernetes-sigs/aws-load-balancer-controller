package aws

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

// TargetGroupManager is responsible for TargetGroup resources.
type TargetGroupManager interface {
	GetTargetGroupsForLoadBalancer(ctx context.Context, lbARN string) ([]elbv2types.TargetGroup, error)
	CheckTargetGroupHealthy(ctx context.Context, tgARN string, expectedTargetCount int) (bool, error)
	GetCurrentTargetCount(ctx context.Context, tgARN string) (int, error)
	GetTargetGroupAttributes(ctx context.Context, tgARN string) ([]elbv2types.TargetGroupAttribute, error)
	GetCurrentTargets(ctx context.Context, tgARN string) ([]elbv2types.TargetHealthDescription, error)
	RegisterTargets(ctx context.Context, tgARN string, targets []elbv2types.TargetDescription) error
}

// NewDefaultTargetGroupManager constructs new defaultTargetGroupManager.
func NewDefaultTargetGroupManager(elbv2Client services.ELBV2, logger logr.Logger) *defaultTargetGroupManager {
	return &defaultTargetGroupManager{
		elbv2Client: elbv2Client,
		logger:      logger,
	}
}

var _ TargetGroupManager = &defaultTargetGroupManager{}

// default implementation for TargetGroupManager
type defaultTargetGroupManager struct {
	elbv2Client services.ELBV2
	logger      logr.Logger
}

// GetTargetGroupsForLoadBalancer returns all targetgroups configured for the load balancer
func (m *defaultTargetGroupManager) GetTargetGroupsForLoadBalancer(ctx context.Context, lbARN string) ([]elbv2types.TargetGroup, error) {
	targetGroups, err := m.elbv2Client.DescribeTargetGroupsWithContext(ctx, &elbv2sdk.DescribeTargetGroupsInput{
		LoadBalancerArn: awssdk.String(lbARN),
	})
	if err != nil {
		return nil, err
	}
	return targetGroups.TargetGroups, nil
}

func (m *defaultTargetGroupManager) GetCurrentTargets(ctx context.Context, tgARN string) ([]elbv2types.TargetHealthDescription, error) {
	resp, err := m.elbv2Client.DescribeTargetHealthWithContext(ctx, &elbv2sdk.DescribeTargetHealthInput{
		TargetGroupArn: awssdk.String(tgARN),
	})
	if err != nil {
		return nil, err
	}
	return resp.TargetHealthDescriptions, nil
}

// GetCurrentTargetCount returns the count of all the targets in the target group that are currently in initial, healthy or unhealthy state
func (m *defaultTargetGroupManager) GetCurrentTargetCount(ctx context.Context, tgARN string) (int, error) {
	targets, err := m.GetCurrentTargets(ctx, tgARN)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, thd := range targets {
		state := string(thd.TargetHealth.State)
		if elbv2types.TargetHealthStateEnum(state) == elbv2types.TargetHealthStateEnumHealthy || elbv2types.TargetHealthStateEnum(state) == elbv2types.TargetHealthStateEnumInitial ||
			elbv2types.TargetHealthStateEnum(state) == elbv2types.TargetHealthStateEnumUnhealthy {
			count++
		}
	}
	return count, nil
}

// GetTargetGroupAttributes returns the targetgroup attributes for the given target group
func (m *defaultTargetGroupManager) GetTargetGroupAttributes(ctx context.Context, tgARN string) ([]elbv2types.TargetGroupAttribute, error) {
	resp, err := m.elbv2Client.DescribeTargetGroupAttributesWithContext(ctx, &elbv2sdk.DescribeTargetGroupAttributesInput{
		TargetGroupArn: awssdk.String(tgARN),
	})
	if err != nil {
		return nil, err
	}
	return resp.Attributes, nil
}

// CheckTargetGroupHealthy returns true only if all of the targets in the target group are in healthy state
func (m *defaultTargetGroupManager) CheckTargetGroupHealthy(ctx context.Context, tgARN string, expectedTargetCount int) (bool, error) {
	resp, err := m.elbv2Client.DescribeTargetHealthWithContext(ctx, &elbv2sdk.DescribeTargetHealthInput{
		TargetGroupArn: awssdk.String(tgARN),
	})
	if err != nil {
		return false, err
	}
	if len(resp.TargetHealthDescriptions) != expectedTargetCount {
		return false, nil
	}
	for _, thd := range resp.TargetHealthDescriptions {
		if thd.TargetHealth.State != elbv2types.TargetHealthStateEnumHealthy {
			return false, nil
		}
	}
	return true, nil
}

// RegisterTargets register targets to the target group.
func (m *defaultTargetGroupManager) RegisterTargets(ctx context.Context, tgARN string, targets []elbv2types.TargetDescription) error {
	_, err := m.elbv2Client.RegisterTargetsWithContext(ctx, &elbv2sdk.RegisterTargetsInput{
		TargetGroupArn: awssdk.String(tgARN),
		Targets:        targets,
	})

	return err
}
