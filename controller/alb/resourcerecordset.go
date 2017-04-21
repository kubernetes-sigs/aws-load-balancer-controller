package alb

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/coreos/alb-ingress-controller/awsutil"
	"github.com/coreos/alb-ingress-controller/log"
)

// ResourceRecordSet contains the relevant Route 53 zone id for the host name along with the
// current and desired state.
type ResourceRecordSet struct {
	IngressID                *string
	ZoneID                   *string
	CurrentResourceRecordSet *route53.ResourceRecordSet
	DesiredResourceRecordSet *route53.ResourceRecordSet
}

// NewResourceRecordSet returns a new route53.ResourceRecordSet based on the LoadBalancer provided.
func NewResourceRecordSet(hostname *string, ingressID *string) (*ResourceRecordSet, error) {
	zoneID, err := awsutil.Route53svc.GetZoneID(hostname)
	if err != nil {
		log.Errorf("Unabled to locate ZoneId for %s.", *ingressID, hostname)
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
		ZoneID:    zoneID.Id,
		IngressID: ingressID,
	}

	return record, nil
}

// SyncState compares the current and desired state of this ResourceRecordSet instance. Comparison
// results in no action, the creation, the deletion, or the modification of Route 53 resource
// record set to satisfy the ingress's current state.
func (r *ResourceRecordSet) SyncState(lb *LoadBalancer) error {
	switch {
	case r.DesiredResourceRecordSet == nil: // rrs should be deleted
		if r.CurrentResourceRecordSet == nil {
			break
		}
		log.Infof("Start Route53 resource record set deletion.", *r.IngressID)
		if err := r.delete(lb); err != nil {
			return err
		}
		log.Infof("Completed deletion of Route 53 resource record set. DNS: %s",
			*lb.IngressID, *lb.Hostname)

	case r.CurrentResourceRecordSet == nil: // rrs doesn't exist and should be created
		log.Infof("Start Route53 resource record set creation.", *r.IngressID)
		r.PopulateFromLoadBalancer(lb.CurrentLoadBalancer)
		if err := r.create(lb); err != nil {
			return err
		}
		log.Infof("Completed Route 53 resource record set creation. DNS: %s | Type: %s | Target: %s.",
			*lb.IngressID, *lb.Hostname, *r.CurrentResourceRecordSet.Type,
			log.Prettify(*r.CurrentResourceRecordSet.AliasTarget))

	default: // check for diff between current and desired rrs; mod if needed
		r.PopulateFromLoadBalancer(lb.CurrentLoadBalancer)
		// Only perform modifictation if needed.
		if r.needsModification() {
			log.Infof("Start Route 53 resource record set modification.", *r.IngressID)
			if err := r.modify(lb); err != nil {
				return err
			}
			log.Infof("Completed Route 53 resource record set modification. DNS: %s | Type: %s | AliasTarget: %s",
				*r.IngressID, *r.CurrentResourceRecordSet.Name, *r.CurrentResourceRecordSet.Type, log.Prettify(*r.CurrentResourceRecordSet.AliasTarget))
		} else {
			log.Debugf("No modification of Route 53 resource record set required.", *r.IngressID)
		}
	}

	return nil
}

func (r *ResourceRecordSet) create(lb *LoadBalancer) error {
	// If a record pre-exists, delete it.
	existing := awsutil.LookupExistingRecord(lb.Hostname)
	if existing != nil {
		if *existing.Type != route53.RRTypeA {
			r.CurrentResourceRecordSet = existing
			r.delete(lb)
		}
	}

	err := r.modify(lb)
	if err != nil {
		log.Infof("Failed Route 53 resource record set creation. DNS: %s | Type: %s | Target: %s | Error: %s.",
			*lb.IngressID, *lb.Hostname, *r.CurrentResourceRecordSet.Type, log.Prettify(*r.CurrentResourceRecordSet.AliasTarget), err.Error())
		return err
	}

	return nil
}

func (r *ResourceRecordSet) delete(lb *LoadBalancer) error {
	// Attempt record deletion
	in := route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action:            aws.String("DELETE"),
					ResourceRecordSet: r.CurrentResourceRecordSet,
				},
			},
		},
		HostedZoneId: r.ZoneID,
	}

	if err := awsutil.Route53svc.Delete(in); err != nil {
		log.Errorf("Failed deletion of route53 resource record set. DNS: %s | Target: %s | Error: %s",
			*r.IngressID, *r.CurrentResourceRecordSet.Name, log.Prettify(*r.CurrentResourceRecordSet.AliasTarget), err.Error())
		return err
	}

	r.CurrentResourceRecordSet = nil
	return nil
}

func (r *ResourceRecordSet) modify(lb *LoadBalancer) error {
	// Use all values from DesiredResourceRecordSet to run upsert against existing RecordSet in AWS.
	in := route53.ChangeResourceRecordSetsInput{
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
		HostedZoneId: r.ZoneID, // Required
	}

	if err := awsutil.Route53svc.Modify(in); err != nil {
		log.Errorf("Failed Route 53 resource record set modification. UPSERT to AWS API failed. Error: %s",
			*r.IngressID, err.Error())
		return err
	}

	// When delete is required, delete the CurrentResourceRecordSet.
	deleteRequired := r.isDeleteRequired()
	if deleteRequired {
		r.delete(lb)
	}

	// Upon success, ensure all possible updated attributes are updated in local Resource Record Set reference
	r.CurrentResourceRecordSet = r.DesiredResourceRecordSet

	return nil
}

// Checks to see if the CurrentResourceRecordSet exists and whether its hostname differs from
// DesiredResourceRecordSet's hostname. If both are true, the CurrentResourceRecordSet will still
// exist in AWS and should be deleted. In that case, this method returns true.
func (r *ResourceRecordSet) isDeleteRequired() bool {
	if r.CurrentResourceRecordSet == nil {
		return false
	}
	if *r.CurrentResourceRecordSet.Name == *r.DesiredResourceRecordSet.Name {
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

// PopulateFromLoadBalancer configures the DesiredResourceRecordSet with values from a n elbv2.LoadBalancer
func (r *ResourceRecordSet) PopulateFromLoadBalancer(lb *elbv2.LoadBalancer) {
	r.DesiredResourceRecordSet.AliasTarget.DNSName = aws.String(*lb.DNSName + ".")
	r.DesiredResourceRecordSet.AliasTarget.HostedZoneId = lb.CanonicalHostedZoneId
}
