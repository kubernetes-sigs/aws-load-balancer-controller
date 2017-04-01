package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/coreos-inc/alb-ingress-controller/pkg/metrics"
	"github.com/coreos-inc/alb-ingress-controller/pkg/config"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/pkg/errors"
	"github.com/karlseguin/ccache"
)

type Route53 struct {
	svc route53iface.Route53API
	cache *ccache.Cache
}

func NewRoute53(awsconfig *aws.Config, config *config.Config, cache *ccache.Cache) *Route53 {
	awsSession, err := session.NewSession(awsconfig)
	if err != nil {
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		metrics.AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "NewSession"}).Add(float64(1))
		return nil
	}

	awsSession.Handlers.Send.PushFront(func(r *request.Request) {
		metrics.AWSRequest.With(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name}).Add(float64(1))
		if config.AWSDebug {
			glog.Infof("Request: %s/%s, Payload: %s", r.ClientInfo.ServiceName, r.Operation, r.Params)
		}
	})

	r53 := Route53{
		svc: route53.New(awsSession),
		cache: cache,
	}
	return &r53
}

// GetDomain looks for the 'domain' of the hostname
// It assumes an ingress resource defined is only adding a single subdomain
// on to an AWS hosted zone. This may be too naive for Ticketmaster's use case
// TODO: review this approach.
func (r *Route53) GetDomain(hostname string) (*string, error) {
	hostname = strings.TrimSuffix(hostname, ".")
	domainParts := strings.Split(hostname, ".")
	if len(domainParts) < 2 {
		return nil, fmt.Errorf("%s hostname does not contain a domain", hostname)
	}

	domain := strings.Join(domainParts[len(domainParts)-2:], ".")

	return aws.String(strings.ToLower(domain)), nil
}

// getZoneID looks for the Route53 zone ID of the hostname passed to it
func (r *Route53) GetZoneID(hostname *string) (*route53.HostedZone, error) {
	if hostname == nil {
		return nil, errors.Errorf("Requested zoneID %s is invalid.", hostname)
	}

	zone, err := r.GetDomain(*hostname) // involves witchcraft
	if err != nil {
		return nil, err
	}

	item := r.cache.Get("r53zone " + *zone)
	if item != nil {
		metrics.AWSCache.With(prometheus.Labels{"cache": "zone", "action": "hit"}).Add(float64(1))
		return item.Value().(*route53.HostedZone), nil
	}
	metrics.AWSCache.With(prometheus.Labels{"cache": "zone", "action": "miss"}).Add(float64(1))

	// glog.Infof("Fetching Zones matching %s", *zone)
	resp, err := r.svc.ListHostedZonesByName(
		&route53.ListHostedZonesByNameInput{
			DNSName: zone,
		})

	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			metrics.AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "ListHostedZonesByName"}).Add(float64(1))
			return nil, fmt.Errorf("Error calling route53.ListHostedZonesByName: %s", awsErr.Code())
		}
		metrics.AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "ListHostedZonesByName"}).Add(float64(1))
		return nil, fmt.Errorf("Error calling route53.ListHostedZonesByName: %s", err)
	}

	if len(resp.HostedZones) == 0 {
		glog.Errorf("Unable to find the %s zone in Route53", *zone)
		metrics.AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "ListHostedZonesByName"}).Add(float64(1))
		return nil, fmt.Errorf("Zone not found")
	}

	for _, i := range resp.HostedZones {
		zoneName := strings.TrimSuffix(*i.Name, ".")
		if *zone == zoneName {
			// glog.Infof("Found DNS Zone %s with ID %s", zoneName, *i.Id)
			r.cache.Set("r53zone "+*zone, i, time.Minute*60)
			return i, nil
		}
	}
	metrics.AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "getZoneID"}).Add(float64(1))
	return nil, fmt.Errorf("Unable to find the zone: %s", *zone)
}

func (r *Route53) describeResourceRecordSets(zoneID *string, hostname *string) (*route53.ResourceRecordSet, error) {
	params := &route53.ListResourceRecordSetsInput{
		HostedZoneId:    zoneID,
		MaxItems:        aws.String("1"),
		StartRecordName: hostname,
	}

	resp, err := r.svc.ListResourceRecordSets(params)
	if err != nil {
		glog.Errorf("Failed to lookup resource record set %s, with request %v", *hostname, params)
		return nil, err
	}

	if len(resp.ResourceRecordSets) == 0 {
		return nil, fmt.Errorf("ListResourceRecordSets(%s, %s) returned an empty list", *zoneID, *hostname)
	}

	return resp.ResourceRecordSets[0], nil
}
