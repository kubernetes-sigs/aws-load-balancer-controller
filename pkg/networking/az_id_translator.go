package networking

import (
	"context"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
)

const defaultAZIDTranslationCacheTTL = 60 * time.Minute

// AZIDTranslator translates availability zone names between AWS accounts.
// Availability zone names are randomized per account, so the same name can refer to different
// physical zones in different accounts. Availability zone IDs are stable across accounts, so they
// are used as the intermediate representation.
type AZIDTranslator interface {
	// TranslateAZName resolves srcZoneName, as named in the cluster's own account, to the equivalent
	// zone name in the account reachable via assumeRoleArn. Returns nil when the zone cannot be
	// resolved in either account.
	TranslateAZName(ctx context.Context, assumeRoleArn string, externalId string, srcZoneName string) (*string, error)
}

// NewDefaultAZIDTranslator constructs new defaultAZIDTranslator.
func NewDefaultAZIDTranslator(ec2Client services.EC2, logger logr.Logger) *defaultAZIDTranslator {
	return &defaultAZIDTranslator{
		ec2Client: ec2Client,
		zoneCache: cache.NewExpiring(),
		logger:    logger,
	}
}

var _ AZIDTranslator = &defaultAZIDTranslator{}

// zoneCacheKey scopes a cached zone lookup to a single account.
// Availability zone names are only meaningful within an account, so entries for different accounts
// must never be shared. scope is empty for the cluster's own account and the assumed role ARN
// otherwise.
type zoneCacheKey struct {
	scope  string
	lookup string
}

type defaultAZIDTranslator struct {
	ec2Client services.EC2

	zoneCache      *cache.Expiring
	zoneCacheMutex sync.RWMutex

	logger logr.Logger
}

func (t *defaultAZIDTranslator) TranslateAZName(ctx context.Context, assumeRoleArn string, externalId string, srcZoneName string) (*string, error) {
	zoneID, err := t.resolveZoneID(ctx, srcZoneName)
	if err != nil {
		return nil, err
	}
	if zoneID == nil {
		return nil, nil
	}

	assumedRoleEC2, err := t.ec2Client.AssumeRole(ctx, assumeRoleArn, externalId)
	if err != nil {
		return nil, err
	}
	return t.resolveZoneName(ctx, assumedRoleEC2, assumeRoleArn, *zoneID)
}

// resolveZoneID resolves a zone name to its zone ID within the cluster's own account.
func (t *defaultAZIDTranslator) resolveZoneID(ctx context.Context, zoneName string) (*string, error) {
	cacheKey := zoneCacheKey{lookup: zoneName}
	if cachedZoneID, exists := t.fetchZoneFromCache(cacheKey); exists {
		return &cachedZoneID, nil
	}

	resp, err := t.ec2Client.DescribeAvailabilityZonesWithContext(ctx, &ec2sdk.DescribeAvailabilityZonesInput{
		ZoneNames: []string{zoneName},
	})
	if err != nil {
		return nil, err
	}
	for _, azInfo := range resp.AvailabilityZones {
		if awssdk.ToString(azInfo.ZoneName) != zoneName {
			continue
		}
		zoneID := awssdk.ToString(azInfo.ZoneId)
		if zoneID == "" {
			continue
		}
		t.saveZoneToCache(cacheKey, zoneID)
		return &zoneID, nil
	}
	t.logger.Info("unable to resolve availability zone ID", "zoneName", zoneName)
	return nil, nil
}

// resolveZoneName resolves a zone ID to the zone name used by the account reachable via ec2Client.
func (t *defaultAZIDTranslator) resolveZoneName(ctx context.Context, ec2Client services.EC2, scope string, zoneID string) (*string, error) {
	cacheKey := zoneCacheKey{scope: scope, lookup: zoneID}
	if cachedZoneName, exists := t.fetchZoneFromCache(cacheKey); exists {
		return &cachedZoneName, nil
	}

	resp, err := ec2Client.DescribeAvailabilityZonesWithContext(ctx, &ec2sdk.DescribeAvailabilityZonesInput{
		ZoneIds: []string{zoneID},
	})
	if err != nil {
		return nil, err
	}
	for _, azInfo := range resp.AvailabilityZones {
		if awssdk.ToString(azInfo.ZoneId) != zoneID {
			continue
		}
		zoneName := awssdk.ToString(azInfo.ZoneName)
		if zoneName == "" {
			continue
		}
		t.saveZoneToCache(cacheKey, zoneName)
		return &zoneName, nil
	}
	t.logger.Info("unable to resolve availability zone name", "zoneID", zoneID, "scope", scope)
	return nil, nil
}

func (t *defaultAZIDTranslator) fetchZoneFromCache(key zoneCacheKey) (string, bool) {
	t.zoneCacheMutex.RLock()
	defer t.zoneCacheMutex.RUnlock()

	if rawCacheItem, exists := t.zoneCache.Get(key); exists {
		return rawCacheItem.(string), true
	}
	return "", false
}

func (t *defaultAZIDTranslator) saveZoneToCache(key zoneCacheKey, value string) {
	t.zoneCacheMutex.Lock()
	defer t.zoneCacheMutex.Unlock()

	t.zoneCache.Set(key, value, defaultAZIDTranslationCacheTTL)
}
