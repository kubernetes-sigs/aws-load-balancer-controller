package services

import (
	"context"
	shieldsdk "github.com/aws/aws-sdk-go-v2/service/shield"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
)

type Shield interface {
	CreateProtectionWithContext(ctx context.Context, input *shieldsdk.CreateProtectionInput) (*shieldsdk.CreateProtectionOutput, error)
	DeleteProtectionWithContext(ctx context.Context, input *shieldsdk.DeleteProtectionInput) (*shieldsdk.DeleteProtectionOutput, error)
	DescribeProtectionWithContext(ctx context.Context, input *shieldsdk.DescribeProtectionInput) (*shieldsdk.DescribeProtectionOutput, error)
	GetSubscriptionStateWithContext(ctx context.Context, input *shieldsdk.GetSubscriptionStateInput) (*shieldsdk.GetSubscriptionStateOutput, error)
}

// NewShield constructs new Shield implementation.
func NewShield(awsClientsProvider provider.AWSClientsProvider) Shield {
	return &shieldClient{
		awsClientsProvider: awsClientsProvider,
	}
}

// default implementation for Shield.
type shieldClient struct {
	awsClientsProvider provider.AWSClientsProvider
}

func (s *shieldClient) GetSubscriptionStateWithContext(ctx context.Context, input *shieldsdk.GetSubscriptionStateInput) (*shieldsdk.GetSubscriptionStateOutput, error) {
	client, err := s.awsClientsProvider.GetShieldClient(ctx, "GetSubscriptionState")
	if err != nil {
		return nil, err
	}
	return client.GetSubscriptionState(ctx, input)
}

func (s *shieldClient) DescribeProtectionWithContext(ctx context.Context, input *shieldsdk.DescribeProtectionInput) (*shieldsdk.DescribeProtectionOutput, error) {
	client, err := s.awsClientsProvider.GetShieldClient(ctx, "DescribeProtection")
	if err != nil {
		return nil, err
	}
	return client.DescribeProtection(ctx, input)
}

func (s *shieldClient) CreateProtectionWithContext(ctx context.Context, input *shieldsdk.CreateProtectionInput) (*shieldsdk.CreateProtectionOutput, error) {
	client, err := s.awsClientsProvider.GetShieldClient(ctx, "CreateProtection")
	if err != nil {
		return nil, err
	}
	return client.CreateProtection(ctx, input)
}

func (s *shieldClient) DeleteProtectionWithContext(ctx context.Context, input *shieldsdk.DeleteProtectionInput) (*shieldsdk.DeleteProtectionOutput, error) {
	client, err := s.awsClientsProvider.GetShieldClient(ctx, "DeleteProtection")
	if err != nil {
		return nil, err
	}
	return client.DeleteProtection(ctx, input)
}
