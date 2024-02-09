package aws

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

// TargetGroupManager is responsible for TargetGroup resources.
type TargetGroupManager interface {
	GetTargetGroupsForLoadBalancer(ctx context.Context, lbARN string) ([]*elbv2sdk.TargetGroup, error)
	CheckTargetGroupHealthy(ctx context.Context, tgARN string, expectedTargetCount int) (bool, error)
	GetCurrentTargetCount(ctx context.Context, tgARN string) (int, error)
	GetTargetGroupAttributes(ctx context.Context, tgARN string) ([]*elbv2sdk.TargetGroupAttribute, error)
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
func (m *defaultTargetGroupManager) GetTargetGroupsForLoadBalancer(ctx context.Context, lbARN string) ([]*elbv2sdk.TargetGroup, error) {
	targetGroups, err := m.elbv2Client.DescribeTargetGroupsWithContext(ctx, &elbv2sdk.DescribeTargetGroupsInput{
		LoadBalancerArn: awssdk.String(lbARN),
	})
	if err != nil {
		return nil, err
	}
	return targetGroups.TargetGroups, nil
}

// GetCurrentTargetCount returns the count of all the targets in the target group that are currently in initial, healthy or unhealthy state
func (m *defaultTargetGroupManager) GetCurrentTargetCount(ctx context.Context, tgARN string) (int, error) {
	resp, err := m.elbv2Client.DescribeTargetHealthWithContext(ctx, &elbv2sdk.DescribeTargetHealthInput{
		TargetGroupArn: awssdk.String(tgARN),
	})
	if err != nil {
		return 0, err
	}
	count := 0
	for _, thd := range resp.TargetHealthDescriptions {
		state := awssdk.StringValue(thd.TargetHealth.State)
		if state == elbv2sdk.TargetHealthStateEnumHealthy || state == elbv2sdk.TargetHealthStateEnumInitial ||
			state == elbv2sdk.TargetHealthStateEnumUnhealthy {
			count++
		}
	}
	return count, nil
}

// GetTargetGroupAttributes returns the targetgroup attributes for the given target group
func (m *defaultTargetGroupManager) GetTargetGroupAttributes(ctx context.Context, tgARN string) ([]*elbv2sdk.TargetGroupAttribute, error) {
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
		if awssdk.StringValue(thd.TargetHealth.State) != elbv2sdk.TargetHealthStateEnumHealthy {
			return false, nil
		}
	}
	return true, nil
}
