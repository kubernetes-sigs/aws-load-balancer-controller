package controller

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

type ResourceRecordSet struct {
	zoneid                   *string
	CurrentResourceRecordSet *route53.ResourceRecordSet
	DesiredResourceRecordSet *route53.ResourceRecordSet
}

type ResourceRecordSets []*ResourceRecordSet

// Returns a new route53.ResourceRecordSet based on the LoadBalancer provided.
func NewResourceRecordSet(lb *LoadBalancer) (*route53.ResourceRecordSet, error) {
	zoneId, err := route53svc.getZoneID(lb.LoadBalancer.DNSName)

	if err != nil {
		glog.Errorf("Unabled to locate zoneId for load balancer DNS %s.", lb.LoadBalancer.DNSName)
		return nil, err
	}
	desired := &route53.ResourceRecordSet{
		AliasTarget: &route53.AliasTarget{
			DNSName:              lb.LoadBalancer.DNSName,
			HostedZoneId:         zoneId.Id,
			EvaluateTargetHealth: aws.Bool(false),
		},
		Type: aws.String("A"),
		ResourceRecords: []*route53.ResourceRecord{
			{
				Value: lb.hostname,
			},
		},
	}

	return desired, nil
}

func (r *ResourceRecordSet) create(a *albIngress, lb *LoadBalancer) error {
	// attempt a delete first, if hostname doesn't exist, it'll return
	r.delete(a, lb)

	err := r.modify(lb, route53.RRTypeA, "UPSERT")
	if err != nil {
		return err
	}

	glog.Infof("%s: Successfully registered %s in Route53", a.Name(), *lb.hostname)
	return nil
}

func (r *ResourceRecordSet) delete(a *albIngress, lb *LoadBalancer) error {
	hostedZone := r.zoneid

	// Attempt record deletion
	params := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action:            aws.String("DELETE"),
					ResourceRecordSet: r.CurrentResourceRecordSet,
				},
			},
		},
		HostedZoneId: hostedZone,
	}

	if noop {
		return nil
	}

	_, err := route53svc.svc.ChangeResourceRecordSets(params)
	if err != nil {
		return err
	}

	r.CurrentResourceRecordSet = nil
	glog.Infof("%s: Deleted %s from Route53", a.Name(), *lb.hostname)
	return nil
}

func (r *ResourceRecordSet) lookupRecord(hostname *string) (*route53.ResourceRecordSet, error) {
	hostedZone := r.zoneid

	item := cache.Get("r53rs " + *hostname)
	if item != nil {
		AWSCache.With(prometheus.Labels{"cache": "hostname", "action": "hit"}).Add(float64(1))
		return item.Value().(*route53.ResourceRecordSet), nil
	}
	AWSCache.With(prometheus.Labels{"cache": "hostname", "action": "miss"}).Add(float64(1))

	params := &route53.ListResourceRecordSetsInput{
		HostedZoneId:    hostedZone,
		StartRecordName: hostname,
		MaxItems:        aws.String("1"),
	}

	resp, err := route53svc.svc.ListResourceRecordSets(params)
	if err != nil {
		return nil, err
	}

	for _, record := range resp.ResourceRecordSets {
		if *record.Name == *hostname || *record.Name == *hostname+"." {
			cache.Set("r53rs "+*hostname, record, time.Minute*60)
			return record, nil
		}
	}
	return nil, errors.New(fmt.Sprintf("Failed to locate record set for %s in Route 53.", *hostname))
}

func (r *ResourceRecordSet) modify(lb *LoadBalancer, recordType string, action string) error {
	hostedZone := r.zoneid

	if action == "UPSERT" && !r.needsModification() {
		return nil
	}

	// Use all values from DesiredResourceRecordSet to run upsert against existing RecordSet in AWS.
	params := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name:        r.DesiredResourceRecordSet.ResourceRecords[0].Value,
						Type:        r.DesiredResourceRecordSet.Type,
						AliasTarget: r.DesiredResourceRecordSet.AliasTarget,
					},
				},
			},
			Comment: aws.String("Managed by Kubernetes"),
		},
		HostedZoneId: hostedZone, // Required
	}

	if noop {
		return nil
	}

	resp, err := route53svc.svc.ChangeResourceRecordSets(params)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "ChangeResourceRecordSets"}).Add(float64(1))
		glog.Errorf("There was an Error calling Route53 ChangeResourceRecordSets: %+v. Error: %s", resp.GoString(), err.Error())
		return err
	}

	// Upon success, ensure all possible updated attributes are updated in local Resource Record Set reference
	r.CurrentResourceRecordSet = r.DesiredResourceRecordSet

	return nil
}

// Determine whether there is a difference between CurrentResourceRecordSet and DesiredResourceRecordSet that requires
// a modification. Checks for whether hostname, record type, load balancer alias target (host name), and/or load
// balancer alias target's hosted zone are different.
func (r *ResourceRecordSet) needsModification() bool {
	currentHostName := r.CurrentResourceRecordSet.ResourceRecords[0].Value
	desiredHostName := r.DesiredResourceRecordSet.ResourceRecords[0].Value

	switch {
	// No resource record set currently exists; modification required.
	case r.CurrentResourceRecordSet == nil:
		return true
	// not sure if we need both conditions here.
	// Hostname has changed; modification required.
	case *currentHostName != *desiredHostName && *currentHostName != *desiredHostName+".":
		return true
	// Load balancer's hostname has changed; modification required.
	case *r.CurrentResourceRecordSet.AliasTarget.DNSName != *r.DesiredResourceRecordSet.AliasTarget.DNSName+".":
		return true
	// DNS record's resource type has changed; modification required.
	case *r.CurrentResourceRecordSet.Type != *r.DesiredResourceRecordSet.Type:
		return true
	// Load balancer's dns hosted zone has changed; modification required.
	case *r.CurrentResourceRecordSet.AliasTarget.HostedZoneId != *r.DesiredResourceRecordSet.AliasTarget.HostedZoneId:
		return true
	}
	return false
}
