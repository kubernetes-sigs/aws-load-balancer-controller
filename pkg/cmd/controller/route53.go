package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/pkg/errors"
)

type Route53 struct {
	svc route53iface.Route53API
}

func newRoute53(awsconfig *aws.Config) *Route53 {
	awsSession, err := session.NewSession(awsconfig)
	if err != nil {
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "NewSession"}).Add(float64(1))
		return nil
	}

	if AWSDebug {
		awsSession.Handlers.Send.PushFront(func(r *request.Request) {
			// Log every request made and its payload
			glog.Infof("Request: %s/%s, Payload: %s",
				r.ClientInfo.ServiceName, r.Operation, r.Params)
		})
	}

	r53 := Route53{
		svc: route53.New(awsSession),
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

// getZoneID looks for the Route53 zone ID of the hostname passed to it
func (r *Route53) getZoneID(hostname *string) (*route53.HostedZone, error) {
	if hostname == nil {
		return nil, errors.Errorf("Requested zoneID %s is invalid.", hostname)
	}

	zone, err := r.getDomain(*hostname) // involves witchcraft
	if err != nil {
		return nil, err
	}

	item := cache.Get(*zone)
	if item != nil {
		AWSCache.With(prometheus.Labels{"cache": "zone", "action": "hit"}).Add(float64(1))
		return item.Value().(*route53.HostedZone), nil
	}
	AWSCache.With(prometheus.Labels{"cache": "zone", "action": "miss"}).Add(float64(1))

	// glog.Infof("Fetching Zones matching %s", *zone)
	resp, err := r.svc.ListHostedZonesByName(
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
			cache.Set(*zone, i, time.Minute*60)
			return i, nil
		}
	}
	AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "getZoneID"}).Add(float64(1))
	return nil, fmt.Errorf("Unable to find the zone: %s", *zone)
}

func (r *Route53) describeResourceRecordSets(zoneID *string, hostname *string) (*route53.ResourceRecordSet, error) {
	params := &route53.ListResourceRecordSetsInput{
		HostedZoneId:    zoneID,
		MaxItems:        aws.String("1"),
		StartRecordName: hostname,
	}

	resp, err := route53svc.svc.ListResourceRecordSets(params)
	if err != nil {
		glog.Errorf("Failed to lookup resource record set %s, with request %s", hostname, params)
		return nil, err
	}

	return resp.ResourceRecordSets[0], nil
}
