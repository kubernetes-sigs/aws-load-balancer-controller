package awsutil

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/karlseguin/ccache"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/pkg/errors"
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
	Svc route53iface.Route53API
}

var r53Cache = ccache.New(ccache.Configure())

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
		Svc: route53.New(awsSession),
	}
	return &r53
}

// getDomain looks for the 'domain' of the hostname
// It assumes an ingress resource defined is only adding a single subdomain
// on to an AWS hosted zone. This may be too naive for Ticketmaster's use case
// TODO: review this approach.
func (r *Route53) getDomain(hostname string) (*string, error) {
	hostname = strings.TrimSuffix(hostname, ".")
	domainParts := strings.Split(hostname, ".")
	if len(domainParts) < 2 {
		return nil, fmt.Errorf("%s hostname does not contain a domain", hostname)
	}

	domain := strings.Join(domainParts[len(domainParts)-2:], ".")

	return aws.String(strings.ToLower(domain)), nil
}

// GetZoneID looks for the Route53 zone ID of the hostname passed to it
func (r *Route53) GetZoneID(hostname *string) (*route53.HostedZone, error) {
	if hostname == nil {
		return nil, errors.Errorf("Requested zoneID %s is invalid.", *hostname)
	}

	zone, err := r.getDomain(*hostname) // involves witchcraft
	if err != nil {
		return nil, err
	}

	item := r53Cache.Get("r53zone " + *zone)
	if item != nil {
		AWSCache.With(prometheus.Labels{"cache": "zone", "action": "hit"}).Add(float64(1))
		return item.Value().(*route53.HostedZone), nil
	}
	AWSCache.With(prometheus.Labels{"cache": "zone", "action": "miss"}).Add(float64(1))

	// glog.Infof("Fetching Zones matching %s", *zone)
	resp, err := r.Svc.ListHostedZonesByName(
		&route53.ListHostedZonesByNameInput{
			DNSName: zone,
		})

	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "ListHostedZonesByName"}).Add(float64(1))
			return nil, fmt.Errorf("Error calling route53.ListHostedZonesByName: %s", awsErr.Code())
		}
		AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "ListHostedZonesByName"}).Add(float64(1))
		return nil, fmt.Errorf("Error calling route53.ListHostedZonesByName: %s", err)
	}

	if len(resp.HostedZones) == 0 {
		glog.Errorf("Unable to find the %s zone in Route53", *zone)
		AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "ListHostedZonesByName"}).Add(float64(1))
		return nil, fmt.Errorf("Zone not found")
	}

	for _, i := range resp.HostedZones {
		zoneName := strings.TrimSuffix(*i.Name, ".")
		if *zone == zoneName {
			// glog.Infof("Found DNS Zone %s with ID %s", zoneName, *i.Id)
			r53Cache.Set("r53zone "+*zone, i, time.Minute*60)
			return i, nil
		}
	}
	AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "GetZoneID"}).Add(float64(1))
	return nil, fmt.Errorf("Unable to find the zone: %s", *zone)
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
	}

	resp, err := r.Svc.ListResourceRecordSets(params)
	if err != nil {
		glog.Errorf("Failed to lookup resource record set %s, with request %v", *hostname, params)
		return nil, err
	}

	if len(resp.ResourceRecordSets) == 0 {
		return nil, fmt.Errorf("ListResourceRecordSets(%s, %s) returned an empty list", *zoneID, *hostname)
	}

	return resp.ResourceRecordSets[0], nil
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
