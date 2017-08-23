package loadbalancer

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	api "k8s.io/api/core/v1"

	"github.com/coreos/alb-ingress-controller/pkg/alb/listeners"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroups"
	"github.com/coreos/alb-ingress-controller/pkg/annotations"
	albelbv2 "github.com/coreos/alb-ingress-controller/pkg/aws/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
)

// LoadBalancer contains the overarching configuration for the ALB
type LoadBalancer struct {
	ID           string
	Current      *elbv2.LoadBalancer // current version of load balancer in AWS
	Desired      *elbv2.LoadBalancer // desired version of load balancer in AWS
	TargetGroups targetgroups.TargetGroups
	Listeners    listeners.Listeners
	CurrentTags  util.Tags
	DesiredTags  util.Tags
	Deleted      bool // flag representing the LoadBalancer instance was fully deleted.
	logger       *log.Logger
}

type loadBalancerChange uint

const (
	securityGroupsModified loadBalancerChange = 1 << iota
	subnetsModified
	tagsModified
	schemeModified
)

type NewDesiredLoadBalancerOptions struct {
	ClusterName          string
	Namespace            string
	IngressName          string
	ExistingLoadBalancer *LoadBalancer
	Logger               *log.Logger
	Annotations          *annotations.Annotations
	Tags                 util.Tags
}

// NewDesiredLoadBalancer returns a new loadbalancer.LoadBalancer based on the parameters provided.
func NewDesiredLoadBalancer(o *NewDesiredLoadBalancerOptions) *LoadBalancer {
	// TODO: LB name  must contain only alphanumeric characters or hyphens, and must
	// not begin or end with a hyphen.

	hasher := md5.New()
	hasher.Write([]byte(o.Namespace + o.IngressName))
	hash := hex.EncodeToString(hasher.Sum(nil))[:4]

	name := fmt.Sprintf("%s-%s-%s",
		o.ClusterName,
		strings.Replace(o.Namespace, "-", "", -1),
		strings.Replace(o.IngressName, "-", "", -1),
	)

	if len(name) > 26 {
		name = name[:26]
	}

	name = name + "-" + hash

	newLoadBalancer := &LoadBalancer{
		ID:          name,
		DesiredTags: o.Tags,
		Desired: &elbv2.LoadBalancer{
			AvailabilityZones: o.Annotations.Subnets.AsAvailabilityZones(),
			LoadBalancerName:  aws.String(name),
			Scheme:            o.Annotations.Scheme,
			SecurityGroups:    o.Annotations.SecurityGroups,
			VpcId:             o.Annotations.VPCID,
		},
		logger: o.Logger,
	}

	if o.ExistingLoadBalancer != nil {
		// we had an existing LoadBalancer in ingress, so just copy the desired state over
		o.ExistingLoadBalancer.Desired = newLoadBalancer.Desired
		o.ExistingLoadBalancer.DesiredTags = newLoadBalancer.DesiredTags
		return o.ExistingLoadBalancer
	}

	// no existing LoadBalancer, so use the one we just created
	return newLoadBalancer
}

type NewCurrentLoadBalancerOptions struct {
	LoadBalancer *elbv2.LoadBalancer
	Tags         util.Tags
	ClusterName  string
	Logger       *log.Logger
}

// NewCurrentLoadBalancer returns a new loadbalancer.LoadBalancer based on an elbv2.LoadBalancer.
func NewCurrentLoadBalancer(o *NewCurrentLoadBalancerOptions) (*LoadBalancer, error) {
	ingressName, ok := o.Tags.Get("IngressName")
	if !ok {
		return nil, fmt.Errorf("The LoadBalancer %s does not have an IngressName tag, can't import", *o.LoadBalancer.LoadBalancerName)
	}

	namespace, ok := o.Tags.Get("Namespace")
	if !ok {
		return nil, fmt.Errorf("The LoadBalancer %s does not have an Namespace tag, can't import", *o.LoadBalancer.LoadBalancerName)
	}

	hasher := md5.New()
	hasher.Write([]byte(namespace + ingressName))
	hash := hex.EncodeToString(hasher.Sum(nil))[:4]

	name := fmt.Sprintf("%s-%s-%s",
		o.ClusterName,
		strings.Replace(namespace, "-", "", -1),
		strings.Replace(ingressName, "-", "", -1),
	)

	if len(name) > 26 {
		name = name[:26]
	}

	name = name + "-" + hash

	return &LoadBalancer{
		ID:          name,
		CurrentTags: o.Tags,
		Current:     o.LoadBalancer,
		logger:      o.Logger,
	}, nil
}

// Reconcile compares the current and desired state of this LoadBalancer instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS ELBV2 (ALB) to
// satisfy the ingress's current state.
func (lb *LoadBalancer) Reconcile(rOpts *ReconcileOptions) []error {
	var errors []error

	switch {
	case lb.Desired == nil: // lb should be deleted
		if lb.Current == nil {
			break
		}
		lb.logger.Infof("Start ELBV2 (ALB) deletion.")
		if err := lb.delete(rOpts); err != nil {
			errors = append(errors, err)
			break
		}
		rOpts.Eventf(api.EventTypeNormal, "DELETE", "%s deleted", *lb.Current.LoadBalancerName)
		lb.logger.Infof("Completed ELBV2 (ALB) deletion. Name: %s | ARN: %s",
			*lb.Current.LoadBalancerName,
			*lb.Current.LoadBalancerArn)

	case lb.Current == nil: // lb doesn't exist and should be created
		lb.logger.Infof("Start ELBV2 (ALB) creation.")
		if err := lb.create(rOpts); err != nil {
			errors = append(errors, err)
			return errors
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s created", *lb.Current.LoadBalancerName)
		lb.logger.Infof("Completed ELBV2 (ALB) creation. Name: %s | ARN: %s",
			*lb.Current.LoadBalancerName,
			*lb.Current.LoadBalancerArn)

	default: // check for diff between lb current and desired, modify if necessary
		needsModification, _ := lb.needsModification()
		if needsModification == 0 {
			lb.logger.Debugf("No modification of ELBV2 (ALB) required.")
			break
		}

		lb.logger.Infof("Start ELBV2 (ALB) modification.")
		if err := lb.modify(rOpts); err != nil {
			errors = append(errors, err)
			break
		}
	}

	tgsOpts := &targetgroups.ReconcileOptions{
		Eventf: rOpts.Eventf,
		VpcID:  lb.Current.VpcId,
	}
	if tgs, err := lb.TargetGroups.Reconcile(tgsOpts); err != nil {
		errors = append(errors, err)
	} else {
		lb.TargetGroups = tgs
	}

	lsOpts := &listeners.ReconcileOptions{
		Eventf:          rOpts.Eventf,
		LoadBalancerArn: lb.Current.LoadBalancerArn,
		TargetGroups:    lb.TargetGroups,
	}
	if ltnrs, err := lb.Listeners.Reconcile(lsOpts); err != nil {
		errors = append(errors, err)
	} else {
		lb.Listeners = ltnrs
	}

	return errors
}

// create requests a new ELBV2 (ALB) is created in AWS.

func (lb *LoadBalancer) create(rOpts *ReconcileOptions) error {
	in := &elbv2.CreateLoadBalancerInput{
		Name:           lb.Desired.LoadBalancerName,
		Subnets:        util.AvailabilityZones(lb.Desired.AvailabilityZones).AsSubnets(),
		Scheme:         lb.Desired.Scheme,
		Tags:           lb.DesiredTags,
		SecurityGroups: lb.Desired.SecurityGroups,
	}

	o, err := albelbv2.ELBV2svc.CreateLoadBalancer(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating %s: %s", *in.Name, err.Error())
		lb.logger.Errorf("Failed to create ELBV2 (ALB): %s", err.Error())
		return err
	}

	lb.Current = o.LoadBalancers[0]
	return nil
}

// modify modifies the attributes of an existing ALB in AWS.
func (lb *LoadBalancer) modify(rOpts *ReconcileOptions) error {
	needsMod, canMod := lb.needsModification()
	if canMod {

		// Modify Security Groups
		if needsMod&securityGroupsModified != 0 {
			lb.logger.Infof("Start ELBV2 security groups modification.")
			in := &elbv2.SetSecurityGroupsInput{
				LoadBalancerArn: lb.Current.LoadBalancerArn,
				SecurityGroups:  lb.Desired.SecurityGroups,
			}
			if _, err := albelbv2.ELBV2svc.SetSecurityGroups(in); err != nil {
				lb.logger.Errorf("Failed ELBV2 security groups modification: %s", err.Error())
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s security group modification failed: %s", *lb.Current.LoadBalancerName, err.Error())
				return err
			}
			lb.Current.SecurityGroups = lb.Desired.SecurityGroups
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s security group modified", *lb.Current.LoadBalancerName)
			lb.logger.Infof("Completed ELBV2 security groups modification. SGs: %s",
				log.Prettify(lb.Current.SecurityGroups))
		}

		// Modify Subnets
		if needsMod&subnetsModified != 0 {
			lb.logger.Infof("Start subnets modification.")
			in := &elbv2.SetSubnetsInput{
				LoadBalancerArn: lb.Current.LoadBalancerArn,
				Subnets:         util.AvailabilityZones(lb.Desired.AvailabilityZones).AsSubnets(),
			}
			if _, err := albelbv2.ELBV2svc.SetSubnets(in); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s subnet modification failed: %s", *lb.Current.LoadBalancerName, err.Error())
				return fmt.Errorf("Failure Setting ALB Subnets: %s", err)
			}
			lb.Current.AvailabilityZones = lb.Desired.AvailabilityZones
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s subnets modified", *lb.Current.LoadBalancerName)
			lb.logger.Infof("Completed subnets modification. Subnets are %s.",
				log.Prettify(lb.Current.AvailabilityZones))
		}

		// Modify Tags
		if needsMod&tagsModified != 0 {
			lb.logger.Infof("Start ELBV2 tag modification.")
			if err := albelbv2.ELBV2svc.UpdateTags(lb.Current.LoadBalancerArn, lb.CurrentTags, lb.DesiredTags); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s tag modification failed: %s", *lb.Current.LoadBalancerName, err.Error())
				lb.logger.Errorf("Failed ELBV2 (ALB) tag modification: %s", err.Error())
			}
			lb.CurrentTags = lb.DesiredTags
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s tags modified", *lb.Current.LoadBalancerName)
			lb.logger.Infof("Completed ELBV2 tag modification. Tags are %s.",
				log.Prettify(lb.CurrentTags))
		}

	} else {
		// Modification is needed, but required full replacement of ALB.
		lb.logger.Infof("Start ELBV2 full modification (delete and create).")
		rOpts.Eventf(api.EventTypeNormal, "REBUILD", "Impossible modification requested, rebuilding %s", *lb.Current.LoadBalancerName)
		lb.delete(rOpts)
		// Since listeners and rules are deleted during lb deletion, ensure their current state is removed
		// as they'll no longer exist.
		lb.Listeners.StripCurrentState()
		lb.create(rOpts)
		lb.logger.Infof("Completed ELBV2 full modification (delete and create). Name: %s | ARN: %s",
			*lb.Current.LoadBalancerName, *lb.Current.LoadBalancerArn)

	}

	return nil
}

// delete Deletes the load balancer from AWS.
func (lb *LoadBalancer) delete(rOpts *ReconcileOptions) error {
	in := &elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: lb.Current.LoadBalancerArn,
	}

	if _, err := albelbv2.ELBV2svc.DeleteLoadBalancer(in); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error deleting %s: %s", *lb.Current.LoadBalancerName, err.Error())
		lb.logger.Errorf("Failed deletion of ELBV2 (ALB): %s.", err.Error())
		return err
	}

	lb.Deleted = true
	return nil
}

// needsModification returns if a LB needs to be modified and if it can be modified in place
// first parameter is true if the LB needs to be changed
// second parameter true if it can be changed in place
func (lb *LoadBalancer) needsModification() (loadBalancerChange, bool) {
	var changes loadBalancerChange

	// In the case that the LB does not exist yet
	if lb.Current == nil {
		return changes, true
	}

	if !util.DeepEqual(lb.Current.Scheme, lb.Desired.Scheme) {
		changes |= schemeModified
		return changes, false
	}

	currentSubnets := util.AvailabilityZones(lb.Current.AvailabilityZones).AsSubnets()
	desiredSubnets := util.AvailabilityZones(lb.Desired.AvailabilityZones).AsSubnets()
	sort.Sort(currentSubnets)
	sort.Sort(desiredSubnets)
	if log.Prettify(currentSubnets) != log.Prettify(desiredSubnets) {
		changes |= subnetsModified
	}

	currentSecurityGroups := util.AWSStringSlice(lb.Current.SecurityGroups)
	desiredSecurityGroups := util.AWSStringSlice(lb.Desired.SecurityGroups)
	sort.Sort(currentSecurityGroups)
	sort.Sort(desiredSecurityGroups)
	if log.Prettify(currentSecurityGroups) != log.Prettify(desiredSecurityGroups) {
		changes |= securityGroupsModified
	}

	sort.Sort(lb.CurrentTags)
	sort.Sort(lb.DesiredTags)
	if log.Prettify(lb.CurrentTags) != log.Prettify(lb.DesiredTags) {
		changes |= tagsModified
	}

	return changes, true
}

// StripDesiredState removes the DesiredLoadBalancer from the LoadBalancer
func (l *LoadBalancer) StripDesiredState() {
	l.Desired = nil
	if l.Listeners != nil {
		l.Listeners.StripDesiredState()
	}
	if l.TargetGroups != nil {
		l.TargetGroups.StripDesiredState()
	}
}

type ReconcileOptions struct {
	Eventf func(string, string, string, ...interface{})
}
