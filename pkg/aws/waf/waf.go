package waf

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/wafregional"
	"github.com/aws/aws-sdk-go/service/wafregional/wafregionaliface"
	"github.com/prometheus/client_golang/prometheus"
)

// WAFRegionalsvc is a pointer to the awsutil WAFRegional service
var WAFRegionalsvc *WAFRegional

// WAFRegional is our extension to AWS's WAFRegional.wafregional
type WAFRegional struct {
	Svc wafregionaliface.WAFRegionalAPI
}

// NewWAFRegional returns an WAFRegional based off of the provided aws.Config
func NewWAFRegional(awsSession *session.Session) *WAFRegional {
	wafClient := WAFRegional{
		wafregional.New(awsSession),
	}
	return &wafClient
}

// WafAclExists checks whether the provided ID existing in AWS.
func (a *WAFRegional) WafAclExists(web_acl_id *string) bool {
	params := &waf.GetWebACLInput{
		WebACLId: web_acl_id,
	}
	_, err := a.Svc.GetWebACL(params)

	if err != nil {
		return false
	}
	return true
}

// GetWebACLSummary return associated summary for resource.
func (a *WAFRegional) GetWebACLSummary(resourceArn *string) (*waf.WebACLSummary, error) {
	params := &wafregional.GetWebACLForResourceInput{
		ResourceArn: aws.String(*resourceArn),
	}
	result, err := a.Svc.GetWebACLForResource(params)

	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "WAFRegional", "request": "GetWebACLForResource"}).Add(float64(1))
		return nil, err
	}

	return result.WebACLSummary, nil
}

// Associate WAF ACL to resource.
func (a *WAFRegional) Associate(resourceArn *string, wafAclId *string) (*wafregional.AssociateWebACLOutput, error) {
	params := &wafregional.AssociateWebACLInput{
		ResourceArn: aws.String(*resourceArn),
		WebACLId:    aws.String(*wafAclId),
	}
	result, err := a.Svc.AssociateWebACL(params)

	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "WAFRegional", "request": "AssociateWebACL"}).Add(float64(1))
		return nil, err
	}

	return result, nil
}

// Disassociate WAF ACL from resource.
func (a *WAFRegional) Disassociate(resourceArn *string) (*wafregional.DisassociateWebACLOutput, error) {
	params := &wafregional.DisassociateWebACLInput{
		ResourceArn: aws.String(*resourceArn),
	}
	result, err := a.Svc.DisassociateWebACL(params)

	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "WAFRegional", "request": "DisassociateWebACL"}).Add(float64(1))
		return nil, err
	}

	return result, nil
}
