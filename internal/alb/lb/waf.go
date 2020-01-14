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
	webACLIdForLBCacheMaxSize = 1024
	webACLIdForLBCacheTTL     = 10 * time.Minute
)

// WAFController provides functionality to manage ALB's WAF associations.
type WAFController interface {
	Reconcile(ctx context.Context, lbArn string, ingress *extensions.Ingress) error
}

func NewWAFController(cloud aws.CloudAPI) WAFController {
	return &defaultWAFController{
		cloud:              cloud,
		webACLIdForLBCache: cache.NewLRUExpireCache(webACLIdForLBCacheMaxSize),
	}
}

type defaultWAFController struct {
	cloud aws.CloudAPI

	// cache that stores webACLIdForLBCache for LoadBalancerARN.
	// The cache value is string, while "" represents no webACL.
	webACLIdForLBCache *cache.LRUExpireCache
}

func (c *defaultWAFController) Reconcile(ctx context.Context, lbArn string, ing *extensions.Ingress) error {
	currentWebACLId, err := c.getCurrentWebACLId(ctx, lbArn)
	if err != nil {
		return err
	}
	desiredWebACLId := c.getDesiredWebACLId(ctx, ing)

	switch {
	case desiredWebACLId == "" && currentWebACLId != "":
		albctx.GetLogger(ctx).Infof("disassociate WAF on %v", lbArn)
		if _, err := c.cloud.DisassociateWAF(ctx, aws.String(lbArn)); err != nil {
			return errors.Wrapf(err, "failed to disassociate webACL on LoadBalancer %v", lbArn)
		}
		c.webACLIdForLBCache.Add(lbArn, desiredWebACLId, webACLIdForLBCacheTTL)
	case desiredWebACLId != "" && currentWebACLId != "" && desiredWebACLId != currentWebACLId:
		albctx.GetLogger(ctx).Infof("associate WAF on %v from %v to %v", lbArn, currentWebACLId, desiredWebACLId)
		if _, err := c.cloud.AssociateWAF(ctx, aws.String(lbArn), aws.String(desiredWebACLId)); err != nil {
			return errors.Wrapf(err, "failed to associate webACL on LoadBalancer %v", lbArn)
		}
		c.webACLIdForLBCache.Add(lbArn, desiredWebACLId, webACLIdForLBCacheTTL)
	case desiredWebACLId != "" && currentWebACLId == "":
		albctx.GetLogger(ctx).Infof("associate WAF on %v to %v", lbArn, desiredWebACLId)
		if _, err := c.cloud.AssociateWAF(ctx, aws.String(lbArn), aws.String(desiredWebACLId)); err != nil {
			return errors.Wrapf(err, "failed to associate webACL on LoadBalancer %v", lbArn)
		}
		c.webACLIdForLBCache.Add(lbArn, desiredWebACLId, webACLIdForLBCacheTTL)
	}
	return nil
}

func (c *defaultWAFController) getDesiredWebACLId(ctx context.Context, ing *extensions.Ingress) string {
	var webACLId string
	// support legacy waf-acl-id annotation
	_ = annotations.LoadStringAnnotation("waf-acl-id", &webACLId, ing.Annotations)
	_ = annotations.LoadStringAnnotation("web-acl-id", &webACLId, ing.Annotations)
	return webACLId
}

func (c *defaultWAFController) getCurrentWebACLId(ctx context.Context, lbArn string) (string, error) {
	cachedWebACLId, exists := c.webACLIdForLBCache.Get(lbArn)
	if exists {
		return cachedWebACLId.(string), nil
	}

	webACLSummary, err := c.cloud.GetWebACLSummary(ctx, aws.String(lbArn))
	if err != nil {
		return "", errors.Wrapf(err, "failed to get web acl for load balancer %v", lbArn)
	}

	var webACLId string
	if webACLSummary != nil {
		webACLId = aws.StringValue(webACLSummary.WebACLId)
	}

	c.webACLIdForLBCache.Add(lbArn, webACLId, webACLIdForLBCacheTTL)
	return webACLId, nil
}
