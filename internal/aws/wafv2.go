package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/wafv2"
)

type WAFV2API interface {
	GetWAFV2WebACLSummary(ctx context.Context, webACLId *string) (*wafv2.WebACL, error)
	AssociateWAFV2(ctx context.Context, resourceArn *string, webACLId *string) (*wafv2.AssociateWebACLOutput, error)
	GetWebACLARN(ctx context.Context, webACLName *string, webACLId *string) (string, error)
	DisassociateWAFV2(ctx context.Context, resourceArn *string) (*wafv2.DisassociateWebACLOutput, error) 
}

// GetWAFV2WebACLSummary return associated summary for resource.
func (c *Cloud) GetWAFV2WebACLSummary(ctx context.Context, resourceArn *string) (*wafv2.WebACL, error) {
	result, err := c.wafv2.GetWebACLForResourceWithContext(ctx, &wafv2.GetWebACLForResourceInput{
		ResourceArn: resourceArn,
	})

	if err != nil {
		return nil, err
	}

	return result.WebACL, nil
}

// AssociateWAFV2 WAF ACL to resource.
func (c *Cloud) AssociateWAFV2(ctx context.Context, resourceArn *string, webACLARN *string) (*wafv2.AssociateWebACLOutput, error) {
	result, err := c.wafv2.AssociateWebACLWithContext(ctx, &wafv2.AssociateWebACLInput{
		ResourceArn: resourceArn,
		WebACLArn:    webACLARN,
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetWebACLARN return associated ARN for WAFv2 ACL resource.
func (c *Cloud) GetWebACLARN(ctx context.Context, webACLName *string, webACLId *string) (string, error) {
	// TODO: Is this neccessary?
	//       Could I just return "arn:aws:$REGION:wafv2:somethng-something"?
	//       Cross account concerns?
	result, err := c.wafv2.GetWebACLWithContext(ctx, &wafv2.GetWebACLInput{
		Id:      webACLId,
		Name:    webACLName,
		Scope:   String(wafv2.ScopeRegional),
	})

	if err != nil {
		return "", err
	}

	return aws.StringValue(result.WebACL.ARN), nil
}

// DisassociateWAFV2 WAFv2 ACL from resource.
func (c *Cloud) DisassociateWAFV2(ctx context.Context, resourceArn *string) (*wafv2.DisassociateWebACLOutput, error) {
	result, err := c.wafv2.DisassociateWebACLWithContext(ctx, &wafv2.DisassociateWebACLInput{
		ResourceArn: resourceArn,
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}
