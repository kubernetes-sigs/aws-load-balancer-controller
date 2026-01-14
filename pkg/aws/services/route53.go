package services

import (
	"context"
	"fmt"
	"slices"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
)

type Route53 interface {
	ChangeRecordsWithContext(ctx context.Context, input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error)
	GetHostedZoneId(ctx context.Context, domain string) (*string, error)
}

func NewRoute53(awsClientsProvider provider.AWSClientsProvider) Route53 {
	return &route53Client{
		awsClientsProvider: awsClientsProvider,
	}
}

type route53Client struct {
	awsClientsProvider provider.AWSClientsProvider
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

func (c *route53Client) GetHostedZoneId(ctx context.Context, domain string) (*string, error) {
	client, err := c.awsClientsProvider.GetRoute53Client(ctx, "ChangeResourceRecordSetsInput")
	if err != nil {
		return nil, err
	}

	// try first if we have an apex record
	req := &route53.ListHostedZonesByNameInput{
		DNSName: awssdk.String(domain),
	}
	resp, err := client.ListHostedZonesByName(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.HostedZones) == 1 {
		return resp.HostedZones[0].Id, nil
	}

	// otherwise list all and find the leftmost match
	reqList := &route53.ListHostedZonesInput{}
	respList, err := client.ListHostedZones(ctx, reqList)
	if err != nil {
		return nil, err
	}

	for _, zone := range respList.HostedZones {
		recParts := strings.Split(domain, ".")
		zoneParts := strings.Split(strings.TrimRight(*zone.Name, "."), ".")
		if slices.Equal(recParts[1:], zoneParts) { // if they have the exact same segments, ignoring the leftmost one of the record
			return zone.Id, nil
		}
	}

	return nil, fmt.Errorf("no hosted zone found for validation records")
}
