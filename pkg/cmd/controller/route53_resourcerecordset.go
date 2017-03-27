package controller

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Amount of time, in seconds, between each attempt to validate a created or modified record's
	// status has reached insyncR53DNSStatus state.
	validateSleepDuration int = 10
	// Maximum attempts should be made to validate a created or modified resource record set has
	// reached insyncR53DNSStatus state.
	maxValidateRecordAttempts int = 30
	// Status used to signify that resource record set that the changes have replicated to all Amazon
	// Route 53 DNS servers.
	insyncR53DNSStatus string = "INSYNC"
)

// ResourceRecordSet contains the relevant Route 53 zone id for the host name along with the
// current and desired state.
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

// SyncState compares the current and desired state of this ResourceRecordSet instance. Comparison
// results in no action, the creation, the deletion, or the modification of Route 53 resource
// record set to satisfy the ingress's current state.
func (r *ResourceRecordSet) SyncState(lb *LoadBalancer) *ResourceRecordSet {
	// When DesiredResourceRecordSet is nil, the resource record set should be deleted from Route 53.
	// TODO: Make this a switch statement for readability.
	if r.DesiredResourceRecordSet == nil {
		if err := r.delete(lb); err != nil {
			// TODO: Needs id or name applicable for record set
			glog.Errorf("Error deleting Route 53 resource record set %s: %s", *r.CurrentResourceRecordSet, err.Error())
			return r
		}
		// When CurrentResourceRecordSet is nil, the resource record set doesn't exist and should be
		// created in Route53.
	} else if r.CurrentResourceRecordSet == nil {
		r.PopulateFromLoadBalancer(lb.CurrentLoadBalancer)
		if err := r.create(lb); err != nil {
			glog.Errorf("Error creating Route 53 Resource Record set %s: %s", *r.DesiredResourceRecordSet, err.Error())
		}
		// When CurrentResourceRecordSet and DesiredResourceRecordSet exist, a comparison is done
		// between current and desired states to determine whether a modification to the AWS resource is
		// needed.
	} else {
		r.PopulateFromLoadBalancer(lb.CurrentLoadBalancer)
		if !r.needsModification() {
			glog.Infof("%s: No modification required after refresh.", *lb.hostname)
			return r
		}
		r.modify(lb)
	}
	return r
}

func (r *ResourceRecordSet) create(lb *LoadBalancer) error {
	// If a record pre-exists, delete it.
	existing := lookupExistingRecord(lb.hostname)
	if existing != nil {
		if *existing.Type != route53.RRTypeA {
			r.CurrentResourceRecordSet = existing
			r.delete(lb)
		}
	}

	err := r.modify(lb)
	if err != nil {
		return err
	}

	glog.Infof("%s: Successfully registered %s in Route53", *lb.hostname)
	return nil
}

func (r *ResourceRecordSet) delete(lb *LoadBalancer) error {
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
	glog.Infof("Deleted %s from Route53", *lb.hostname)
	return nil
}

func (r *ResourceRecordSet) modify(lb *LoadBalancer) error {
	// Use all values from DesiredResourceRecordSet to run upsert against existing RecordSet in AWS.
	params := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(route53.ChangeActionUpsert),
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

	// When delete is required, delete the CurrentResourceRecordSet.
	deleteRequired := r.isDeleteRequired()
	if deleteRequired {
		r.delete(lb)
	}

	// Upon success, ensure all possible updated attributes are updated in local Resource Record Set reference
	r.CurrentResourceRecordSet = r.DesiredResourceRecordSet
	r.DesiredResourceRecordSet = nil

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
			glog.Infof("%s status was %s in Route 53. Attempt %d/%d.", *r.DesiredResourceRecordSet.Name,
				status, i+1, maxValidateRecordAttempts)
			continue
		}
		// Record located. Set created to true and break from loop
		created = true
		break
	}

	return created
}

// Checks to see if the CurrentResourceRecordSet exists and whether its hostname differs from
// DesiredResourceRecordSet's hostname. If both are true, the CurrentResourceRecordSet will still
// exist in AWS and should be deleted. In that case, this method returns true.
func (r *ResourceRecordSet) isDeleteRequired() bool {
	if r.CurrentResourceRecordSet == nil {
		return false
	}
	if r.CurrentResourceRecordSet.Name == r.DesiredResourceRecordSet.Name {
		return false
	}
	// The ResourceRecordSet DNS name has changed between desired and current and should be deleted.
	return true
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
