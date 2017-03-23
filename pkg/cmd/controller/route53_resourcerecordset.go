package controller

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

type ResourceRecordSet struct {
	ZoneId                   *string
	CurrentResourceRecordSet *route53.ResourceRecordSet
	DesiredResourceRecordSet *route53.ResourceRecordSet
}

type ResourceRecordSets []*ResourceRecordSet

// Returns a new route53.ResourceRecordSet based on the LoadBalancer provided.
func NewResourceRecordSet(hostname *string) (*ResourceRecordSet, error) {
	zoneId, err := route53svc.getZoneID(hostname)

	if err != nil {
		glog.Errorf("Unabled to locate ZoneId for %s.", hostname)
		return nil, err
	}

	name := *hostname
	if !strings.HasPrefix(*hostname, ".") {
		name = *hostname + "."
	}
	record := &ResourceRecordSet{
		DesiredResourceRecordSet: &route53.ResourceRecordSet{
			AliasTarget: &route53.AliasTarget{
				EvaluateTargetHealth: aws.Bool(false),
			},
			Name: aws.String(name),
			Type: aws.String("A"),
		},
		ZoneId: zoneId.Id,
	}

	return record, nil
}

func (r *ResourceRecordSet) create(a *albIngress, lb *LoadBalancer) error {
	// If a record pre-exists, delete it.
	existing := r.existingRecord(lb)
	if existing != nil {
		r.CurrentResourceRecordSet = existing
		r.delete(a, lb)
	}

	err := r.modify(lb, route53.RRTypeA, "UPSERT")
	if err != nil {
		return err
	}

	glog.Infof("%s: Successfully registered %s in Route53", a.Name(), *lb.hostname)
	return nil
}

func (r *ResourceRecordSet) delete(a *ALBIngress, lb *LoadBalancer) error {
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
		HostedZoneId: r.ZoneId,
	}

	_, err := route53svc.svc.ChangeResourceRecordSets(params)
	if err != nil {
		return err
	}

	r.CurrentResourceRecordSet = nil
	glog.Infof("%s: Deleted %s from Route53", a.Name(), *lb.hostname)
	return nil
}

func (r *ResourceRecordSet) modify(lb *LoadBalancer, recordType string, action string) error {
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
						Name:        r.DesiredResourceRecordSet.Name,
						Type:        r.DesiredResourceRecordSet.Type,
						AliasTarget: r.DesiredResourceRecordSet.AliasTarget,
					},
				},
			},
			Comment: aws.String("Managed by Kubernetes"),
		},
		HostedZoneId: r.ZoneId, // Required
	}

	resp, err := route53svc.svc.ChangeResourceRecordSets(params)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "ChangeResourceRecordSets"}).Add(float64(1))
		glog.Errorf("There was an Error calling Route53 ChangeResourceRecordSets: %+v. Error: %s", resp.GoString(), err.Error())
		return err
	}

	// TODO, wait for Status != PENDING?
	// (*route53.ChangeResourceRecordSetsOutput)(0xc42086e448)({
	//   ChangeInfo: {
	//     Comment: "Managed by Kubernetes",
	//     Id: "/change/C32J02SERDPTFA",
	//     Status: "PENDING",
	//     SubmittedAt: 2017-03-23 14:35:08.018 +0000 UTC
	//   }
	// })

	// Upon success, ensure all possible updated attributes are updated in local Resource Record Set reference
	r.CurrentResourceRecordSet = r.DesiredResourceRecordSet

	return nil
}

func (r *ResourceRecordSet) existingRecord(lb *LoadBalancer) *route53.ResourceRecordSet {
	// Lookup zone for hostname. Error is returned when zone cannot be found, a result of the
	// hostname not existing.
	zone, err := route53svc.getZoneID(lb.hostname)
	if err != nil {
		return nil
	}

	// If zone was resolved, then host exists. Return the respective route53.ResourceRecordSet.
	rrs, err := route53svc.describeResourceRecordSets(zone.Id, lb.hostname)
	if err != nil {
		return nil
	}
	return rrs
}

// Determine whether there is a difference between CurrentResourceRecordSet and DesiredResourceRecordSet that requires
// a modification. Checks for whether hostname, record type, load balancer alias target (host name), and/or load
// balancer alias target's hosted zone are different.
func (r *ResourceRecordSet) needsModification() bool {
	switch {
	// No resource record set currently exists; modification required.
	case r.CurrentResourceRecordSet == nil:
		return true
	// not sure if we need both conditions here.
	// Hostname has changed; modification required.
	case *r.CurrentResourceRecordSet.Name != *r.DesiredResourceRecordSet.Name:
		return true
	// Load balancer's hostname has changed; modification required.
	case *r.CurrentResourceRecordSet.AliasTarget.DNSName != *r.DesiredResourceRecordSet.AliasTarget.DNSName:
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

func (r *ResourceRecordSet) PopulateFromLoadBalancer(lb *elbv2.LoadBalancer) {
	r.DesiredResourceRecordSet.AliasTarget.DNSName = aws.String(*lb.DNSName + ".")
	r.DesiredResourceRecordSet.AliasTarget.HostedZoneId = lb.CanonicalHostedZoneId
}
