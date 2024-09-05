package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	shieldsdk "github.com/aws/aws-sdk-go-v2/service/shield"
)

type Shield interface {
	CreateProtectionWithContext(ctx context.Context, input *shieldsdk.CreateProtectionInput) (*shieldsdk.CreateProtectionOutput, error)
	DeleteProtectionWithContext(ctx context.Context, input *shieldsdk.DeleteProtectionInput) (*shieldsdk.DeleteProtectionOutput, error)
	DescribeProtectionWithContext(ctx context.Context, input *shieldsdk.DescribeProtectionInput) (*shieldsdk.DescribeProtectionOutput, error)
	GetSubscriptionStateWithContext(ctx context.Context, input *shieldsdk.GetSubscriptionStateInput) (*shieldsdk.GetSubscriptionStateOutput, error)
}

// NewShield constructs new Shield implementation.
func NewShield(cfg aws.Config) Shield {
	// shield is only available as a global API in us-east-1.
	client := shieldsdk.NewFromConfig(cfg, func(o *shieldsdk.Options) {
		o.Region = "us-east-1"
	})
	return &shieldClient{shieldClient: client}
}

// default implementation for Shield.
type shieldClient struct {
	shieldClient *shieldsdk.Client
}

func (s *shieldClient) GetSubscriptionStateWithContext(ctx context.Context, input *shieldsdk.GetSubscriptionStateInput) (*shieldsdk.GetSubscriptionStateOutput, error) {
	return s.shieldClient.GetSubscriptionState(ctx, input)
}

func (s *shieldClient) DescribeProtectionWithContext(ctx context.Context, input *shieldsdk.DescribeProtectionInput) (*shieldsdk.DescribeProtectionOutput, error) {
	return s.shieldClient.DescribeProtection(ctx, input)
}

func (s *shieldClient) CreateProtectionWithContext(ctx context.Context, input *shieldsdk.CreateProtectionInput) (*shieldsdk.CreateProtectionOutput, error) {
	return s.shieldClient.CreateProtection(ctx, input)
}

func (s *shieldClient) DeleteProtectionWithContext(ctx context.Context, input *shieldsdk.DeleteProtectionInput) (*shieldsdk.DeleteProtectionOutput, error) {
	return s.shieldClient.DeleteProtection(ctx, input)
}
