package albwaf

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/waf"
	"github.com/aws/aws-sdk-go/service/wafregional"
	"github.com/aws/aws-sdk-go/service/wafregional/wafregionaliface"
	"github.com/karlseguin/ccache"
	albprom "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/prometheus"
	"github.com/prometheus/client_golang/prometheus"
)

// WAFRegionalsvc is a pointer to the awsutil WAFRegional service
var WAFRegionalsvc *WAFRegional

// WAFRegional is our extension to AWS's WAFRegional.wafregional
type WAFRegional struct {
	wafregionaliface.WAFRegionalAPI
	cache *ccache.Cache
}

// NewWAFRegional returns an WAFRegional based off of the provided aws.Config
func NewWAFRegional(awsSession *session.Session) {
	WAFRegionalsvc = &WAFRegional{
		wafregional.New(awsSession),
		ccache.New(ccache.Configure()),
	}
}

// WafACWebACLExistsLExists checks whether the provided ID existing in AWS.
func (a *WAFRegional) WebACLExists(webACLId *string) (bool, error) {
	cache := "WAFRegional-WebACLExists"
	key := cache + "." + *webACLId
	item := a.cache.Get(key)

	if item != nil {
		v := item.Value().(bool)
		albprom.AWSCache.With(prometheus.Labels{"cache": cache, "action": "hit"}).Add(float64(1))
		return v, nil
	}

	albprom.AWSCache.With(prometheus.Labels{"cache": cache, "action": "miss"}).Add(float64(1))

	_, err := a.GetWebACL(&waf.GetWebACLInput{
		WebACLId: webACLId,
	})

	if err != nil {
		a.cache.Set(key, false, time.Minute*5)
		return false, err
	}

	a.cache.Set(key, true, time.Minute*5)
	return true, nil
}

// GetWebACLSummary return associated summary for resource.
func (a *WAFRegional) GetWebACLSummary(resourceArn *string) (*waf.WebACLSummary, error) {
	cache := "WAFRegional-GetWebACLSummary"
	key := cache + "." + *resourceArn
	item := a.cache.Get(key)

	if item != nil {
		v := item.Value().(*waf.WebACLSummary)
		albprom.AWSCache.With(prometheus.Labels{"cache": cache, "action": "hit"}).Add(float64(1))
		return v, nil
	}

	result, err := a.GetWebACLForResource(&wafregional.GetWebACLForResourceInput{
		ResourceArn: aws.String(*resourceArn),
	})

	if err != nil {
		albprom.AWSErrorCount.With(
			prometheus.Labels{"service": "WAFRegional", "operation": "GetWebACLForResource"}).Add(float64(1))
		return nil, err
	}

	a.cache.Set(key, result.WebACLSummary, time.Minute*5)
	albprom.AWSCache.With(prometheus.Labels{"cache": cache, "action": "miss"}).Add(float64(1))
	return result.WebACLSummary, nil
}

// Associate WAF ACL to resource.
func (a *WAFRegional) Associate(resourceArn *string, webACLId *string) (*wafregional.AssociateWebACLOutput, error) {
	result, err := a.AssociateWebACL(&wafregional.AssociateWebACLInput{
		ResourceArn: resourceArn,
		WebACLId:    webACLId,
	})

	if err != nil {
		albprom.AWSErrorCount.With(
			prometheus.Labels{"service": "WAFRegional", "operation": "AssociateWebACL"}).Add(float64(1))
		return nil, err
	}

	return result, nil
}

// Disassociate WAF ACL from resource.
func (a *WAFRegional) Disassociate(resourceArn *string) (*wafregional.DisassociateWebACLOutput, error) {
	result, err := a.DisassociateWebACL(&wafregional.DisassociateWebACLInput{
		ResourceArn: resourceArn,
	})

	if err != nil {
		albprom.AWSErrorCount.With(
			prometheus.Labels{"service": "WAFRegional", "operation": "DisassociateWebACL"}).Add(float64(1))
		return nil, err
	}

	return result, nil
}
