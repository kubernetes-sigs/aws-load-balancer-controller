package aws

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

// LoadBalancerManager is responsible for LoadBalancer resources.
type LoadBalancerManager interface {
	FindLoadBalancerByDNSName(ctx context.Context, dnsName string) (string, error)
	WaitUntilLoadBalancerAvailable(ctx context.Context, lbARN string) error
	GetLoadBalancerFromARN(ctx context.Context, lbARN string) (*elbv2types.LoadBalancer, error)
	GetLoadBalancerListeners(ctx context.Context, lbARN string) ([]elbv2types.Listener, error)
	GetLoadBalancerListenerCertificates(ctx context.Context, listenerARN string) ([]elbv2types.Certificate, error)
	GetLoadBalancerAttributes(ctx context.Context, lbARN string) ([]elbv2types.LoadBalancerAttribute, error)
	GetLoadBalancerResourceTags(ctx context.Context, resARN string) ([]elbv2types.Tag, error)
	GetLoadBalancerListenerRules(ctx context.Context, lsARN string) ([]elbv2types.Rule, error)
	GetListenerAttributes(ctx context.Context, lsARN string) ([]elbv2types.ListenerAttribute, error)
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
		if awssdk.ToString(lb.DNSName) == dnsName {
			return awssdk.ToString(lb.LoadBalancerArn), nil
		}
	}
	return "", errors.Errorf("couldn't find LoadBalancer with dnsName: %v", dnsName)
}

func (m *defaultLoadBalancerManager) WaitUntilLoadBalancerAvailable(ctx context.Context, lbARN string) error {
	req := &elbv2sdk.DescribeLoadBalancersInput{
		LoadBalancerArns: []string{lbARN},
	}
	return m.elbv2Client.WaitUntilLoadBalancerAvailableWithContext(ctx, req)
}

func (m *defaultLoadBalancerManager) GetLoadBalancerFromARN(ctx context.Context, lbARN string) (*elbv2types.LoadBalancer, error) {
	req := &elbv2sdk.DescribeLoadBalancersInput{
		LoadBalancerArns: []string{lbARN},
	}
	lbs, err := m.elbv2Client.DescribeLoadBalancersWithContext(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(lbs.LoadBalancers) == 0 {
		return nil, errors.Errorf("couldn't find LoadBalancer with ARN %v", lbARN)
	}
	return &lbs.LoadBalancers[0], nil
}

func (m *defaultLoadBalancerManager) GetLoadBalancerListeners(ctx context.Context, lbARN string) ([]elbv2types.Listener, error) {
	listeners, err := m.elbv2Client.DescribeListenersWithContext(ctx, &elbv2sdk.DescribeListenersInput{
		LoadBalancerArn: awssdk.String(lbARN),
	})
	if err != nil {
		return nil, err
	}
	return listeners.Listeners, nil
}

func (m *defaultLoadBalancerManager) GetLoadBalancerListenerCertificates(ctx context.Context, listenerARN string) ([]elbv2types.Certificate, error) {
	return m.elbv2Client.DescribeListenerCertificatesAsList(ctx, &elbv2sdk.DescribeListenerCertificatesInput{
		ListenerArn: awssdk.String(listenerARN),
	})
}

func (m *defaultLoadBalancerManager) GetLoadBalancerAttributes(ctx context.Context, lbARN string) ([]elbv2types.LoadBalancerAttribute, error) {
	resp, err := m.elbv2Client.DescribeLoadBalancerAttributesWithContext(ctx, &elbv2sdk.DescribeLoadBalancerAttributesInput{
		LoadBalancerArn: awssdk.String(lbARN),
	})
	if err != nil {
		return nil, err
	}
	return resp.Attributes, nil
}

func (m *defaultLoadBalancerManager) GetLoadBalancerResourceTags(ctx context.Context, resARN string) ([]elbv2types.Tag, error) {
	resp, err := m.elbv2Client.DescribeTagsWithContext(ctx, &elbv2sdk.DescribeTagsInput{
		ResourceArns: []string{resARN},
	})
	if err != nil {
		return nil, err
	}
	return resp.TagDescriptions[0].Tags, nil
}

func (m *defaultLoadBalancerManager) GetLoadBalancerListenerRules(ctx context.Context, lsARN string) ([]elbv2types.Rule, error) {
	listenersRules, err := m.elbv2Client.DescribeRulesWithContext(ctx, &elbv2sdk.DescribeRulesInput{
		ListenerArn: awssdk.String(lsARN),
	})
	if err != nil {
		return nil, err
	}
	return listenersRules.Rules, nil
}

func (m *defaultLoadBalancerManager) GetListenerAttributes(ctx context.Context, lsARN string) ([]elbv2types.ListenerAttribute, error) {
	resp, err := m.elbv2Client.DescribeListenerAttributesWithContext(ctx, &elbv2sdk.DescribeListenerAttributesInput{
		ListenerArn: awssdk.String(lsARN),
	})
	if err != nil {
		return nil, err
	}
	return resp.Attributes, nil
}
