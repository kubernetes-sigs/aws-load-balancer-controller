package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/wafv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
)

func TestCloud_GetWAFV2WebACLSummary(t *testing.T) {
	resourceArn := aws.String("arn")

	for _, tc := range []struct {
		Name                                    string
		GetWebACLForResourceWithContextResponse *wafv2.GetWebACLForResourceOutput
		GetWebACLForResourceWithContextError    error
		Expected                                *wafv2.WebACL
		ExpectedError                           error
	}{
		{
			Name:                                    "No error from GetWAFV2WebACLSummary",
			GetWebACLForResourceWithContextResponse: &wafv2.GetWebACLForResourceOutput{WebACL: &wafv2.WebACL{}},
			GetWebACLForResourceWithContextError:    nil,
			Expected:                                &wafv2.WebACL{},
		},
		{
			Name:                                 "Error from GetWAFV2WebACLSummary, ACL doesn't exist",
			GetWebACLForResourceWithContextError: errors.New("not found error"),
			ExpectedError:                        errors.New("not found error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			wafsvc := &mocks.WAFV2API{}
			wafsvc.On("GetWebACLForResourceWithContext", ctx, &wafv2.GetWebACLForResourceInput{
				ResourceArn: resourceArn,
			}).Return(tc.GetWebACLForResourceWithContextResponse, tc.GetWebACLForResourceWithContextError)

			cloud := &Cloud{
				wafv2: wafsvc,
			}

			output, err := cloud.GetWAFV2WebACLSummary(ctx, resourceArn)
			assert.Equal(t, tc.Expected, output)
			assert.Equal(t, tc.ExpectedError, err)
			wafsvc.AssertExpectations(t)
		})
	}
}

func TestCloud_AssociateWAFV2(t *testing.T) {
	resourceArn := aws.String("arn")
	webACLARN := aws.String("web_acl_arn")

	for _, tc := range []struct {
		Name                               string
		AssociateWebACLWithContextResponse *wafv2.AssociateWebACLOutput
		AssociateWebACLWithContextError    error
		Expected                           *wafv2.AssociateWebACLOutput
		ExpectedError                      error
	}{
		{
			Name:                               "No error from API",
			AssociateWebACLWithContextResponse: &wafv2.AssociateWebACLOutput{},
			AssociateWebACLWithContextError:    nil,
			Expected:                           &wafv2.AssociateWebACLOutput{},
		},
		{
			Name:                            "Error from API",
			AssociateWebACLWithContextError: errors.New("api query failed"),
			ExpectedError:                   errors.New("api query failed"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			wafsvc := &mocks.WAFV2API{}
			wafsvc.On("AssociateWebACLWithContext", ctx, &wafv2.AssociateWebACLInput{
				ResourceArn: resourceArn,
				WebACLArn:   webACLARN,
			}).Return(tc.AssociateWebACLWithContextResponse, tc.AssociateWebACLWithContextError)

			cloud := &Cloud{
				wafv2: wafsvc,
			}

			output, err := cloud.AssociateWAFV2(ctx, resourceArn, webACLARN)
			assert.Equal(t, tc.Expected, output)
			assert.Equal(t, tc.ExpectedError, err)
			wafsvc.AssertExpectations(t)
		})
	}
}

func TestCloud_DisassociateWAFV2(t *testing.T) {
	resourceArn := aws.String("arn")

	for _, tc := range []struct {
		Name                                  string
		DisassociateWebACLWithContextResponse *wafv2.DisassociateWebACLOutput
		DisassociateWebACLWithContextError    error
		Expected                              *wafv2.DisassociateWebACLOutput
		ExpectedError                         error
	}{
		{
			Name:                                  "No error from API",
			DisassociateWebACLWithContextResponse: &wafv2.DisassociateWebACLOutput{},
			DisassociateWebACLWithContextError:    nil,
			Expected:                              &wafv2.DisassociateWebACLOutput{},
		},
		{
			Name:                               "Error from API",
			DisassociateWebACLWithContextError: errors.New("api query failed"),
			ExpectedError:                      errors.New("api query failed"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			wafsvc := &mocks.WAFV2API{}
			wafsvc.On("DisassociateWebACLWithContext", ctx, &wafv2.DisassociateWebACLInput{
				ResourceArn: resourceArn,
			}).Return(tc.DisassociateWebACLWithContextResponse, tc.DisassociateWebACLWithContextError)

			cloud := &Cloud{
				wafv2: wafsvc,
			}

			output, err := cloud.DisassociateWAFV2(ctx, resourceArn)
			assert.Equal(t, tc.Expected, output)
			assert.Equal(t, tc.ExpectedError, err)
			wafsvc.AssertExpectations(t)
		})
	}
}
