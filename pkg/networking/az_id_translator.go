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

// zoneMapping holds every availability zone of a single account, indexed by name and by ID.
type zoneMapping struct {
	idByName map[string]string
	nameByID map[string]string
}

type defaultAZIDTranslator struct {
	ec2Client services.EC2

	// zoneCache maps an account scope to its zoneMapping. Availability zone names are only
	// meaningful within an account, so entries for different accounts must never be shared. The
	// scope is empty for the cluster's own account and the assumed role ARN otherwise.
	zoneCache      *cache.Expiring
	zoneCacheMutex sync.RWMutex

	logger logr.Logger
}

func (t *defaultAZIDTranslator) TranslateAZName(ctx context.Context, assumeRoleArn string, externalId string, srcZoneName string) (*string, error) {
	srcZones, err := t.fetchZoneMapping(ctx, t.ec2Client, "")
	if err != nil {
		return nil, err
	}
	zoneID, exists := srcZones.idByName[srcZoneName]
	if !exists {
		t.logger.Info("unable to resolve availability zone ID", "zoneName", srcZoneName)
		return nil, nil
	}

	assumedRoleEC2, err := t.ec2Client.AssumeRole(ctx, assumeRoleArn, externalId)
	if err != nil {
		return nil, err
	}
	dstZones, err := t.fetchZoneMapping(ctx, assumedRoleEC2, assumeRoleArn)
	if err != nil {
		return nil, err
	}
	zoneName, exists := dstZones.nameByID[zoneID]
	if !exists {
		t.logger.Info("unable to resolve availability zone name", "zoneID", zoneID, "scope", assumeRoleArn)
		return nil, nil
	}
	return &zoneName, nil
}

// fetchZoneMapping returns the availability zones visible to ec2Client, cached under scope.
// The zones of an account are fetched in a single unfiltered call so that a zone missing from the
// mapping is known to be unresolvable without issuing another call for it.
func (t *defaultAZIDTranslator) fetchZoneMapping(ctx context.Context, ec2Client services.EC2, scope string) (*zoneMapping, error) {
	if cachedMapping, exists := t.fetchZoneMappingFromCache(scope); exists {
		return cachedMapping, nil
	}

	resp, err := ec2Client.DescribeAvailabilityZonesWithContext(ctx, &ec2sdk.DescribeAvailabilityZonesInput{})
	if err != nil {
		return nil, err
	}

	mapping := &zoneMapping{
		idByName: make(map[string]string, len(resp.AvailabilityZones)),
		nameByID: make(map[string]string, len(resp.AvailabilityZones)),
	}
	for _, azInfo := range resp.AvailabilityZones {
		zoneName := awssdk.ToString(azInfo.ZoneName)
		zoneID := awssdk.ToString(azInfo.ZoneId)
		if zoneName == "" || zoneID == "" {
			continue
		}
		mapping.idByName[zoneName] = zoneID
		mapping.nameByID[zoneID] = zoneName
	}

	t.zoneCacheMutex.Lock()
	defer t.zoneCacheMutex.Unlock()
	t.zoneCache.Set(scope, mapping, defaultAZIDTranslationCacheTTL)
	return mapping, nil
}

func (t *defaultAZIDTranslator) fetchZoneMappingFromCache(scope string) (*zoneMapping, bool) {
	t.zoneCacheMutex.RLock()
	defer t.zoneCacheMutex.RUnlock()

	if rawCacheItem, exists := t.zoneCache.Get(scope); exists {
		return rawCacheItem.(*zoneMapping), true
	}
	return nil, false
}
