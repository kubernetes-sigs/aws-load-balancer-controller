package controller

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

type ResourceRecordSet struct {
	name              *string
	zoneid            *string
	ResourceRecordSet *route53.ResourceRecordSet
}

func (r *Route53) UpsertRecord(a *albIngress, lb *LoadBalancer) error {
	// should do a better test
	// record, err := r.lookupRecord(a, lb.hostname)
	// if record != nil {
	// 	r.modifyRecord(lb, "DELETE")
	// }

	err := r.modifyRecord(lb, "UPSERT")
	if err != nil {
		glog.Infof("%s: Successfully registered %s in Route53", a.Name(), *lb.hostname)
	}
	return err
}

func (r *Route53) DeleteRecord(a *albIngress, lb *LoadBalancer) error {
	err := r.modifyRecord(lb, "DELETE")
	if err != nil {
		glog.Infof("%s: Successfully deleted %s from Route53", a.Name(), *lb.hostname)
	}
	return err
}

func (r *Route53) lookupRecord(a *albIngress, hostname *string) (*route53.ResourceRecordSet, error) {
	hostedZone, err := r.getZoneID(hostname)
	if err != nil {
		return nil, err
	}

	params := &route53.ListResourceRecordSetsInput{
		HostedZoneId:    hostedZone.Id,
		StartRecordName: hostname,
		MaxItems:        aws.String("1"),
	}

	resp, err := r.svc.ListResourceRecordSets(params)
	for _, record := range resp.ResourceRecordSets {
		if *record.Name == *hostname || *record.Name == *hostname+"." {
			return record, nil
		}
	}

	return nil, fmt.Errorf("%s: Unable to find record for %v", a.Name(), *hostname)
}

func (r *Route53) modifyRecord(lb *LoadBalancer, action string) error {
	hostedZone, err := r.getZoneID(lb.hostname)
	if err != nil {
		return err
	}

	// Need check if the record exists and remove it if it does in this changeset
	params := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: lb.hostname,
						Type: aws.String("A"),
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
		HostedZoneId: hostedZone.Id, // Required
	}

	// glog.Infof("Modify r53.ChangeResourceRecordSets ")
	if noop {
		return nil
	}

	resp, err := route53svc.svc.ChangeResourceRecordSets(params)
	if err != nil &&
		err.(awserr.Error).Code() != "InvalidChangeBatch" &&
		!strings.Contains(err.Error(), "Tried to delete resource record") &&
		!strings.Contains(err.Error(), "type='A'] but it was not found") {
		AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "ChangeResourceRecordSets"}).Add(float64(1))
		glog.Errorf("There was an Error calling Route53 ChangeResourceRecordSets: %+v. Error: %s", resp.GoString(), err.Error())
		return err
	}

	return nil
}
