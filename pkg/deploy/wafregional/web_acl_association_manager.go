package wafregional

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	wafregionalsdk "github.com/aws/aws-sdk-go/service/wafregional"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"time"
)

const defaultWebACLIDByResourceARNCacheTTL = 10 * time.Minute

// WebACLAssociationManager is responsible for manage WAFRegion webACL associations.
type WebACLAssociationManager interface {
	// AssociateWebACL associate webACL to resources.
	AssociateWebACL(ctx context.Context, resourceARN string, webACLID string) error

	// DisassociateWebACL disassociate webACL from resources.
	DisassociateWebACL(ctx context.Context, resourceARN string) error

	// GetAssociatedWebACL returns the associated webACL for resource, returns empty if no webACL is associated.
	GetAssociatedWebACL(ctx context.Context, resourceARN string) (string, error)
}

// NewDefaultWebACLAssociationManager constructs new defaultWebACLAssociationManager.
func NewDefaultWebACLAssociationManager(wafRegionalClient services.WAFRegional, logger logr.Logger) *defaultWebACLAssociationManager {
	return &defaultWebACLAssociationManager{
		wafRegionalClient:             wafRegionalClient,
		logger:                        logger,
		webACLIDByResourceARNCache:    cache.NewExpiring(),
		webACLIDByResourceARNCacheTTL: defaultWebACLIDByResourceARNCacheTTL,
	}
}

var _ WebACLAssociationManager = &defaultWebACLAssociationManager{}

// default implementation for WebACLAssociationManager.
type defaultWebACLAssociationManager struct {
	wafRegionalClient services.WAFRegional
	logger            logr.Logger

	// cache that stores webACLARN indexed by resourceARN
	// The cache value is string, while "" represents no webACL.
	webACLIDByResourceARNCache *cache.Expiring
	// ttl for webACLARNByResourceARNCache
	webACLIDByResourceARNCacheTTL time.Duration
}

func (m *defaultWebACLAssociationManager) AssociateWebACL(ctx context.Context, resourceARN string, webACLID string) error {
	req := &wafregionalsdk.AssociateWebACLInput{
		ResourceArn: awssdk.String(resourceARN),
		WebACLId:    awssdk.String(webACLID),
	}
	m.logger.Info("associating WAFRegional webACL",
		"resourceARN", resourceARN,
		"webACLID", webACLID)
	if _, err := m.wafRegionalClient.AssociateWebACLWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("associated WAFRegional webACL",
		"resourceARN", resourceARN,
		"webACLID", webACLID)
	m.webACLIDByResourceARNCache.Set(resourceARN, webACLID, m.webACLIDByResourceARNCacheTTL)
	return nil
}

func (m *defaultWebACLAssociationManager) DisassociateWebACL(ctx context.Context, resourceARN string) error {
	req := &wafregionalsdk.DisassociateWebACLInput{
		ResourceArn: awssdk.String(resourceARN),
	}
	m.logger.Info("disassociating WAFRegional webACL",
		"resourceARN", resourceARN)
	if _, err := m.wafRegionalClient.DisassociateWebACLWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("disassociated WAFRegional webACL",
		"resourceARN", resourceARN)
	m.webACLIDByResourceARNCache.Set(resourceARN, "", m.webACLIDByResourceARNCacheTTL)
	return nil
}

func (m *defaultWebACLAssociationManager) GetAssociatedWebACL(ctx context.Context, resourceARN string) (string, error) {
	rawCacheItem, exists := m.webACLIDByResourceARNCache.Get(resourceARN)
	if exists {
		return rawCacheItem.(string), nil
	}

	req := &wafregionalsdk.GetWebACLForResourceInput{
		ResourceArn: awssdk.String(resourceARN),
	}

	resp, err := m.wafRegionalClient.GetWebACLForResourceWithContext(ctx, req)
	if err != nil {
		return "", err
	}
	var webACLID string
	if resp.WebACLSummary != nil {
		webACLID = awssdk.StringValue(resp.WebACLSummary.WebACLId)
	}

	m.webACLIDByResourceARNCache.Set(resourceARN, webACLID, m.webACLIDByResourceARNCacheTTL)
	return webACLID, nil
}
