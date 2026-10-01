package services

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/provider"
)

const (
	hostedZonesCacheKey = "hostedZones"
	// the hosted zones with their IDs will be cached for 5 minutes
	defaultHostedZonesCacheTTL = 5 * time.Minute
)

type Route53 interface {
	ChangeRecordsWithContext(ctx context.Context, input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error)
	GetHostedZoneID(ctx context.Context, domain string) (*string, error)
	GetPublicHostedZoneID(ctx context.Context, domain string) (*string, error)
}

func NewRoute53(awsClientsProvider provider.AWSClientsProvider) Route53 {
	return &route53Client{
		awsClientsProvider:  awsClientsProvider,
		hostedZonesCache:    cache.NewExpiring(),
		hostedZonesCacheTTL: defaultHostedZonesCacheTTL,
	}
}

type route53Client struct {
	awsClientsProvider  provider.AWSClientsProvider
	hostedZonesCache    *cache.Expiring
	hostedZonesCacheTTL time.Duration
}

func (c *route53Client) ChangeRecordsWithContext(ctx context.Context, input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	client, err := c.awsClientsProvider.GetRoute53Client(ctx, "ChangeResourceRecordSetsInput")
	if err != nil {
		return &route53.ChangeResourceRecordSetsOutput{}, err
	}

	resp, err := client.ChangeResourceRecordSets(ctx, input)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *route53Client) GetHostedZoneID(ctx context.Context, domain string) (*string, error) {
	zones, err := c.listHostedZones(ctx)
	if err != nil {
		return nil, err
	}

	if bestID := findHostedZoneID(zones, domain, false); bestID != nil {
		return bestID, nil
	}

	return nil, fmt.Errorf("no hosted zone found for validation records")
}

// GetPublicHostedZoneID skips private zones: Amazon-issued ACM certificates are
// validated over public DNS, so a validation record in a private zone (e.g. the
// most-specific match in split-horizon Route 53) leaves the cert in PENDING_VALIDATION.
func (c *route53Client) GetPublicHostedZoneID(ctx context.Context, domain string) (*string, error) {
	zones, err := c.listHostedZones(ctx)
	if err != nil {
		return nil, err
	}

	if bestID := findHostedZoneID(zones, domain, true); bestID != nil {
		return bestID, nil
	}

	return nil, fmt.Errorf("no public Route 53 hosted zone found for %q", domain)
}

// findHostedZoneID returns the nearest-ancestor hosted zone (longest matching suffix).
func findHostedZoneID(zones []types.HostedZone, domain string, publicOnly bool) *string {
	recParts := strings.Split(domain, ".")

	var bestID *string
	bestLen := -1
	for _, zone := range zones {
		if publicOnly && zone.Config != nil && zone.Config.PrivateZone {
			continue
		}
		zoneParts := strings.Split(strings.TrimRight(*zone.Name, "."), ".")
		if len(zoneParts) > len(recParts) {
			continue
		}
		if slices.Equal(recParts[len(recParts)-len(zoneParts):], zoneParts) && len(zoneParts) > bestLen {
			bestLen = len(zoneParts)
			bestID = zone.Id
		}
	}

	return bestID
}

func (c *route53Client) listHostedZones(ctx context.Context) ([]types.HostedZone, error) {
	if rawCacheItem, ok := c.hostedZonesCache.Get(hostedZonesCacheKey); ok {
		return rawCacheItem.([]types.HostedZone), nil
	}

	client, err := c.awsClientsProvider.GetRoute53Client(ctx, "ChangeResourceRecordSetsInput")
	if err != nil {
		return nil, err
	}

	reqList := &route53.ListHostedZonesInput{}
	respList, err := client.ListHostedZones(ctx, reqList)
	if err != nil {
		return nil, err
	}

	c.hostedZonesCache.Set(hostedZonesCacheKey, respList.HostedZones, c.hostedZonesCacheTTL)
	return respList.HostedZones, nil
}
