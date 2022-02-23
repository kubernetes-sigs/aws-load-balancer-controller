package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sync"
	"time"
)

// ElasticIPAddressManager is an abstraction around EC2's Elastic IP address API.
type ElasticIPAddressManager interface {
	// FetchEIPInfosByRequest will fetch ElasticIPAddressInfo with raw DescribeAddressesInput request.
	FetchEIPInfosByRequest(ctx context.Context, req *ec2sdk.DescribeAddressesInput) (map[string]ElasticIPAddressInfo, error)
}

// NewDefaultElasticIPAddressManager constructs new defaultElasticIPAddressManager.
func NewDefaultElasticIPAddressManager(ec2Client services.EC2, logger logr.Logger) *defaultElasticIPAddressManager {
	return &defaultElasticIPAddressManager{
		ec2Client: ec2Client,
		logger:    logger,

		eipInfoCache:      cache.NewExpiring(),
		eipInfoCacheMutex: sync.RWMutex{},
		eipInfoCacheTTL:   defaultSGInfoCacheTTL,
	}
}

var _ ElasticIPAddressManager = &defaultElasticIPAddressManager{}

// default implementation for ElasticIPAddressManager
type defaultElasticIPAddressManager struct {
	ec2Client services.EC2
	logger    logr.Logger

	eipInfoCache      *cache.Expiring
	eipInfoCacheMutex sync.RWMutex
	eipInfoCacheTTL   time.Duration
}

func (m *defaultElasticIPAddressManager) FetchEIPInfosByRequest(ctx context.Context, req *ec2sdk.DescribeAddressesInput) (map[string]ElasticIPAddressInfo, error) {
	eipInfosByID, err := m.fetchEIPInfosFromAWS(ctx, req)
	if err != nil {
		return nil, err
	}
	m.saveEIPInfosToCache(eipInfosByID)
	return eipInfosByID, nil
}

func (m *defaultElasticIPAddressManager) saveEIPInfosToCache(eipInfoByID map[string]ElasticIPAddressInfo) {
	m.eipInfoCacheMutex.Lock()
	defer m.eipInfoCacheMutex.Unlock()

	for eipID, eipInfo := range eipInfoByID {
		m.eipInfoCache.Set(eipID, eipInfo, m.eipInfoCacheTTL)
	}
}

func (m *defaultElasticIPAddressManager) clearEIPInfosFromCache(eipID string) {
	m.eipInfoCache.Delete(eipID)
}

func (m *defaultElasticIPAddressManager) fetchEIPInfosFromAWS(ctx context.Context, req *ec2sdk.DescribeAddressesInput) (map[string]ElasticIPAddressInfo, error) {
	resp, err := m.ec2Client.DescribeAddressesWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	eipInfoByID := make(map[string]ElasticIPAddressInfo, len(resp.Addresses))
	for _, eip := range resp.Addresses {
		eipID := awssdk.StringValue(eip.AllocationId)
		eipInfo := NewRawElasticIPAddressInfo(eip)
		eipInfoByID[eipID] = eipInfo
	}
	return eipInfoByID, nil
}
