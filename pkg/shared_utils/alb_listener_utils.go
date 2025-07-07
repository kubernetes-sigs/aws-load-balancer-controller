package shared_utils

import (
	"context"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

const (
	defaultTrustStoreARNCacheTTL = 10 * time.Minute
)

var (
	trustStoreARNCache      *cache.Expiring
	trustStoreARNCacheMutex sync.RWMutex
)

func init() {
	trustStoreARNCache = cache.NewExpiring()
}

// GetTrustStoreArnFromName retrieves trust store ARNs for the given names, using cache where possible
func GetTrustStoreArnFromName(ctx context.Context, elbv2Client services.ELBV2, trustStoreNames []string) (map[string]*string, error) {
	if len(trustStoreNames) == 0 {
		return nil, nil
	}

	tsNameAndArnMap := make(map[string]*string, len(trustStoreNames))
	var namesToLookup []string

	// Check cache first
	trustStoreARNCacheMutex.RLock()
	for _, tsName := range trustStoreNames {
		if cachedItem, exists := trustStoreARNCache.Get(tsName); exists {
			tsNameAndArnMap[tsName] = cachedItem.(*string)
		} else {
			namesToLookup = append(namesToLookup, tsName)
		}
	}
	trustStoreARNCacheMutex.RUnlock()

	// If all names were in cache, return immediately
	if len(namesToLookup) == 0 {
		return tsNameAndArnMap, nil
	}

	// Look up the remaining names from AWS API
	req := &elbv2sdk.DescribeTrustStoresInput{
		Names: namesToLookup,
	}
	trustStores, err := elbv2Client.DescribeTrustStoresWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(trustStores.TrustStores) == 0 {
		return nil, errors.Errorf("couldn't find TrustStores with names %v", namesToLookup)
	}

	// Map API response to result and update cache
	trustStoreARNCacheMutex.Lock()
	defer trustStoreARNCacheMutex.Unlock()
	for _, tsName := range namesToLookup {
		found := false
		for _, ts := range trustStores.TrustStores {
			if tsName == awssdk.ToString(ts.Name) {
				tsNameAndArnMap[tsName] = ts.TrustStoreArn
				// Update cache
				trustStoreARNCache.Set(tsName, ts.TrustStoreArn, defaultTrustStoreARNCacheTTL)
				found = true
				break
			}
		}
		if !found {
			return nil, errors.Errorf("couldn't find TrustStore with name %v", tsName)
		}
	}

	return tsNameAndArnMap, nil
}
