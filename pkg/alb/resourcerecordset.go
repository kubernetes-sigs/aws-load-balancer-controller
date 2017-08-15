package alb

import (
	"fmt"
	"strings"

	api "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
	awsutil "github.com/coreos/alb-ingress-controller/pkg/util/aws"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
)

// ResourceRecordSet contains the relevant Route 53 zone id for the host name along with the
// current and desired state.
type ResourceRecordSet struct {
	ZoneID                   *string
	Resolveable              bool
	CurrentResourceRecordSet *route53.ResourceRecordSet
	DesiredResourceRecordSet *route53.ResourceRecordSet
	logger                   *log.Logger
}

// NewResourceRecordSet returns a new route53.ResourceRecordSet based on the LoadBalancer provided.
func NewResourceRecordSet(hostname *string, logger *log.Logger) (*ResourceRecordSet, error) {
	record := &ResourceRecordSet{
		DesiredResourceRecordSet: &route53.ResourceRecordSet{
			AliasTarget: &route53.AliasTarget{
				EvaluateTargetHealth: aws.Bool(false),
			},
			Type: aws.String("A"),
		},
		logger:      logger,
		Resolveable: true,
	}

	zoneID, err := awsutil.Route53svc.GetZoneID(hostname)

	if err != nil {
		record.Resolveable = false
		e := fmt.Errorf("Unable to locate ZoneId for %s: %s", *hostname, err.Error())
		return record, e
	}

	name := *hostname
	if !strings.HasPrefix(*hostname, ".") {
		name = *hostname + "."
	}

	record.ZoneID = zoneID.Id
	record.DesiredResourceRecordSet.Name = aws.String(name)

	return record, nil
}

// Reconcile compares the current and desired state of this ResourceRecordSet instance. Comparison
// results in no action, the creation, the deletion, or the modification of Route 53 resource
// record set to satisfy the ingress's current state.
func (r *ResourceRecordSet) Reconcile(rOpts *ReconcileOptions) error {
	lb := rOpts.loadbalancer
	switch {
	case !r.Resolveable:
		rOpts.Eventf(api.EventTypeNormal, "ERROR", "%s Route 53 record unresolvable", *lb.Hostname)
		return fmt.Errorf("Route53 Resource record set flagged as unresolveable. Record: %s",
			*lb.Hostname)
	case r.DesiredResourceRecordSet == nil: // rrs should be deleted
		if r.CurrentResourceRecordSet == nil {
			break
		}
		r.logger.Infof("Start Route53 resource record set deletion.")
		if err := r.delete(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "DELETE", "%s Route 53 record deleted", *lb.Hostname)
		r.logger.Infof("Completed deletion of Route 53 resource record set. DNS: %s",
			*lb.Hostname)

	case r.CurrentResourceRecordSet == nil: // rrs doesn't exist and should be created
		r.logger.Infof("Start Route53 resource record set creation.")
		r.PopulateFromLoadBalancer(lb.CurrentLoadBalancer)
		if err := r.create(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s Route 53 record created", *lb.Hostname)
		r.logger.Infof("Completed Route 53 resource record set creation. DNS: %s | Type: %s | Target: %s.",
			*lb.Hostname, *r.CurrentResourceRecordSet.Type,
			log.Prettify(*r.CurrentResourceRecordSet.AliasTarget))

	default: // check for diff between current and desired rrs; mod if needed
		r.PopulateFromLoadBalancer(lb.CurrentLoadBalancer)
		// Only perform modifictation if needed.
		if r.needsModification() {
			r.logger.Infof("Start Route 53 resource record set modification.")
			if err := r.modify(rOpts); err != nil {
				return err
			}
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s Route 53 record modified", *lb.Hostname)
			r.logger.Infof("Completed Route 53 resource record set modification. DNS: %s | Type: %s | AliasTarget: %s",
				*r.CurrentResourceRecordSet.Name, *r.CurrentResourceRecordSet.Type, log.Prettify(*r.CurrentResourceRecordSet.AliasTarget))
		} else {
			r.logger.Debugf("No modification of Route 53 resource record set required.")
		}
	}

	return nil
}

func (r *ResourceRecordSet) create(rOpts *ReconcileOptions) error {
	lb := rOpts.loadbalancer
	// If a record pre-exists, delete it.
	existing := awsutil.LookupExistingRecord(lb.Hostname)
	if existing != nil {
		if *existing.Type != route53.RRTypeA {
			r.CurrentResourceRecordSet = existing
			r.delete(rOpts)
		}
	}

	err := r.modify(rOpts)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating %s record: %s", *lb.Hostname, err.Error())
		r.logger.Infof("Failed Route 53 resource record set creation. DNS: %s | Type: %s | Target: %s | Error: %s.",
			*lb.Hostname, *r.CurrentResourceRecordSet.Type, log.Prettify(*r.CurrentResourceRecordSet.AliasTarget), err.Error())
		return err
	}

	return nil
}

func (r *ResourceRecordSet) delete(rOpts *ReconcileOptions) error {
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
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error deleting %s record: %s", *r.CurrentResourceRecordSet.Name, err.Error())
		r.logger.Errorf("Failed deletion of route53 resource record set. DNS: %s | Target: %s | Error: %s",
			*r.CurrentResourceRecordSet.Name, log.Prettify(*r.CurrentResourceRecordSet.AliasTarget), err.Error())
		return err
	}

	r.CurrentResourceRecordSet = nil
	return nil
}

func (r *ResourceRecordSet) modify(rOpts *ReconcileOptions) error {
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
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error modifying %s record: %s", *r.DesiredResourceRecordSet.Name, err.Error())
		r.logger.Errorf("Failed Route 53 resource record set modification. UPSERT to AWS API failed. Error: %s",
			err.Error())
		return err
	}

	// When delete is required, delete the CurrentResourceRecordSet.
	deleteRequired := r.isDeleteRequired()
	if deleteRequired {
		r.delete(rOpts)
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
	case !util.DeepEqual(r.CurrentResourceRecordSet.Name, r.DesiredResourceRecordSet.Name):
		return true
		// Load balancer's hostname has changed; modification required.
	case !util.DeepEqual(r.CurrentResourceRecordSet.AliasTarget.DNSName, r.DesiredResourceRecordSet.AliasTarget.DNSName):
		return true
		// DNS record's resource type has changed; modification required.
	case !util.DeepEqual(r.CurrentResourceRecordSet.Type, r.DesiredResourceRecordSet.Type):
		return true
		// Load balancer's dns hosted zone has changed; modification required.
	case !util.DeepEqual(r.CurrentResourceRecordSet.AliasTarget.HostedZoneId, r.DesiredResourceRecordSet.AliasTarget.HostedZoneId):
		return true
	}
	return false
}

// PopulateFromLoadBalancer configures the DesiredResourceRecordSet with values from a n elbv2.LoadBalancer
func (r *ResourceRecordSet) PopulateFromLoadBalancer(lb *elbv2.LoadBalancer) {
	r.DesiredResourceRecordSet.AliasTarget.DNSName = aws.String(*lb.DNSName + ".")
	r.DesiredResourceRecordSet.AliasTarget.HostedZoneId = lb.CanonicalHostedZoneId
}
