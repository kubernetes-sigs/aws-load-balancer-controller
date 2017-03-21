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
	name              *string
	zoneid            *string
	ResourceRecordSet *route53.ResourceRecordSet
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
					ResourceRecordSet: r.ResourceRecordSet,
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

	r.ResourceRecordSet = nil
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

	if action == "UPSERT" && !r.needsModification(lb) {
		return nil
	}

	// Need check if the record exists and remove it if it does in this changeset
	params := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: lb.hostname,
						Type: aws.String(recordType),
						AliasTarget: &route53.AliasTarget{
							DNSName:              lb.LoadBalancer.DNSName,
							EvaluateTargetHealth: aws.Bool(false),
							HostedZoneId:         lb.LoadBalancer.CanonicalHostedZoneId,
						},
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
	r.ResourceRecordSet, err = r.lookupRecord(lb.hostname)
	if err != nil {
		glog.Errorf("Unable to retrieve resource %s record after upsert", lb.hostname)
	}

	return nil
}

func (r *ResourceRecordSet) needsModification(lb *LoadBalancer) bool {

	switch {
	case r.ResourceRecordSet == nil:
		return true
	case *r.ResourceRecordSet.Name != *lb.hostname && *r.ResourceRecordSet.Name != *lb.hostname+".":
		return true
	case *r.ResourceRecordSet.AliasTarget.DNSName != *lb.LoadBalancer.DNSName+".":
		return true
	case *r.ResourceRecordSet.Type != "A":
		return true
	case *r.ResourceRecordSet.AliasTarget.HostedZoneId != *lb.LoadBalancer.CanonicalHostedZoneId:
		return true
	}
	return false
}
