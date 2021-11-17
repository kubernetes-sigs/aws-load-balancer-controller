package networking

import (
	"context"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

const defaultVPCInfoCacheTTL = 10 * time.Minute

// VPCInfoProvider is responsible for providing VPC info.
type VPCInfoProvider interface {
	FetchVPCInfo(ctx context.Context, vpcID string) (*ec2sdk.Vpc, error)
}

// NewDefaultVPCInfoProvider constructs new defaultVPCInfoProvider.
func NewDefaultVPCInfoProvider(ec2Client services.EC2, logger logr.Logger) *defaultVPCInfoProvider {
	return &defaultVPCInfoProvider{
		ec2Client:         ec2Client,
		vpcInfoCache:      cache.NewExpiring(),
		vpcInfoCacheMutex: sync.RWMutex{},
		vpcInfoCacheTTL:   defaultVPCInfoCacheTTL,
		logger:            logger,
	}
}

var _ VPCInfoProvider = &defaultVPCInfoProvider{}

// default implementation for VPCInfoProvider.
type defaultVPCInfoProvider struct {
	ec2Client         services.EC2
	vpcInfoCache      *cache.Expiring
	vpcInfoCacheMutex sync.RWMutex
	vpcInfoCacheTTL   time.Duration

	logger logr.Logger
}

func (p *defaultVPCInfoProvider) FetchVPCInfo(ctx context.Context, vpcID string) (*ec2sdk.Vpc, error) {
	if vpcInfo := p.fetchVPCInfoFromCache(); vpcInfo != nil {
		return vpcInfo, nil
	}

	// Fetch VPC info from the AWS API and cache response before returning.
	vpcInfo, err := p.fetchVPCInfoFromAWS(ctx, vpcID)
	if err != nil {
		return nil, err
	}
	p.saveVPCInfoToCache(vpcInfo)

	return vpcInfo, nil
}

func (p *defaultVPCInfoProvider) fetchVPCInfoFromCache() *ec2sdk.Vpc {
	p.vpcInfoCacheMutex.RLock()
	defer p.vpcInfoCacheMutex.RUnlock()

	if rawCacheItem, exists := p.vpcInfoCache.Get("vpcInfo"); exists {
		return rawCacheItem.(*ec2sdk.Vpc)
	}

	return nil
}

func (p *defaultVPCInfoProvider) saveVPCInfoToCache(vpcInfo *ec2sdk.Vpc) {
	p.vpcInfoCacheMutex.Lock()
	defer p.vpcInfoCacheMutex.Unlock()

	p.vpcInfoCache.Set("vpcInfo", vpcInfo, p.vpcInfoCacheTTL)
}

// fetchVPCInfoFromAWS will fetch VPC info from the AWS API.
func (p *defaultVPCInfoProvider) fetchVPCInfoFromAWS(ctx context.Context, vpcID string) (*ec2sdk.Vpc, error) {
	req := &ec2sdk.DescribeVpcsInput{
		VpcIds: []*string{awssdk.String(vpcID)},
	}
	resp, err := p.ec2Client.DescribeVpcsWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Vpcs[0], nil
}
