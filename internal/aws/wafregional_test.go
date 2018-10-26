package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/waf"
	"github.com/aws/aws-sdk-go/service/wafregional"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
)

func TestCloud_WebACLExists(t *testing.T) {
	webACLId := aws.String("web_acl_id")

	for _, tc := range []struct {
		Name                      string
		GetWebACLWithContextError error
		Expected                  bool
		ExpectedError             error
	}{
		{
			Name:                      "No error from GetWebACL",
			GetWebACLWithContextError: nil,
			Expected:                  true,
			ExpectedError:             nil,
		},
		{
			Name:                      "Error from GetWebACL, ACL doesn't exist",
			GetWebACLWithContextError: errors.New("Not found error"),
			Expected:                  false,
			ExpectedError:             errors.New("Not found error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			wafsvc := &mocks.WAFRegionalAPI{}
			wafsvc.On("GetWebACLWithContext", ctx, &waf.GetWebACLInput{
				WebACLId: webACLId,
			}).Return(nil, tc.GetWebACLWithContextError)

			cloud := &Cloud{
				wafregional: wafsvc,
			}

			b, err := cloud.WebACLExists(ctx, webACLId)
			assert.Equal(t, tc.Expected, b)
			assert.Equal(t, tc.ExpectedError, err)
			wafsvc.AssertExpectations(t)
		})
	}
}

func TestCloud_GetWebACLSummary(t *testing.T) {
	resourceArn := aws.String("arn")

	for _, tc := range []struct {
		Name                                    string
		GetWebACLForResourceWithContextResponse *wafregional.GetWebACLForResourceOutput
		GetWebACLForResourceWithContextError    error
		Expected                                *waf.WebACLSummary
		ExpectedError                           error
	}{
		{
			Name:                                    "No error from GetWebACL",
			GetWebACLForResourceWithContextResponse: &wafregional.GetWebACLForResourceOutput{WebACLSummary: &waf.WebACLSummary{}},
			GetWebACLForResourceWithContextError:    nil,
			Expected:                                &waf.WebACLSummary{},
		},
		{
			Name:                                 "Error from GetWebACL, ACL doesn't exist",
			GetWebACLForResourceWithContextError: errors.New("not found error"),
			ExpectedError:                        errors.New("not found error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			wafsvc := &mocks.WAFRegionalAPI{}
			wafsvc.On("GetWebACLForResourceWithContext", ctx, &wafregional.GetWebACLForResourceInput{
				ResourceArn: resourceArn,
			}).Return(tc.GetWebACLForResourceWithContextResponse, tc.GetWebACLForResourceWithContextError)

			cloud := &Cloud{
				wafregional: wafsvc,
			}

			output, err := cloud.GetWebACLSummary(ctx, resourceArn)
			assert.Equal(t, tc.Expected, output)
			assert.Equal(t, tc.ExpectedError, err)
			wafsvc.AssertExpectations(t)
		})
	}
}

func TestCloud_AssociateWAF(t *testing.T) {
	resourceArn := aws.String("arn")
	webACLId := aws.String("web_acl_id")

	for _, tc := range []struct {
		Name                               string
		AssociateWebACLWithContextResponse *wafregional.AssociateWebACLOutput
		AssociateWebACLWithContextError    error
		Expected                           *wafregional.AssociateWebACLOutput
		ExpectedError                      error
	}{
		{
			Name:                               "No error from API",
			AssociateWebACLWithContextResponse: &wafregional.AssociateWebACLOutput{},
			AssociateWebACLWithContextError:    nil,
			Expected:                           &wafregional.AssociateWebACLOutput{},
		},
		{
			Name:                            "Error from API",
			AssociateWebACLWithContextError: errors.New("api query failed"),
			ExpectedError:                   errors.New("api query failed"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			wafsvc := &mocks.WAFRegionalAPI{}
			wafsvc.On("AssociateWebACLWithContext", ctx, &wafregional.AssociateWebACLInput{
				ResourceArn: resourceArn,
				WebACLId:    webACLId,
			}).Return(tc.AssociateWebACLWithContextResponse, tc.AssociateWebACLWithContextError)

			cloud := &Cloud{
				wafregional: wafsvc,
			}

			output, err := cloud.AssociateWAF(ctx, resourceArn, webACLId)
			assert.Equal(t, tc.Expected, output)
			assert.Equal(t, tc.ExpectedError, err)
			wafsvc.AssertExpectations(t)
		})
	}
}

func TestCloud_DisassociateWAF(t *testing.T) {
	resourceArn := aws.String("arn")

	for _, tc := range []struct {
		Name                                  string
		DisassociateWebACLWithContextResponse *wafregional.DisassociateWebACLOutput
		DisassociateWebACLWithContextError    error
		Expected                              *wafregional.DisassociateWebACLOutput
		ExpectedError                         error
	}{
		{
			Name:                                  "No error from API",
			DisassociateWebACLWithContextResponse: &wafregional.DisassociateWebACLOutput{},
			DisassociateWebACLWithContextError:    nil,
			Expected:                              &wafregional.DisassociateWebACLOutput{},
		},
		{
			Name:                               "Error from API",
			DisassociateWebACLWithContextError: errors.New("api query failed"),
			ExpectedError:                      errors.New("api query failed"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			wafsvc := &mocks.WAFRegionalAPI{}
			wafsvc.On("DisassociateWebACLWithContext", ctx, &wafregional.DisassociateWebACLInput{
				ResourceArn: resourceArn,
			}).Return(tc.DisassociateWebACLWithContextResponse, tc.DisassociateWebACLWithContextError)

			cloud := &Cloud{
				wafregional: wafsvc,
			}

			output, err := cloud.DisassociateWAF(ctx, resourceArn)
			assert.Equal(t, tc.Expected, output)
			assert.Equal(t, tc.ExpectedError, err)
			wafsvc.AssertExpectations(t)
		})
	}
}
