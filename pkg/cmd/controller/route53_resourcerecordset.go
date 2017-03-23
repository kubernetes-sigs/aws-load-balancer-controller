package controller

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	validateSleepDuration     int    = 10
	maxValidateRecordAttempts int    = 30
	insyncR53DNSStatus        string = "INSYNC"
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
	existing := lookupExistingRecord(lb.hostname)
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

	// Verify that resource record set was created successfully, otherwise return error.
	success := r.verifyRecordCreated(*resp.ChangeInfo.Id)
	if !success {
		glog.Errorf("ResourceRecordSet for %s was unable to be successfully validated in Route53",
			r.DesiredResourceRecordSet.Name)
		return errors.New(fmt.Sprintf("ResourceRecordSet %s never validated.", r.DesiredResourceRecordSet.Name))
	}

	// TODO: Delete CurrentResourceRecordSet from r53 when necessary (e.g. a modification that changed the hostname).

	// Upon success, ensure all possible updated attributes are updated in local Resource Record Set reference
	r.CurrentResourceRecordSet = r.DesiredResourceRecordSet

	return nil
}

// Verify the ResourceRecordSet's desired state has been setup and reached RUNNING status.
func (r *ResourceRecordSet) verifyRecordCreated(changeID string) bool {
	created := false

	// Attempt to verify the existence of the deisred Resource Record Set up to as many times defined in
	// maxValidateRecordAttempts
	for i := 0; i < maxValidateRecordAttempts; i++ {
		time.Sleep(time.Duration(validateSleepDuration) * time.Second)
		params := &route53.GetChangeInput{
			Id: &changeID,
		}
		resp, _ := route53svc.svc.GetChange(params)
		status := *resp.ChangeInfo.Status
		if status != insyncR53DNSStatus {
			// Record does not exist, loop again.
			glog.Infof("%s was not located in Route 53. Attempt %d/%d.", r.DesiredResourceRecordSet.Name, i,
				maxValidateRecordAttempts)
			continue
		}
		// Record located. Set created to true and break from loop
		created = true
		break
	}

	return created
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
