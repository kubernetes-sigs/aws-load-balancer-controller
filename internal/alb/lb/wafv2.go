package lb

import (
	"context"
	"time"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/pkg/errors"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/cache"
)

const (
	webACLARNForLBCacheMaxSize = 1024
	webACLARNForLBCacheTTL     = 10 * time.Minute
)

// WAFCV2ontroller provides functionality to manage ALB's WAF V2 associations.
type WAFV2Controller interface {
	Reconcile(ctx context.Context, lbArn string, ingress *extensions.Ingress) error
}

func NewWAFV2Controller(cloud aws.CloudAPI) WAFV2Controller {
	return &defaultWAFV2Controller{
		cloud:               cloud,
		webACLARNForLBCache: cache.NewLRUExpireCache(webACLARNForLBCacheMaxSize),
	}
}

type defaultWAFV2Controller struct {
	cloud aws.CloudAPI

	// cache that stores webACLARNForLBCache for LoadBalancerARN.
	// The cache value is string, while "" represents no webACL.
	webACLARNForLBCache *cache.LRUExpireCache
}

func (c *defaultWAFV2Controller) Reconcile(ctx context.Context, lbArn string, ing *extensions.Ingress) error {
	var desiredWebACLARN string

	annotationPresent := annotations.LoadStringAnnotation("wafv2-acl-arn", &desiredWebACLARN, ing.Annotations)
	if !annotationPresent {
		// No annotation found, so do nothing
		return nil
	}

	currentWebACLId, err := c.getCurrentWebACLARN(ctx, lbArn)
	if err != nil {
		return err
	}

	switch {
	case desiredWebACLARN == "" && currentWebACLId != "":
		albctx.GetLogger(ctx).Infof("disassociate WAFv2 webACL on %v", lbArn)
		if _, err := c.cloud.DisassociateWAFV2(ctx, aws.String(lbArn)); err != nil {
			return errors.Wrapf(err, "failed to disassociate WAFv2 webACL on LoadBalancer %v", lbArn)
		}
		c.webACLARNForLBCache.Add(lbArn, desiredWebACLARN, webACLARNForLBCacheTTL)
	case desiredWebACLARN != "" && currentWebACLId != "" && desiredWebACLARN != currentWebACLId:
		albctx.GetLogger(ctx).Infof("change WAFv2 webACL on %v from %v to %v", lbArn, currentWebACLId, desiredWebACLARN)
		if _, err := c.cloud.AssociateWAFV2(ctx, aws.String(lbArn), aws.String(desiredWebACLARN)); err != nil {
			return errors.Wrapf(err, "failed to associate WAFv2 webACL on LoadBalancer %v", lbArn)
		}
		c.webACLARNForLBCache.Add(lbArn, desiredWebACLARN, webACLARNForLBCacheTTL)
	case desiredWebACLARN != "" && currentWebACLId == "":
		albctx.GetLogger(ctx).Infof("associate WAFv2 webACL %v on %v", desiredWebACLARN, lbArn)
		if _, err := c.cloud.AssociateWAFV2(ctx, aws.String(lbArn), aws.String(desiredWebACLARN)); err != nil {
			return errors.Wrapf(err, "failed to associate WAFv2 webACL on LoadBalancer %v", lbArn)
		}
		c.webACLARNForLBCache.Add(lbArn, desiredWebACLARN, webACLARNForLBCacheTTL)
	}

	return nil
}

func (c *defaultWAFV2Controller) getCurrentWebACLARN(ctx context.Context, lbArn string) (string, error) {
	cachedWebACLARN, exists := c.webACLARNForLBCache.Get(lbArn)
	if exists {
		return cachedWebACLARN.(string), nil
	}

	webACL, err := c.cloud.GetWAFV2WebACLSummary(ctx, aws.String(lbArn))
	if err != nil {
		return "", errors.Wrapf(err, "failed get WAFv2 webACL for load balancer %v", lbArn)
	}

	var webACLARN string
	if webACL != nil {
		webACLARN = aws.StringValue(webACL.ARN)
	}

	c.webACLARNForLBCache.Add(lbArn, webACLARN, webACLARNForLBCacheTTL)
	return webACLARN, nil
}
