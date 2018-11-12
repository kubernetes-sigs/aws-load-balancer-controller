package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/aws/endpoints"

	"github.com/aws/aws-sdk-go/service/waf"
	"github.com/aws/aws-sdk-go/service/wafregional"
)

type WAFRegionalAPI interface {
	WebACLExists(ctx context.Context, webACLId *string) (bool, error)
	GetWebACLSummary(ctx context.Context, resourceArn *string) (*waf.WebACLSummary, error)
	AssociateWAF(ctx context.Context, resourceArn *string, webACLId *string) (*wafregional.AssociateWebACLOutput, error)
	DisassociateWAF(ctx context.Context, resourceArn *string) (*wafregional.DisassociateWebACLOutput, error)

	// WAFRegionalAvailable whether WAFRegional service are available.
	WAFRegionalAvailable() bool
}

// WebACLExists checks whether the provided ID existing in AWS.
func (c *Cloud) WebACLExists(ctx context.Context, webACLId *string) (bool, error) {
	_, err := c.wafregional.GetWebACLWithContext(ctx, &waf.GetWebACLInput{
		WebACLId: webACLId,
	})

	if err != nil {
		return false, err
	}

	return true, nil
}

// GetWebACLSummary return associated summary for resource.
func (c *Cloud) GetWebACLSummary(ctx context.Context, resourceArn *string) (*waf.WebACLSummary, error) {
	result, err := c.wafregional.GetWebACLForResourceWithContext(ctx, &wafregional.GetWebACLForResourceInput{
		ResourceArn: resourceArn,
	})

	if err != nil {
		return nil, err
	}

	return result.WebACLSummary, nil
}

// AssociateWAF WAF ACL to resource.
func (c *Cloud) AssociateWAF(ctx context.Context, resourceArn *string, webACLId *string) (*wafregional.AssociateWebACLOutput, error) {
	result, err := c.wafregional.AssociateWebACLWithContext(ctx, &wafregional.AssociateWebACLInput{
		ResourceArn: resourceArn,
		WebACLId:    webACLId,
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// DisassociateWAF WAF ACL from resource.
func (c *Cloud) DisassociateWAF(ctx context.Context, resourceArn *string) (*wafregional.DisassociateWebACLOutput, error) {
	result, err := c.wafregional.DisassociateWebACLWithContext(ctx, &wafregional.DisassociateWebACLInput{
		ResourceArn: resourceArn,
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Cloud) WAFRegionalAvailable() bool {
	resolver := endpoints.DefaultResolver()
	_, err := resolver.EndpointFor(endpoints.WafRegionalServiceID, c.region)
	return err == nil
}
