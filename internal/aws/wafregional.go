package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/service/waf"
	"github.com/aws/aws-sdk-go/service/wafregional"
)

type WAFRegionalAPI interface {
	WebACLExists(webACLId *string) (bool, error)
	GetWebACLSummary(resourceArn *string) (*waf.WebACLSummary, error)
	AssociateWAF(resourceArn *string, webACLId *string) (*wafregional.AssociateWebACLOutput, error)
	DisassociateWAF(resourceArn *string) (*wafregional.DisassociateWebACLOutput, error)
}

// WafACWebACLExistsLExists checks whether the provided ID existing in AWS.
func (c *Cloud) WebACLExists(webACLId *string) (bool, error) {
	_, err := c.wafregional.GetWebACLWithContext(context.TODO(), &waf.GetWebACLInput{
		WebACLId: webACLId,
	})

	if err != nil {
		return false, err
	}

	return true, nil
}

// GetWebACLSummary return associated summary for resource.
func (c *Cloud) GetWebACLSummary(resourceArn *string) (*waf.WebACLSummary, error) {
	result, err := c.wafregional.GetWebACLForResourceWithContext(context.TODO(), &wafregional.GetWebACLForResourceInput{
		ResourceArn: resourceArn,
	})

	if err != nil {
		return nil, err
	}

	return result.WebACLSummary, nil
}

// AssociateWAF WAF ACL to resource.
func (c *Cloud) AssociateWAF(resourceArn *string, webACLId *string) (*wafregional.AssociateWebACLOutput, error) {
	result, err := c.wafregional.AssociateWebACLWithContext(context.TODO(), &wafregional.AssociateWebACLInput{
		ResourceArn: resourceArn,
		WebACLId:    webACLId,
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// DisassociateWAF WAF ACL from resource.
func (c *Cloud) DisassociateWAF(resourceArn *string) (*wafregional.DisassociateWebACLOutput, error) {
	result, err := c.wafregional.DisassociateWebACLWithContext(context.TODO(), &wafregional.DisassociateWebACLInput{
		ResourceArn: resourceArn,
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}
