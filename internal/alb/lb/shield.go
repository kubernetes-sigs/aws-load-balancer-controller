package lb

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"

	"github.com/aws/aws-sdk-go/service/shield"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/pkg/errors"
	extensions "k8s.io/api/extensions/v1beta1"

	"k8s.io/apimachinery/pkg/util/cache"
)

const (
	protectionEnabledForLBCacheMaxSize = 1024
	protectionEnabledForLBCacheTTL     = 10 * time.Second
	shieldAvailableCacheMaxSize        = 1
	shieldAvailableCacheTTL            = 10 * time.Second
	shieldAvailableCacheKey            = "shieldAvailable"
	protectionName                     = "managed by aws-alb-ingress-controller"
)

// ShieldController provides functionality to manage ALB's Shield Advanced protection.
type ShieldController interface {
	Reconcile(ctx context.Context, lbArn string, ingress *extensions.Ingress) error
}

func NewShieldController(cloud aws.CloudAPI) ShieldController {
	return &defaultShieldController{
		cloud:                       cloud,
		protectionEnabledForLBCache: cache.NewLRUExpireCache(protectionEnabledForLBCacheMaxSize),
		shieldAvailableCache:        cache.NewLRUExpireCache(shieldAvailableCacheMaxSize),
	}
}

type defaultShieldController struct {
	cloud aws.CloudAPI

	// cache that stores protection id for LoadBalancerARN.
	// The cache value is string, while "" represents no protection.
	protectionEnabledForLBCache *cache.LRUExpireCache
	// cache that stores shield availability for current account.
	// The cache value is string, while "" represents not available.
	shieldAvailableCache *cache.LRUExpireCache
}

func (c *defaultShieldController) Reconcile(ctx context.Context, lbArn string, ingress *extensions.Ingress) error {
	var enableProtection bool
	annotationPresent, err := annotations.LoadBoolAnnocation("shield-advanced-protection", &enableProtection, ingress.Annotations)
	if err != nil {
		return err
	}
	if !annotationPresent {
		return nil
	}

	available, err := c.getCurrentShieldAvailability(ctx)
	if err != nil {
		return err
	}

	if enableProtection && !available {
		return fmt.Errorf("unable to create shield advanced protection for loadBalancer %v, shield advanced subscription is not active", lbArn)
	}

	protection, err := c.getCurrentProtectionStatus(ctx, lbArn)
	if err != nil {
		return fmt.Errorf("failed to get shield advanced protection status for loadBalancer %v due to %v", lbArn, err)
	}

	if protection == nil && enableProtection {
		_, err := c.cloud.CreateProtection(ctx, aws.String(lbArn), aws.String(protectionName))
		if err != nil {
			return fmt.Errorf("failed to create shield advanced protection for loadBalancer %v due to %v", lbArn, err)
		}
		albctx.GetLogger(ctx).Infof("enabled shield advanced protection for %v", lbArn)
	} else if protection != nil && !enableProtection {
		if aws.StringValue(protection.Name) == protectionName {
			_, err := c.cloud.DeleteProtection(ctx, protection.Id)
			c.protectionEnabledForLBCache.Remove(lbArn)
			if err != nil {
				return fmt.Errorf("failed to delete shield advanced protection for loadBalancer %v due to %v", lbArn, err)
			}

			albctx.GetLogger(ctx).Infof("deleted shield advanced protection for %v", lbArn)
		} else {
			albctx.GetLogger(ctx).Warnf("unable to delete shield advanced protection for %v, the protection name does not match \"%v\"", lbArn, protectionName)
		}
	}
	return nil
}

func (c *defaultShieldController) getCurrentShieldAvailability(ctx context.Context) (bool, error) {
	cachedShieldAvailable, exists := c.shieldAvailableCache.Get(shieldAvailableCacheKey)
	if exists {
		available, err := strconv.ParseBool(cachedShieldAvailable.(string))
		if err == nil {
			return available, nil
		}
	}

	available, err := c.cloud.ShieldAvailable(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get shield advanced subscription state %v", err)
	}
	c.shieldAvailableCache.Add(shieldAvailableCacheKey, "true", shieldAvailableCacheTTL)
	return available, nil
}

func (c *defaultShieldController) getCurrentProtectionStatus(ctx context.Context, lbArn string) (*shield.Protection, error) {
	cachedProtection, exists := c.protectionEnabledForLBCache.Get(lbArn)
	if exists {
		return cachedProtection.(*shield.Protection), nil
	}

	protection, err := c.cloud.GetProtection(ctx, aws.String(lbArn))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get protection status for load balancer %v", lbArn)
	}

	c.protectionEnabledForLBCache.Add(lbArn, protection, protectionEnabledForLBCacheTTL)
	return protection, nil
}
