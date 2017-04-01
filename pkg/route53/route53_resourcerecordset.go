package controller

import (
	"strings"

	"github.com/coreos-inc/alb-ingress-controller/pkg/elbv2"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

type ResourceRecordSet struct {
	svc *Route53
	ZoneId                   *string
	CurrentResourceRecordSet *route53.ResourceRecordSet
	DesiredResourceRecordSet *route53.ResourceRecordSet
}

type ResourceRecordSets []*ResourceRecordSet

// Returns a new route53.ResourceRecordSet based on the LoadBalancer provided.
func NewResourceRecordSet(svc *Route53, hostname *string) (*ResourceRecordSet, error) {
	zoneId, err := svc.GetZoneID(hostname)

	if err != nil {
		glog.Errorf("Unabled to locate ZoneId for %s.", hostname)
		return nil, err
	}

	name := *hostname
	if !strings.HasPrefix(*hostname, ".") {
		name = *hostname + "."
	}
	record := &ResourceRecordSet{
		svc: svc,
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

func (r *ResourceRecordSet) Create(name string, lb *elbv2.LoadBalancer) error {
	// attempt a delete first, if hostname doesn't exist, it'll return
	r.Delete(name, lb)

	err := r.Modify(lb, route53.RRTypeA, "UPSERT")
	if err != nil {
		return err
	}

	glog.Infof("%s: Successfully registered %s in Route53", name, *lb.Hostname)
	return nil
}

func (r *ResourceRecordSet) Delete(name string, lb *elbv2.LoadBalancer) error {
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

	_, err := r.svc.svc.ChangeResourceRecordSets(params)
	if err != nil {
		return err
	}

	r.CurrentResourceRecordSet = nil
	glog.Infof("%s: Deleted %s from Route53", name, *lb.Hostname)
	return nil
}

func (r *ResourceRecordSet) Modify(lb *LoadBalancer, recordType string, action string) error {
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

	resp, err := r.svc.svc.ChangeResourceRecordSets(params)
	if err != nil {
		metrics.AWSErrorCount.With(prometheus.Labels{"service": "Route53", "request": "ChangeResourceRecordSets"}).Add(float64(1))
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
