package albwafregional

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/waf"
	"github.com/aws/aws-sdk-go/service/wafregional"
	"github.com/aws/aws-sdk-go/service/wafregional/wafregionaliface"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// WAFRegionalsvc is a pointer to the awsutil WAFRegional service
var WAFRegionalsvc *WAFRegional

// WAFRegional is our extension to AWS's WAFRegional.wafregional
type WAFRegional struct {
	wafregionaliface.WAFRegionalAPI
	mc metric.Collector
}

// NewWAFRegional returns an WAFRegional based off of the provided aws.Config
func NewWAFRegional(awsSession *session.Session, mc metric.Collector) {
	WAFRegionalsvc = &WAFRegional{
		WAFRegionalAPI: wafregional.New(awsSession),
		mc:             mc,
	}
	if WAFRegionalsvc.mc == nil {
		// prevent nil pointer panic
		WAFRegionalsvc.mc = metric.DummyCollector{}
	}
}

// WafACWebACLExistsLExists checks whether the provided ID existing in AWS.
func (a *WAFRegional) WebACLExists(webACLId *string) (bool, error) {
	cacheName := "WAFRegional.WebACLExists"
	item := albcache.Get(cacheName, *webACLId)

	if item != nil {
		v := item.Value().(bool)
		return v, nil
	}

	start := time.Now()
	_, err := a.GetWebACL(&waf.GetWebACLInput{
		WebACLId: webACLId,
	})
	a.mc.ObserveAPIRequest(prometheus.Labels{"operation": "GetWebACL"}, start)

	if err != nil {
		albcache.Set(cacheName, *webACLId, false, time.Minute*5)
		return false, err
	}

	albcache.Set(cacheName, *webACLId, true, time.Minute*5)
	return true, nil
}

// GetWebACLSummary return associated summary for resource.
func (a *WAFRegional) GetWebACLSummary(resourceArn *string) (*waf.WebACLSummary, error) {
	cacheName := "WAFRegional.GetWebACLSummary"
	item := albcache.Get(cacheName, *resourceArn)

	if item != nil {
		v := item.Value().(*waf.WebACLSummary)
		return v, nil
	}

	start := time.Now()
	result, err := a.GetWebACLForResource(&wafregional.GetWebACLForResourceInput{
		ResourceArn: aws.String(*resourceArn),
	})
	if err != nil {
		return nil, err
	}
	a.mc.ObserveAPIRequest(prometheus.Labels{"operation": "GetWebACLForResource"}, start)

	albcache.Set(cacheName, *resourceArn, result.WebACLSummary, time.Minute*5)
	return result.WebACLSummary, nil
}

// Associate WAF ACL to resource.
func (a *WAFRegional) Associate(resourceArn *string, webACLId *string) (*wafregional.AssociateWebACLOutput, error) {
	start := time.Now()
	result, err := a.AssociateWebACL(&wafregional.AssociateWebACLInput{
		ResourceArn: resourceArn,
		WebACLId:    webACLId,
	})
	if err != nil {
		return nil, err
	}
	a.mc.ObserveAPIRequest(prometheus.Labels{"operation": "AssociateWebACL"}, start)

	return result, nil
}

// Disassociate WAF ACL from resource.
func (a *WAFRegional) Disassociate(resourceArn *string) (*wafregional.DisassociateWebACLOutput, error) {
	start := time.Now()
	result, err := a.DisassociateWebACL(&wafregional.DisassociateWebACLInput{
		ResourceArn: resourceArn,
	})
	if err != nil {
		return nil, err
	}
	a.mc.ObserveAPIRequest(prometheus.Labels{"operation": "DisassociateWebACL"}, start)

	return result, nil
}
