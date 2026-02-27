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
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
)

const (
	hostedZonesCacheKey = "hostedZones"
	// the hosted zones with their IDs will be cached for 5 minutes
	defaultHostedZonesCacheTTL = 5 * time.Minute
)

type Route53 interface {
	ChangeRecordsWithContext(ctx context.Context, input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error)
	GetHostedZoneID(ctx context.Context, domain string) (*string, error)
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

	recParts := strings.Split(domain, ".")

	// try first if we have an apex record
	for _, zone := range zones {
		zoneParts := strings.Split(strings.TrimRight(*zone.Name, "."), ".")
		if slices.Equal(recParts, zoneParts) {
			return zone.Id, nil
		}
	}

	// otherwise trim the leftmost part of the domain
	for _, zone := range zones {
		zoneParts := strings.Split(strings.TrimRight(*zone.Name, "."), ".")
		if slices.Equal(recParts[1:], zoneParts) {
			return zone.Id, nil
		}
	}

	return nil, fmt.Errorf("no hosted zone found for validation records")
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
