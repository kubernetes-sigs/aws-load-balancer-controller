package wafv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	wafv2sdk "github.com/aws/aws-sdk-go/service/wafv2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"time"
)

const defaultWebACLARNByResourceARNCacheTTL = 10 * time.Minute

// WebACLAssociationManager is responsible for manage WAFv2 webACL associations.
type WebACLAssociationManager interface {
	// AssociateWebACL associate webACL to resources.
	AssociateWebACL(ctx context.Context, resourceARN string, webACLARN string) error

	// DisassociateWebACL disassociate webACL from resources.
	DisassociateWebACL(ctx context.Context, resourceARN string) error

	// GetAssociatedWebACL returns the associated webACL for resource, returns empty if no webACL is associated.
	GetAssociatedWebACL(ctx context.Context, resourceARN string) (string, error)
}

// NewDefaultWebACLAssociationManager constructs new defaultWebACLAssociationManager.
func NewDefaultWebACLAssociationManager(wafv2Client services.WAFv2, logger logr.Logger) *defaultWebACLAssociationManager {
	return &defaultWebACLAssociationManager{
		wafv2Client:                    wafv2Client,
		logger:                         logger,
		webACLARNByResourceARNCache:    cache.NewExpiring(),
		webACLARNByResourceARNCacheTTL: defaultWebACLARNByResourceARNCacheTTL,
	}
}

var _ WebACLAssociationManager = &defaultWebACLAssociationManager{}

// default implementation for WebACLAssociationManager.
type defaultWebACLAssociationManager struct {
	wafv2Client services.WAFv2
	logger      logr.Logger

	// cache that stores webACLARN indexed by resourceARN
	// The cache value is string, while "" represents no webACL.
	webACLARNByResourceARNCache *cache.Expiring
	// ttl for webACLARNByResourceARNCache
	webACLARNByResourceARNCacheTTL time.Duration
}

func (m *defaultWebACLAssociationManager) AssociateWebACL(ctx context.Context, resourceARN string, webACLARN string) error {
	req := &wafv2sdk.AssociateWebACLInput{
		ResourceArn: awssdk.String(resourceARN),
		WebACLArn:   awssdk.String(webACLARN),
	}
	m.logger.Info("associating WAFv2 webACL",
		"resourceARN", resourceARN,
		"webACLARN", webACLARN)
	if _, err := m.wafv2Client.AssociateWebACLWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("associated WAFv2 webACL",
		"resourceARN", resourceARN,
		"webACLARN", webACLARN)
	m.webACLARNByResourceARNCache.Set(resourceARN, webACLARN, m.webACLARNByResourceARNCacheTTL)
	return nil
}

func (m *defaultWebACLAssociationManager) DisassociateWebACL(ctx context.Context, resourceARN string) error {
	req := &wafv2sdk.DisassociateWebACLInput{
		ResourceArn: awssdk.String(resourceARN),
	}
	m.logger.Info("disassociating WAFv2 webACL",
		"resourceARN", resourceARN)
	if _, err := m.wafv2Client.DisassociateWebACLWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("disassociated WAFv2 webACL",
		"resourceARN", resourceARN)
	m.webACLARNByResourceARNCache.Set(resourceARN, "", m.webACLARNByResourceARNCacheTTL)
	return nil
}

func (m *defaultWebACLAssociationManager) GetAssociatedWebACL(ctx context.Context, resourceARN string) (string, error) {
	rawCacheItem, exists := m.webACLARNByResourceARNCache.Get(resourceARN)
	if exists {
		return rawCacheItem.(string), nil
	}

	req := &wafv2sdk.GetWebACLForResourceInput{
		ResourceArn: awssdk.String(resourceARN),
	}

	resp, err := m.wafv2Client.GetWebACLForResourceWithContext(ctx, req)
	if err != nil {
		return "", err
	}
	var webACLARN string
	if resp.WebACL != nil {
		webACLARN = awssdk.StringValue(resp.WebACL.ARN)
	}

	m.webACLARNByResourceARNCache.Set(resourceARN, webACLARN, m.webACLARNByResourceARNCacheTTL)
	return webACLARN, nil
}
