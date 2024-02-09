package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sync"
)

// AZInfoProvider is responsible for provide AZ info.
type AZInfoProvider interface {
	FetchAZInfos(ctx context.Context, availabilityZoneIDs []string) (map[string]ec2sdk.AvailabilityZone, error)
}

// NewDefaultAZInfoProvider constructs new defaultAZInfoProvider.
func NewDefaultAZInfoProvider(ec2Client services.EC2, logger logr.Logger) *defaultAZInfoProvider {
	return &defaultAZInfoProvider{
		ec2Client:        ec2Client,
		azInfoCache:      make(map[string]ec2sdk.AvailabilityZone),
		azInfoCacheMutex: sync.RWMutex{},
		logger:           logger,
	}
}

var _ AZInfoProvider = &defaultAZInfoProvider{}

// default implementation for AZInfoProvider.
// AZ info for each zone is cached indefinitely.
type defaultAZInfoProvider struct {
	ec2Client        services.EC2
	azInfoCache      map[string]ec2sdk.AvailabilityZone
	azInfoCacheMutex sync.RWMutex

	logger logr.Logger
}

func (p *defaultAZInfoProvider) FetchAZInfos(ctx context.Context, availabilityZoneIDs []string) (map[string]ec2sdk.AvailabilityZone, error) {
	azInfoByAZID := p.fetchAZInfosFromCache(availabilityZoneIDs)
	azIDsWithoutAZInfo := computeAZIDsWithoutAZInfo(availabilityZoneIDs, azInfoByAZID)
	if len(azIDsWithoutAZInfo) == 0 {
		return azInfoByAZID, nil
	}

	azInfoByAZIDViaAWS, err := p.fetchAZInfosFromAWS(ctx, azIDsWithoutAZInfo)
	if err != nil {
		return nil, err
	}
	p.saveAZInfosToCache(azInfoByAZIDViaAWS)
	for azID, azInfo := range azInfoByAZIDViaAWS {
		azInfoByAZID[azID] = azInfo
	}

	azIDsWithoutAZInfo = computeAZIDsWithoutAZInfo(availabilityZoneIDs, azInfoByAZID)
	if len(azIDsWithoutAZInfo) > 0 {
		// NOTE: this branch should never be triggered as fetchAZInfosFromAWS will error out first if some AZ is not found.
		// however, we still add this check to not depend on this specific AWS API behavior.
		return nil, errors.Errorf("cannot resolve AZ info for AZs: %v", azIDsWithoutAZInfo)
	}
	return azInfoByAZID, nil
}

func (p *defaultAZInfoProvider) fetchAZInfosFromCache(availabilityZoneIDs []string) map[string]ec2sdk.AvailabilityZone {
	p.azInfoCacheMutex.RLock()
	defer p.azInfoCacheMutex.RUnlock()

	azInfoByAZID := make(map[string]ec2sdk.AvailabilityZone)
	for _, azID := range availabilityZoneIDs {
		if azInfo, exists := p.azInfoCache[azID]; exists {
			azInfoByAZID[azID] = azInfo
		}
	}
	return azInfoByAZID
}

func (p *defaultAZInfoProvider) saveAZInfosToCache(azInfoByAZID map[string]ec2sdk.AvailabilityZone) {
	p.azInfoCacheMutex.Lock()
	defer p.azInfoCacheMutex.Unlock()

	for azID, azInfo := range azInfoByAZID {
		p.azInfoCache[azID] = azInfo
	}
}

// fetchAZInfosFromAWS will fetch AZ info from AWS API.
// the availabilityZoneIDs shouldn't be empty.
func (p *defaultAZInfoProvider) fetchAZInfosFromAWS(ctx context.Context, availabilityZoneIDs []string) (map[string]ec2sdk.AvailabilityZone, error) {
	req := &ec2sdk.DescribeAvailabilityZonesInput{
		ZoneIds: awssdk.StringSlice(availabilityZoneIDs),
	}
	resp, err := p.ec2Client.DescribeAvailabilityZonesWithContext(ctx, req)
	if err != nil {
		return nil, err
	}
	azInfoByAZID := make(map[string]ec2sdk.AvailabilityZone)
	for _, azInfo := range resp.AvailabilityZones {
		azInfoByAZID[awssdk.StringValue(azInfo.ZoneId)] = *azInfo
	}
	return azInfoByAZID, nil
}

// computeAZIDsWithoutAZInfo computes az IDs that don't have az Info.
func computeAZIDsWithoutAZInfo(availabilityZoneIDs []string, azInfoByAZID map[string]ec2sdk.AvailabilityZone) []string {
	azIDsWithoutAZInfo := make([]string, 0, len(availabilityZoneIDs)-len(azInfoByAZID))
	for _, azID := range availabilityZoneIDs {
		if _, ok := azInfoByAZID[azID]; !ok {
			azIDsWithoutAZInfo = append(azIDsWithoutAZInfo, azID)
		}
	}
	return azIDsWithoutAZInfo
}
