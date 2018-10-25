package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/waf"
	"github.com/aws/aws-sdk-go/service/wafregional"
)

type WAFRegionalAPI interface {
	WebACLExists(webACLId *string) (bool, error)
	GetWebACLSummary(resourceArn *string) (*waf.WebACLSummary, error)
	Associate(resourceArn *string, webACLId *string) (*wafregional.AssociateWebACLOutput, error)
	Disassociate(resourceArn *string) (*wafregional.DisassociateWebACLOutput, error)
}

// WafACWebACLExistsLExists checks whether the provided ID existing in AWS.
func (c *Cloud) WebACLExists(webACLId *string) (bool, error) {
	_, err := c.wafregional.GetWebACL(&waf.GetWebACLInput{
		WebACLId: webACLId,
	})

	if err != nil {
		return false, err
	}

	return true, nil
}

// GetWebACLSummary return associated summary for resource.
func (c *Cloud) GetWebACLSummary(resourceArn *string) (*waf.WebACLSummary, error) {
	result, err := c.wafregional.GetWebACLForResource(&wafregional.GetWebACLForResourceInput{
		ResourceArn: aws.String(*resourceArn),
	})

	if err != nil {
		return nil, err
	}

	return result.WebACLSummary, nil
}

// Associate WAF ACL to resource.
func (c *Cloud) Associate(resourceArn *string, webACLId *string) (*wafregional.AssociateWebACLOutput, error) {
	result, err := c.wafregional.AssociateWebACL(&wafregional.AssociateWebACLInput{
		ResourceArn: resourceArn,
		WebACLId:    webACLId,
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// Disassociate WAF ACL from resource.
func (c *Cloud) Disassociate(resourceArn *string) (*wafregional.DisassociateWebACLOutput, error) {
	result, err := c.wafregional.DisassociateWebACL(&wafregional.DisassociateWebACLInput{
		ResourceArn: resourceArn,
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}
