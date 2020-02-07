package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/shield"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
)

func TestCloud_GetSubscriptionStatus(t *testing.T) {
	for _, tc := range []struct {
		Name                                    string
		GetSubscriptionStateWithContextResponse *shield.GetSubscriptionStateOutput
		GetSubscriptionStateWithContextError    error
		Expected                                *shield.GetSubscriptionStateOutput
		ExpectedError                           error
	}{
		{
			Name:                                    "No error from GetSubscriptionStateWithContext",
			GetSubscriptionStateWithContextResponse: &shield.GetSubscriptionStateOutput{SubscriptionState: String("ACTIVE")},
			GetSubscriptionStateWithContextError:    nil,
			Expected:                                &shield.GetSubscriptionStateOutput{SubscriptionState: String("ACTIVE")},
			ExpectedError:                           nil,
		},
		{
			Name:                                 "Error from GetSubscriptionStateWithContext, unknown error",
			GetSubscriptionStateWithContextError: errors.New(shield.ErrCodeInternalErrorException),
			Expected:                             nil,
			ExpectedError:                        errors.New(shield.ErrCodeInternalErrorException),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloudsvc := &mocks.ShieldAPI{}
			cloudsvc.On("GetSubscriptionStateWithContext", ctx, &shield.GetSubscriptionStateInput{}).Return(tc.GetSubscriptionStateWithContextResponse, tc.GetSubscriptionStateWithContextError)

			cloud := &Cloud{
				shield: cloudsvc,
			}

			b, err := cloud.GetSubscriptionStatus(ctx)
			assert.Equal(t, tc.Expected, b)
			assert.Equal(t, tc.ExpectedError, err)
			cloudsvc.AssertExpectations(t)
		})
	}
}

func TestCloud_ShieldAvailable(t *testing.T) {
	for _, tc := range []struct {
		Name                                    string
		GetSubscriptionStateWithContextResponse *shield.GetSubscriptionStateOutput
		GetSubscriptionStateWithContextError    error
		Expected                                bool
		ExpectedError                           error
	}{
		{
			Name:                                    "No error from ShieldAvailable, Shield Advanced subscription active",
			GetSubscriptionStateWithContextResponse: &shield.GetSubscriptionStateOutput{SubscriptionState: String("ACTIVE")},
			GetSubscriptionStateWithContextError:    nil,
			Expected:                                true,
			ExpectedError:                           nil,
		},
		{
			Name:                                    "No error from ShieldAvailable, Shield Advanced subscription inactive",
			GetSubscriptionStateWithContextResponse: &shield.GetSubscriptionStateOutput{SubscriptionState: String("INACTIVE")},
			GetSubscriptionStateWithContextError:    nil,
			Expected:                                false,
			ExpectedError:                           nil,
		},
		{
			Name:                                 "Error from GetSubscriptionStateWithContext, unknown error",
			GetSubscriptionStateWithContextError: errors.New(shield.ErrCodeInternalErrorException),
			Expected:                             false,
			ExpectedError:                        errors.New(shield.ErrCodeInternalErrorException),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloudsvc := &mocks.ShieldAPI{}
			cloudsvc.On("GetSubscriptionStateWithContext", ctx, &shield.GetSubscriptionStateInput{}).Return(tc.GetSubscriptionStateWithContextResponse, tc.GetSubscriptionStateWithContextError)

			cloud := &Cloud{
				shield: cloudsvc,
			}

			b, err := cloud.ShieldAvailable(ctx)
			assert.Equal(t, tc.Expected, b)
			assert.Equal(t, tc.ExpectedError, err)
			cloudsvc.AssertExpectations(t)
		})
	}
}

func TestCloud_GetProtection(t *testing.T) {
	resourceArn := aws.String("arn")

	for _, tc := range []struct {
		Name                                  string
		DescribeProtectionWithContextResponse *shield.DescribeProtectionOutput
		DescribeProtectionWithContextError    error
		error
		Expected      *shield.Protection
		ExpectedError error
	}{
		{
			Name:                                  "No error from DescribeProtection",
			DescribeProtectionWithContextResponse: &shield.DescribeProtectionOutput{Protection: &shield.Protection{}},
			DescribeProtectionWithContextError:    nil,
			Expected:                              &shield.Protection{},
		},
		{
			Name:                               "Error from DescribeProtection, protection doesn't exist",
			DescribeProtectionWithContextError: awserr.New(shield.ErrCodeResourceNotFoundException, "not found", nil),
			Expected:                           nil,
			ExpectedError:                      nil,
		},
		{
			Name:                               "Error from DescribeProtection, internal server error",
			DescribeProtectionWithContextError: errors.New(shield.ErrCodeInternalErrorException),
			ExpectedError:                      errors.New(shield.ErrCodeInternalErrorException),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloudsvc := &mocks.ShieldAPI{}
			cloudsvc.On("DescribeProtectionWithContext", ctx, &shield.DescribeProtectionInput{
				ResourceArn: resourceArn,
			}).Return(tc.DescribeProtectionWithContextResponse, tc.DescribeProtectionWithContextError)

			cloud := &Cloud{
				shield: cloudsvc,
			}

			output, err := cloud.GetProtection(ctx, resourceArn)
			assert.Equal(t, tc.Expected, output)
			assert.Equal(t, tc.ExpectedError, err)
			cloudsvc.AssertExpectations(t)
		})
	}
}

func TestCloud_CreateProtection(t *testing.T) {
	resourceArn := aws.String("arn")

	for _, tc := range []struct {
		Name                                string
		CreateProtectionWithContextResponse *shield.CreateProtectionOutput
		CreateProtectionWithContextError    error
		Expected                            *shield.CreateProtectionOutput
		ExpectedError                       error
	}{
		{
			Name:                                "No error from API",
			CreateProtectionWithContextResponse: &shield.CreateProtectionOutput{},
			CreateProtectionWithContextError:    nil,
			Expected:                            &shield.CreateProtectionOutput{},
		},
		{
			Name:                             "Error from API, concurrent modification",
			CreateProtectionWithContextError: errors.New("api query failed"),
			ExpectedError:                    errors.New("api query failed"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			shieldsvc := &mocks.ShieldAPI{}
			shieldsvc.On("CreateProtectionWithContext", ctx, &shield.CreateProtectionInput{
				Name:        aws.String("managed by aws-alb-ingress-controller"),
				ResourceArn: resourceArn,
			}).Return(tc.CreateProtectionWithContextResponse, tc.CreateProtectionWithContextError)

			cloud := &Cloud{
				shield: shieldsvc,
			}

			output, err := cloud.CreateProtection(ctx, resourceArn, aws.String("managed by aws-alb-ingress-controller"))
			assert.Equal(t, tc.Expected, output)
			assert.Equal(t, tc.ExpectedError, err)
			shieldsvc.AssertExpectations(t)
		})
	}
}

func TestCloud_DeleteProtection(t *testing.T) {
	protectionID := aws.String("protectionID")

	for _, tc := range []struct {
		Name                                string
		DeleteProtectionWithContextResponse *shield.DeleteProtectionOutput
		DeleteProtectionWithContextError    error
		Expected                            *shield.DeleteProtectionOutput
		ExpectedError                       error
	}{
		{
			Name:                                "No error from API",
			DeleteProtectionWithContextResponse: &shield.DeleteProtectionOutput{},
			DeleteProtectionWithContextError:    nil,
			Expected:                            &shield.DeleteProtectionOutput{},
		},
		{
			Name:                             "Error from API",
			DeleteProtectionWithContextError: errors.New("api query failed"),
			ExpectedError:                    errors.New("api query failed"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			shieldsvc := &mocks.ShieldAPI{}
			shieldsvc.On("DeleteProtectionWithContext", ctx, &shield.DeleteProtectionInput{
				ProtectionId: protectionID,
			}).Return(tc.DeleteProtectionWithContextResponse, tc.DeleteProtectionWithContextError)

			cloud := &Cloud{
				shield: shieldsvc,
			}

			output, err := cloud.DeleteProtection(ctx, protectionID)
			assert.Equal(t, tc.Expected, output)
			assert.Equal(t, tc.ExpectedError, err)
			shieldsvc.AssertExpectations(t)
		})
	}
}
