package aga

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"sync"
	"time"

	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/hashicorp/golang-lru"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

// DNSResolver resolves load balancer DNS names to ARNs
type DNSResolver struct {
	elbv2Client services.ELBV2
	cache       *lru.Cache
	cacheMutex  sync.RWMutex
	ttl         time.Duration
}

type cacheEntry struct {
	arn      string
	expireAt time.Time
}

// NewDNSResolver creates a new DNSResolver
func NewDNSResolver(elbv2Client services.ELBV2) (*DNSResolver, error) {
	// AWS Global Accelerator has a quota of 420 endpoints per AWS account (can be increased)
	// Using 420 provides headroom while efficiently caching DNS-to-ARN resolutions
	cache, err := lru.New(420)
	if err != nil {
		return nil, err
	}

	return &DNSResolver{
		elbv2Client: elbv2Client,
		cache:       cache,
		ttl:         5 * time.Minute, // Default TTL of 5 minutes
	}, nil
}

// ResolveDNSToARN resolves a load balancer DNS name to an ARN
func (r *DNSResolver) ResolveDNSToARN(ctx context.Context, dnsName string) (string, error) {
	if dnsName == "" {
		return "", fmt.Errorf("empty DNS name")
	}

	// Check cache first
	r.cacheMutex.RLock()
	if value, found := r.cache.Get(dnsName); found {
		entry := value.(cacheEntry)
		// Check if the cache entry is still valid
		if time.Now().Before(entry.expireAt) {
			r.cacheMutex.RUnlock()
			return entry.arn, nil
		}
		// Entry has expired, remove from cache
		r.cache.Remove(dnsName)
	}
	r.cacheMutex.RUnlock()

	req := &elbv2sdk.DescribeLoadBalancersInput{}
	lbs, err := r.elbv2Client.DescribeLoadBalancersAsList(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to describe load balancers: %w", err)
	}
	if len(lbs) == 0 {
		return "", fmt.Errorf("no load balancers found")
	}
	arn := ""
	for _, lb := range lbs {
		if awssdk.ToString(lb.DNSName) == dnsName {
			arn = awssdk.ToString(lb.LoadBalancerArn)
			break
		}
	}
	if arn == "" {
		return "", fmt.Errorf("no load balancer found for dns %s", dnsName)
	}

	// Cache the result
	r.cacheMutex.Lock()
	r.cache.Add(dnsName, cacheEntry{
		arn:      arn,
		expireAt: time.Now().Add(r.ttl),
	})
	r.cacheMutex.Unlock()

	return arn, nil
}

// Ensure DNSResolver implements DNSResolverInterface
var _ DNSResolverInterface = (*DNSResolver)(nil)
