package aws

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/golang/sync/syncmap"
	"github.com/karlseguin/ccache"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// Route53 is our extension to AWS's route53.Route53
type Route53 struct {
	route53iface.Route53API
	cache APICache
}

// NewRoute53 returns a new Route53 based off of an AWS session
func NewRoute53(awsSession *session.Session) *Route53 {
	r53 := Route53{
		route53.New(awsSession),
		APICache{ccache.New(ccache.Configure())},
	}
	return &r53
}

var zoneSyncMap *syncmap.Map

func init() {
	zoneSyncMap = new(syncmap.Map)
}

// GetZoneID looks for the Route53 zone ID of the hostname passed to it. It iteratively looks up
// very possible domain combination the hosted zone could represent. The most qualified will always
// win. e.g. If your domain is 1.2.example.com and you have 2.example.com and example.com both as
// hosted zones Route 53, 2.example.com will always win.
func (r *Route53) GetZoneID(hostname *string) (*route53.HostedZone, error) {
	if hostname == nil {
		return nil, errors.Errorf("Requested zoneID %s is invalid.", *hostname)
	}

	hnFull := strings.TrimSuffix(*hostname, ".")
	hnParts := strings.Split(hnFull, ".")

	// loop through each hostname combo until a hosted zone is found.
	for i := 0; i < len(hnParts)-2; i++ {
		hnAttempt := strings.Join(hnParts[i+1:], ".")

		l, _ := zoneSyncMap.LoadOrStore(hnAttempt, new(sync.Mutex))
		lock := l.(*sync.Mutex)

		lock.Lock()
		defer lock.Unlock()

		if r.cache.Get("r53zoneErr"+hnAttempt) != nil {
			continue
		}

		item := r.cache.Get("r53zone" + hnAttempt)
		if item != nil {
			AWSCache.With(prometheus.Labels{"cache": "zone", "action": "hit"}).Add(float64(1))
			return item.Value().(*route53.HostedZone), nil
		}

		resp, err := r.ListHostedZonesByName(
			&route53.ListHostedZonesByNameInput{
				DNSName: &hnAttempt,
			})
		if err != nil {
			return nil, fmt.Errorf("Error calling route53.ListHostedZonesByName: %s", err)
		}

		for _, i := range resp.HostedZones {
			zoneName := strings.TrimSuffix(*i.Name, ".")
			if hnAttempt == zoneName {
				r.cache.Set("r53zone"+hnAttempt, i, time.Minute*60)
				return i, nil
			}
		}

		r.cache.Set("r53zoneErr"+hnAttempt, "fail", time.Minute*60)
	}

	return nil, fmt.Errorf("Unable to find the zone using any subset of hostname: %s", *hostname)
}

// Modify is the general way to interact with Route 53 Resource Record Sets. It handles create
// and modifications based on the input passed. It will verify the AWS DNS is propogated before
// returning.
func (r *Route53) Modify(in route53.ChangeResourceRecordSetsInput) error {
	o, err := r.ChangeResourceRecordSets(&in)
	if err != nil {
		return err
	}

	if err = r.WaitUntilResourceRecordSetsChangedWithContext(context.Background(), &route53.GetChangeInput{Id: o.ChangeInfo.Id}); err != nil {
		return fmt.Errorf("Failed Route 53 resource record set modification. Unable to verify DNS propagation. DNS: %s | Type: %s: %s",
			*in.ChangeBatch.Changes[0].ResourceRecordSet.Name,
			*in.ChangeBatch.Changes[0].ResourceRecordSet.Type,
			err)
	}

	return nil
}

// Delete removes a Route53 Resource Record Set from Route 53. When a route53.InvalidChangeBatch
// error is detected, it's considered a failure as the Route 53 record no longer exists in its
// current states (is likley already deleted). All other failures return an error.
func (r *Route53) Delete(in route53.ChangeResourceRecordSetsInput) error {
	if *in.ChangeBatch.Changes[0].Action != "DELETE" {
		return fmt.Errorf("Invalid action was passed to route53.delete. Action was %s; must be DELETE", *in.ChangeBatch.Changes[0].Action)
	}

	_, err := r.ChangeResourceRecordSets(&in)
	if err != nil && err.(awserr.Error).Code() != route53.ErrCodeInvalidChangeBatch {
		return err
	}

	return nil
}

// DescribeResourceRecordSets returns the route53.ResourceRecordSet for a zone & hostname
func (r *Route53) DescribeResourceRecordSets(zoneID *string, hostname *string) (*route53.ResourceRecordSet, error) {
	params := &route53.ListResourceRecordSetsInput{
		HostedZoneId:    zoneID,
		MaxItems:        aws.String("1"),
		StartRecordName: hostname,
		StartRecordType: aws.String(route53.RRTypeA),
	}

	resp, err := r.ListResourceRecordSets(params)
	if err != nil {
		return nil, err
	}

	if len(resp.ResourceRecordSets) == 0 {
		return nil, fmt.Errorf("ListResourceRecordSets(%s, %s) returned an empty list", *zoneID, *hostname)
	}

	for _, record := range resp.ResourceRecordSets {
		if *record.Type != route53.RRTypeCname && *record.Type != route53.RRTypeA {
			continue
		}

		if strings.HasPrefix(*record.Name, *hostname) {
			return record, nil
		}
	}

	return nil, fmt.Errorf("ListResourceRecordSets(%s, %s) did not return any valid records", *zoneID, *hostname)
}

// LookupExistingRecord returns the route53.ResourceRecordSet for a hostname
func LookupExistingRecord(hostname *string) *route53.ResourceRecordSet {
	// Lookup zone for hostname. Error is returned when zone cannot be found, a result of the
	// hostname not existing.
	zone, err := Route53svc.GetZoneID(hostname)
	if err != nil {
		return nil
	}

	// If zone was resolved, then host exists. Return the respective route53.ResourceRecordSet.
	rrs, err := Route53svc.DescribeResourceRecordSets(zone.Id, hostname)
	if err != nil {
		return nil
	}
	return rrs
}
