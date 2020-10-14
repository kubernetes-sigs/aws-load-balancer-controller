package aws

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

// LoadBalancerManager is responsible for LoadBalancer resources.
type LoadBalancerManager interface {
	FindLoadBalancerByDNSName(ctx context.Context, dnsName string) (string, error)
	WaitUntilLoadBalancerAvailable(ctx context.Context, lbARN string) error
}

// NewDefaultLoadBalancerManager constructs new defaultLoadBalancerManager.
func NewDefaultLoadBalancerManager(elbv2Client services.ELBV2, logger logr.Logger) *defaultLoadBalancerManager {
	return &defaultLoadBalancerManager{
		elbv2Client: elbv2Client,
		logger:      logger,
	}
}

var _ LoadBalancerManager = &defaultLoadBalancerManager{}

// default implementation for LoadBalancerManager
type defaultLoadBalancerManager struct {
	elbv2Client services.ELBV2
	logger      logr.Logger
}

func (m *defaultLoadBalancerManager) FindLoadBalancerByDNSName(ctx context.Context, dnsName string) (string, error) {
	req := &elbv2sdk.DescribeLoadBalancersInput{}
	lbs, err := m.elbv2Client.DescribeLoadBalancersAsList(ctx, req)
	if err != nil {
		return "", err
	}
	for _, lb := range lbs {
		if awssdk.StringValue(lb.DNSName) == dnsName {
			return awssdk.StringValue(lb.LoadBalancerArn), nil
		}
	}
	return "", errors.Errorf("couldn't find LoadBalancer with dnsName: %v", dnsName)
}

func (m *defaultLoadBalancerManager) WaitUntilLoadBalancerAvailable(ctx context.Context, lbARN string) error {
	req := &elbv2sdk.DescribeLoadBalancersInput{
		LoadBalancerArns: awssdk.StringSlice([]string{lbARN}),
	}
	return m.elbv2Client.WaitUntilLoadBalancerAvailableWithContext(ctx, req)
}
