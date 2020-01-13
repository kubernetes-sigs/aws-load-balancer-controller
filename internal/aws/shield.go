package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/shield"
)

type ShieldAPI interface {
	ShieldAvailable(ctx context.Context) (bool, error)
	GetSubscriptionStatus(ctx context.Context) (*shield.GetSubscriptionStateOutput, error)
	GetProtection(ctx context.Context, resourceArn *string) (*shield.Protection, error)
	CreateProtection(ctx context.Context, resourceArn *string, protectionName *string) (*shield.CreateProtectionOutput, error)
	DeleteProtection(ctx context.Context, protectionID *string) (*shield.DeleteProtectionOutput, error)
}

func (c *Cloud) ShieldAvailable(ctx context.Context) (bool, error) {
	status, err := c.GetSubscriptionStatus(ctx)
	if err != nil {
		return false, err
	}
	return *status.SubscriptionState == "ACTIVE", nil
}

func (c *Cloud) GetSubscriptionStatus(ctx context.Context) (*shield.GetSubscriptionStateOutput, error) {
	return c.shield.GetSubscriptionStateWithContext(ctx, &shield.GetSubscriptionStateInput{})
}

func (c *Cloud) GetProtection(ctx context.Context, resourceArn *string) (*shield.Protection, error) {
	result, err := c.shield.DescribeProtectionWithContext(ctx, &shield.DescribeProtectionInput{
		ResourceArn: resourceArn,
	})

	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == shield.ErrCodeResourceNotFoundException {
			return nil, nil
		}
		return nil, err
	}

	return result.Protection, nil
}

func (c *Cloud) CreateProtection(ctx context.Context, resourceArn *string, protectionName *string) (*shield.CreateProtectionOutput, error) {
	return c.shield.CreateProtectionWithContext(ctx, &shield.CreateProtectionInput{
		Name:        protectionName,
		ResourceArn: resourceArn,
	})
}

func (c *Cloud) DeleteProtection(ctx context.Context, protectionID *string) (*shield.DeleteProtectionOutput, error) {
	return c.shield.DeleteProtectionWithContext(ctx, &shield.DeleteProtectionInput{
		ProtectionId: protectionID,
	})
}
