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

type VPCInfo ec2sdk.Vpc

// AssociatedIPv4CIDRs computes associated IPv4CIDRs for VPC.
func (vpc *VPCInfo) AssociatedIPv4CIDRs() []string {
	var ipv4CIDRs []string
	for _, cidr := range vpc.CidrBlockAssociationSet {
		if awssdk.StringValue(cidr.CidrBlockState.State) != ec2sdk.VpcCidrBlockStateCodeAssociated {
			continue
		}
		ipv4CIDRs = append(ipv4CIDRs, awssdk.StringValue(cidr.CidrBlock))
	}
	return ipv4CIDRs
}

// AssociatedIPv6CIDRs computes associated IPv6CIDRs for VPC.
func (vpc *VPCInfo) AssociatedIPv6CIDRs() []string {
	var ipv6CIDRs []string
	for _, cidr := range vpc.Ipv6CidrBlockAssociationSet {
		if awssdk.StringValue(cidr.Ipv6CidrBlockState.State) != ec2sdk.VpcCidrBlockStateCodeAssociated {
			continue
		}
		ipv6CIDRs = append(ipv6CIDRs, awssdk.StringValue(cidr.Ipv6CidrBlock))
	}
	return ipv6CIDRs
}

type FetchVPCInfoOptions struct {
	// whether to ignore cache and reload VPC Info from AWS directly.
	ReloadIgnoringCache bool
}

// ApplyOptions applies FetchVPCInfoOption options
func (opts *FetchVPCInfoOptions) ApplyOptions(options ...FetchVPCInfoOption) {
	for _, option := range options {
		option(opts)
	}
}

type FetchVPCInfoOption func(opts *FetchVPCInfoOptions)

// FetchVPCInfoWithoutCache is an option that sets the ReloadIgnoringCache to true.
func FetchVPCInfoWithoutCache() FetchVPCInfoOption {
	return func(opts *FetchVPCInfoOptions) {
		opts.ReloadIgnoringCache = true
	}
}

// VPCInfoProvider is responsible for providing VPC info.
type VPCInfoProvider interface {
	FetchVPCInfo(ctx context.Context, vpcID string, opts ...FetchVPCInfoOption) (VPCInfo, error)
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

// FetchVPCInfo fetches VPC info for vpcID.
func (p *defaultVPCInfoProvider) FetchVPCInfo(ctx context.Context, vpcID string, opts ...FetchVPCInfoOption) (VPCInfo, error) {
	fetchOpts := FetchVPCInfoOptions{
		ReloadIgnoringCache: false,
	}
	fetchOpts.ApplyOptions(opts...)

	if !fetchOpts.ReloadIgnoringCache {
		if vpcInfo, exists := p.fetchVPCInfoFromCache(vpcID); exists {
			return vpcInfo, nil
		}
	}

	vpcInfo, err := p.fetchVPCInfoFromAWS(ctx, vpcID)
	if err != nil {
		return VPCInfo{}, err
	}
	p.saveVPCInfoToCache(vpcID, vpcInfo)
	return vpcInfo, nil
}

// fetchVPCInfoFromCache fetches VPC info for vpcID from cache.
func (p *defaultVPCInfoProvider) fetchVPCInfoFromCache(vpcID string) (VPCInfo, bool) {
	p.vpcInfoCacheMutex.RLock()
	defer p.vpcInfoCacheMutex.RUnlock()

	if rawCacheItem, exists := p.vpcInfoCache.Get(vpcID); exists {
		return rawCacheItem.(VPCInfo), true
	}
	return VPCInfo{}, false
}

// saveVPCInfoToCache saves VPC info for vpcID into cache.
func (p *defaultVPCInfoProvider) saveVPCInfoToCache(vpcID string, vpcInfo VPCInfo) {
	p.vpcInfoCacheMutex.Lock()
	defer p.vpcInfoCacheMutex.Unlock()

	p.vpcInfoCache.Set(vpcID, vpcInfo, p.vpcInfoCacheTTL)
}

// fetchVPCInfoFromAWS will fetch VPC info from the AWS API.
func (p *defaultVPCInfoProvider) fetchVPCInfoFromAWS(ctx context.Context, vpcID string) (VPCInfo, error) {
	req := &ec2sdk.DescribeVpcsInput{
		VpcIds: []*string{awssdk.String(vpcID)},
	}
	resp, err := p.ec2Client.DescribeVpcsWithContext(ctx, req)
	if err != nil {
		return VPCInfo{}, err
	}

	return VPCInfo(*resp.Vpcs[0]), nil
}
