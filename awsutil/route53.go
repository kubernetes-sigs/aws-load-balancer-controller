package awsutil

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/golang/glog"
	"github.com/karlseguin/ccache"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Amount of time, in seconds, between each attempt to validate a created or modified record's
	// status has reached insyncR53DNSStatus state.
	validateSleepDuration int = 10
	// Maximum attempts should be made to validate a created or modified resource record set has
	// reached insyncR53DNSStatus state.
	maxValidateRecordAttempts int = 10
	// Status used to signify that resource record set that the changes have replicated to all Amazon
	// Route 53 DNS servers.
	insyncR53DNSStatus string = "INSYNC"
)

// Route53 is our extension to AWS's route53.Route53
type Route53 struct {
	Svc   route53iface.Route53API
	cache APICache
}

// NewRoute53 returns a new Route53 based off of an aws.Config
func NewRoute53(awsconfig *aws.Config) *Route53 {
	awsSession, err := session.NewSession(awsconfig)
	if err != nil {
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "NewSession"}).Add(float64(1))
		return nil
	}

	awsSession.Handlers.Send.PushFront(func(r *request.Request) {
		AWSRequest.With(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name}).Add(float64(1))
		if AWSDebug {
			glog.Infof("Request: %s/%s, Payload: %s", r.ClientInfo.ServiceName, r.Operation, r.Params)
		}
	})

	r53 := Route53{
		route53.New(awsSession),
		APICache{ccache.New(ccache.Configure()), },
	}
	return &r53
}

// GetZoneID looks for the Route53 zone ID of the hostname passed to it. It iteratively looks up
// very possible domain combination the hosted zone could represent. The most qualified will always
// win. e.g. If your domain is 1.2.example.com and you have 2.example.com and example.com both as
// hosted zones Route 53, 2.example.com will always win.
func (r *Route53) GetZoneID(hostname *string) (*route53.HostedZone, error) {
	if hostname == nil || r.cache.Get("r53zoneErr " + *hostname) != nil {
		return nil, errors.Errorf("Requested zoneID %s is invalid.", *hostname)
	}

	item := r.cache.Get("r53zone " + *hostname)
	if item != nil {
		AWSCache.With(prometheus.Labels{"cache": "zone", "action": "hit"}).Add(float64(1))
		return item.Value().(*route53.HostedZone), nil
	}
	AWSCache.With(prometheus.Labels{"cache": "zone", "action": "miss"}).Add(float64(1))

	hnFull := strings.TrimSuffix(*hostname, ".")
	hnParts := strings.Split(hnFull, ".")
	var err error
	var resp *route53.ListHostedZonesByNameOutput

	// loop through each hostname combo until a hosted zone is found.
	for i := 0; i < len(hnParts)-2; i++ {
		hnAttempt := strings.Join(hnParts[i+1:], ".")
		resp, err = r.Svc.ListHostedZonesByName(
			&route53.ListHostedZonesByNameInput{
				DNSName: &hnAttempt,
			})
		if err != nil {
			AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "ListHostedZonesByName"}).Add(float64(1))
			return nil, fmt.Errorf("Error calling route53.ListHostedZonesByName: %s", err)
		}

		for _, i := range resp.HostedZones {
			zoneName := strings.TrimSuffix(*i.Name, ".")
			if hnAttempt == zoneName {
				r.cache.Set("r53zone " + *hostname, i, time.Minute*60)
				return i, nil
			}
		}

	}

	AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "GetZoneID"}).Add(float64(1))
	r.cache.Set("r53zoneErr " + *hostname, "fail", time.Minute*60)
	return nil, fmt.Errorf("Unable to find the zone using any subset of hostname: %s", *hostname)
}

// Modify is the general way to interact with Route 53 Resource Record Sets. It handles create
// and modifications based on the input passed. It will verify the AWS DNS is propogated before
// returning.
func (r *Route53) Modify(in route53.ChangeResourceRecordSetsInput) error {
	o, err := r.Svc.ChangeResourceRecordSets(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "Route53", "request": "ChangeResourceRecordSets"}).Add(float64(1))
		return err
	}

	if ok := r.verifyRecordCreated(*o.ChangeInfo.Id); !ok {
		return fmt.Errorf("Failed Route 53 resource record set modification. Unable to verify DNS propagation. DNS: %s | Type: %s",
			*in.ChangeBatch.Changes[0].ResourceRecordSet.Name,
			*in.ChangeBatch.Changes[0].ResourceRecordSet.Type)
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

	_, err := r.Svc.ChangeResourceRecordSets(&in)
	if err != nil && err.(awserr.Error).Code() != route53.ErrCodeInvalidChangeBatch {
		AWSErrorCount.With(
			prometheus.Labels{"service": "Route53", "request": "ChangeResourceRecordSets"}).Add(float64(1))
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

	resp, err := r.Svc.ListResourceRecordSets(params)
	if err != nil {
		glog.Errorf("Failed to lookup resource record set %s, with request %v", *hostname, params)
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

// verifyRecordCreated ensures the ResourceRecordSet's desired state has been setup and reached
// RUNNING status.
func (r *Route53) verifyRecordCreated(changeID string) bool {
	created := false

	// Attempt to verify the existence of the deisred Resource Record Set up to as many times defined in
	// maxValidateRecordAttempts
	for i := 0; i < maxValidateRecordAttempts; i++ {
		time.Sleep(time.Duration(validateSleepDuration) * time.Second)
		in := &route53.GetChangeInput{
			Id: &changeID,
		}
		resp, _ := r.Svc.GetChange(in)
		status := *resp.ChangeInfo.Status
		if status != insyncR53DNSStatus {
			// Record does not exist, loop again.
			continue
		}
		// Record located. Set created to true and break from loop
		created = true
		break
	}

	return created
}
