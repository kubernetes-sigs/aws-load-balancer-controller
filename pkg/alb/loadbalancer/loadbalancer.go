package loadbalancer

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/alb/listeners"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroup"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroups"
	"github.com/coreos/alb-ingress-controller/pkg/annotations"
	"github.com/coreos/alb-ingress-controller/pkg/aws/ec2"
	albelbv2 "github.com/coreos/alb-ingress-controller/pkg/aws/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
	api "k8s.io/api/core/v1"
)

// LoadBalancer contains the overarching configuration for the ALB
type LoadBalancer struct {
	ID                       string
	Current                  *elbv2.LoadBalancer // current version of load balancer in AWS
	Desired                  *elbv2.LoadBalancer // desired version of load balancer in AWS
	TargetGroups             targetgroups.TargetGroups
	Listeners                listeners.Listeners
	DesiredIdleTimeout       int64
	CurrentIdleTimeout       *int64
	CurrentTags              util.Tags
	DesiredTags              util.Tags
	CurrentAttributes        []*elbv2.LoadBalancerAttribute
	DesiredAttributes        []*elbv2.LoadBalancerAttribute
	CurrentPorts             portList
	DesiredPorts             portList
	CurrentManagedSG         *string
	DesiredManagedSG         *string
	CurrentManagedInstanceSG *string
	DesiredManagedInstanceSG *string
	Deleted                  bool // flag representing the LoadBalancer instance was fully deleted.
	logger                   *log.Logger
}

type loadBalancerChange uint

const (
	securityGroupsModified loadBalancerChange = 1 << iota
	subnetsModified
	tagsModified
	schemeModified
	attributesModified
	managedSecurityGroupsModified
	connectionIdleTimeoutModified
)

type NewDesiredLoadBalancerOptions struct {
	ALBNamePrefix        string
	Namespace            string
	IngressName          string
	ExistingLoadBalancer *LoadBalancer
	Logger               *log.Logger
	Annotations          *annotations.Annotations
	Tags                 util.Tags
	Attributes           []*elbv2.LoadBalancerAttribute
}

type portList []int64

func (a portList) Len() int           { return len(a) }
func (a portList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a portList) Less(i, j int) bool { return a[i] < a[j] }

// NewDesiredLoadBalancer returns a new loadbalancer.LoadBalancer based on the parameters provided.
func NewDesiredLoadBalancer(o *NewDesiredLoadBalancerOptions) *LoadBalancer {
	// TODO: LB name  must contain only alphanumeric characters or hyphens, and must
	// not begin or end with a hyphen.
	name := createLBName(o.Namespace, o.IngressName, o.ALBNamePrefix)

	newLoadBalancer := &LoadBalancer{
		ID:                 name,
		DesiredTags:        o.Tags,
		DesiredIdleTimeout: o.Annotations.ConnectionIdleTimeout,
		Desired: &elbv2.LoadBalancer{
			AvailabilityZones: o.Annotations.Subnets.AsAvailabilityZones(),
			LoadBalancerName:  aws.String(name),
			Scheme:            o.Annotations.Scheme,
			SecurityGroups:    o.Annotations.SecurityGroups,
			VpcId:             o.Annotations.VPCID,
		},
		logger: o.Logger,
	}

	lsps := portList{}
	for _, port := range o.Annotations.Ports {
		lsps = append(lsps, port.Port)
	}

	if len(o.Annotations.Attributes) != 0 {
		newLoadBalancer.DesiredAttributes = o.Annotations.Attributes
	}
	if len(newLoadBalancer.Desired.SecurityGroups) == 0 {
		newLoadBalancer.DesiredPorts = lsps
	}

	// TODO: What is this for??
	if o.ExistingLoadBalancer != nil {
		// we had an existing LoadBalancer in ingress, so just copy the desired state over
		o.ExistingLoadBalancer.Desired = newLoadBalancer.Desired
		o.ExistingLoadBalancer.DesiredTags = newLoadBalancer.DesiredTags
		o.ExistingLoadBalancer.DesiredIdleTimeout = newLoadBalancer.DesiredIdleTimeout
		if len(o.ExistingLoadBalancer.Desired.SecurityGroups) == 0 {
			o.ExistingLoadBalancer.DesiredPorts = lsps
		}
		return o.ExistingLoadBalancer
	}

	// no existing LoadBalancer, so use the one we just created
	return newLoadBalancer
}

type NewCurrentLoadBalancerOptions struct {
	LoadBalancer          *elbv2.LoadBalancer
	Tags                  util.Tags
	ALBNamePrefix         string
	Logger                *log.Logger
	ManagedSG             *string
	ManagedInstanceSG     *string
	ManagedSGPorts        []int64
	ConnectionIdleTimeout *int64
}

// NewCurrentLoadBalancer returns a new loadbalancer.LoadBalancer based on an elbv2.LoadBalancer.
func NewCurrentLoadBalancer(o *NewCurrentLoadBalancerOptions) (*LoadBalancer, error) {
	ingressName, ok := o.Tags.Get("IngressName")
	if !ok {
		return nil, fmt.Errorf("The LoadBalancer %s does not have an IngressName tag, can't import", *o.LoadBalancer.LoadBalancerName)
	}

	namespace, ok := o.Tags.Get("Namespace")
	if !ok {
		return nil, fmt.Errorf("The LoadBalancer %s does not have a Namespace tag, can't import", *o.LoadBalancer.LoadBalancerName)
	}

	name := createLBName(namespace, ingressName, o.ALBNamePrefix)
	if name != *o.LoadBalancer.LoadBalancerName {
		return nil, fmt.Errorf("Loadbalancer does not have expected (calculated) name. "+
			"Expecting %s but was %s.", name, *o.LoadBalancer.LoadBalancerName)
	}

	return &LoadBalancer{
		ID:                       name,
		CurrentTags:              o.Tags,
		Current:                  o.LoadBalancer,
		logger:                   o.Logger,
		CurrentManagedSG:         o.ManagedSG,
		CurrentPorts:             o.ManagedSGPorts,
		CurrentManagedInstanceSG: o.ManagedInstanceSG,
		CurrentIdleTimeout:       o.ConnectionIdleTimeout,
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
		lb.logger.Infof("Completed ELBV2 (ALB) modification.")
	}

	tgsOpts := &targetgroups.ReconcileOptions{
		Eventf:            rOpts.Eventf,
		VpcID:             lb.Current.VpcId,
		ManagedSGInstance: lb.CurrentManagedInstanceSG,
	}

	tgs, cleanUp, err := lb.TargetGroups.Reconcile(tgsOpts)
	if err != nil {
		errors = append(errors, err)
		return errors
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

	for _, l := range lb.Listeners {
		if l.Deleted {
			i := lb.Listeners.Find(l.Current)
			lb.Listeners = append(lb.Listeners[:i], lb.Listeners[i+1:]...)
		}
	}

	// Return now if listeners are already deleted, signifies has already been destructed and
	// TG clean-up, based on rules below does not need to occur.
	if len(lb.Listeners) < 1 {
		for _, tg := range cleanUp {
			if err := targetgroup.DeleteTG(tg); err != nil {
				errors = append(errors, err)
				return errors
			}
			index, _ := lb.TargetGroups.FindById(tg.ID)
			lb.TargetGroups = append(lb.TargetGroups[:index], lb.TargetGroups[index+1:]...)
		}
		return errors
	}

	unusedTGs := lb.Listeners[0].Rules.FindUnusedTGs(lb.TargetGroups)
	for _, tg := range unusedTGs {
		if err := targetgroup.DeleteTG(tg); err != nil {
			errors = append(errors, err)
			return errors
		}
		index, _ := lb.TargetGroups.FindById(tg.ID)
		lb.TargetGroups = append(lb.TargetGroups[:index], lb.TargetGroups[index+1:]...)
	}

	return errors
}

// reconcileExistingManagedSG checks AWS for an existing SG with that matches the description of what would
// otherwise be created. If an SG is found, it will run an update to ensure the rules are up to date.
func (lb *LoadBalancer) reconcileExistingManagedSG() error {
	if len(lb.DesiredPorts) < 1 {
		return fmt.Errorf("No ports specified on ingress. Ingress resource may be misconfigured")
	}
	vpcID, err := ec2.EC2svc.GetVPCID()
	if err != nil {
		return err
	}

	sgID, instanceSG, err := ec2.EC2svc.UpdateSGIfNeeded(vpcID, aws.String(lb.ID), lb.CurrentPorts, lb.DesiredPorts)
	if err != nil {
		return err
	}

	// sgID could be nil, if an existing SG didn't exist or it could have a pointer to an sgID in it.
	lb.DesiredManagedSG = sgID
	lb.DesiredManagedInstanceSG = instanceSG
	return nil
}

// create requests a new ELBV2 (ALB) is created in AWS.
func (lb *LoadBalancer) create(rOpts *ReconcileOptions) error {

	// TODO: This whole thing can become a resolveSGs func
	var sgs util.AWSStringSlice
	// check if desired securitygroups are already expressed through annotations
	if len(lb.Desired.SecurityGroups) > 0 {
		sgs = lb.Desired.SecurityGroups
	} else {
		lb.reconcileExistingManagedSG()
	}
	if lb.DesiredManagedSG != nil {
		sgs = append(sgs, lb.DesiredManagedSG)

		if lb.DesiredManagedInstanceSG == nil {
			vpcID, err := ec2.EC2svc.GetVPCID()
			if err != nil {
				return err
			}
			instSG, err := ec2.EC2svc.CreateNewInstanceSG(aws.String(lb.ID), lb.DesiredManagedSG, vpcID)
			if err != nil {
				return err
			}
			lb.DesiredManagedInstanceSG = instSG
		}
	}

	// when sgs are not known, attempt to create them
	if len(sgs) < 1 {
		vpcID, err := ec2.EC2svc.GetVPCID()
		if err != nil {
			return err
		}
		newSG, newInstSG, err := ec2.EC2svc.CreateSecurityGroupFromPorts(vpcID, aws.String(lb.ID), lb.DesiredPorts)
		if err != nil {
			return err
		}
		sgs = append(sgs, newSG)
		lb.DesiredManagedSG = newSG
		lb.DesiredManagedInstanceSG = newInstSG
	}

	in := &elbv2.CreateLoadBalancerInput{
		Name:           lb.Desired.LoadBalancerName,
		Subnets:        util.AvailabilityZones(lb.Desired.AvailabilityZones).AsSubnets(),
		Scheme:         lb.Desired.Scheme,
		Tags:           lb.DesiredTags,
		SecurityGroups: sgs,
	}

	o, err := albelbv2.ELBV2svc.CreateLoadBalancer(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating %s: %s", *in.Name, err.Error())
		lb.logger.Errorf("Failed to create ELBV2 (ALB): %s", err.Error())
		return err
	}

	// lb created. set to current
	lb.Current = o.LoadBalancers[0]

	// DesiredIdleTimeout is 0 when no annotation was set, thus no modification should be attempted
	// this will result in using the AWS default
	if lb.DesiredIdleTimeout != 0 {
		if err := albelbv2.ELBV2svc.SetIdleTimeout(lb.Current.LoadBalancerArn, lb.DesiredIdleTimeout); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s tag modification failed: %s", *lb.Current.LoadBalancerName, err.Error())
			lb.logger.Errorf("Failed ELBV2 (ALB) tag modification: %s", err.Error())
			return err
		}
		lb.CurrentIdleTimeout = aws.Int64(lb.DesiredIdleTimeout)
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "Set ALB's connection idle timeout to %d", *lb.CurrentIdleTimeout)
		lb.logger.Infof("Connection idle timeout set to %d", *lb.CurrentIdleTimeout)
	}
	if len(lb.DesiredAttributes) > 0 {
		newAttributes := &elbv2.ModifyLoadBalancerAttributesInput{
			LoadBalancerArn: lb.Current.LoadBalancerArn,
			Attributes:      lb.DesiredAttributes,
		}

		_, err = albelbv2.ELBV2svc.ModifyLoadBalancerAttributes(newAttributes)
		if err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error adding attributes to %s: %s", *in.Name, err.Error())
			lb.logger.Errorf("Failed to modify ELBV2 attributes (ALB): %s", err.Error())
			return err
		}
	}

	// when a desired managed sg was present, it was used and should be set as the new CurrentManagedSG.
	if lb.DesiredManagedSG != nil {
		lb.CurrentManagedSG = lb.DesiredManagedSG
		lb.CurrentManagedInstanceSG = lb.DesiredManagedInstanceSG
		lb.CurrentPorts = lb.DesiredPorts
	}
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

		// Modify ALB-managed security groups
		if needsMod&managedSecurityGroupsModified != 0 {
			lb.logger.Infof("Start ELBV2-managed security groups modification.")
			if err := lb.reconcileExistingManagedSG(); err != nil {
				lb.logger.Errorf("Failed ELBV2-managed security groups modification: %s", err.Error())
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s security group modification failed: %s", *lb.Current.LoadBalancerName, err.Error())
				return err
			}
			lb.CurrentPorts = lb.DesiredPorts
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

		// Modify Connection Idle Timeout
		if needsMod&connectionIdleTimeoutModified != 0 {
			if lb.DesiredIdleTimeout != 0 {
				if err := albelbv2.ELBV2svc.SetIdleTimeout(lb.Current.LoadBalancerArn, lb.DesiredIdleTimeout); err != nil {
					rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s tag modification failed: %s", *lb.Current.LoadBalancerName, err.Error())
					lb.logger.Errorf("Failed ELBV2 (ALB) tag modification: %s", err.Error())
					return err
				}
				lb.CurrentIdleTimeout = aws.Int64(lb.DesiredIdleTimeout)
				rOpts.Eventf(api.EventTypeNormal, "MODIFY", "Connection idle timeout updated to %d", *lb.CurrentIdleTimeout)
				lb.logger.Infof("Connection idle timeout updated to %d", *lb.CurrentIdleTimeout)
			}
		}

		// Modify Attributes
		if needsMod&attributesModified != 0 {
			lb.logger.Infof("Start ELBV2 tag modification.")
			if err := albelbv2.ELBV2svc.UpdateAttributes(lb.Current.LoadBalancerArn, lb.DesiredAttributes); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s tag modification failed: %s", *lb.Current.LoadBalancerName, err.Error())
				lb.logger.Errorf("Failed ELBV2 (ALB) tag modification: %s", err.Error())
				return fmt.Errorf("Failure adding ALB attributes: %s", err)
			}
			lb.CurrentAttributes = lb.DesiredAttributes
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s attributes modified", *lb.Current.LoadBalancerName)
			lb.logger.Infof("Completed ELBV2 tag modification. Attributes are %s.",
				log.Prettify(lb.CurrentAttributes))
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

	// if the alb controller was managing a SG we must:
	// - Remove the InstanceSG from all instances known to targetgroups
	// - Delete the InstanceSG
	// - Delete the ALB's SG
	// Deletions are attempted as best effort, if it fails we log the error but don't
	// fail the overall reconcile
	if lb.CurrentManagedSG != nil {
		if err := ec2.EC2svc.DisassociateSGFromInstanceIfNeeded(lb.TargetGroups[0].Targets.Current, lb.CurrentManagedInstanceSG); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "WARN", "Failed disassociating sgs from instances: %s", err.Error())
			lb.logger.Warnf("Failed in deletion of managed SG: %s.", err.Error())
		}
		if err := attemptSGDeletion(lb.CurrentManagedInstanceSG); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "WARN", "Failed deleting %s: %s", *lb.CurrentManagedInstanceSG, err.Error())
			lb.logger.Warnf("Failed in deletion of managed SG: %s. Continuing remaining deletions, may leave orphaned SGs in AWS.", err.Error())
		} else { // only attempt this SG deletion if the above passed, otherwise it will fail due to depenencies.
			if err := attemptSGDeletion(lb.CurrentManagedSG); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "WARN", "Failed deleting %s: %s", *lb.CurrentManagedSG, err.Error())
				lb.logger.Warnf("Failed in deletion of managed SG: %s. Continuing remaining deletions, may leave orphaned SG in AWS.", err.Error())
			}
		}

	}

	lb.Deleted = true
	return nil
}

// attemptSGDeletion makes a few attempts to remove an SG. If it cannot due to DependencyViolations
// it reattempts in 10 seconds. For up to 2 minutes.
func attemptSGDeletion(sg *string) error {
	// Possible a DependencyViolation will be seen, make a few attempts incase
	var rErr error
	for i := 0; i < 6; i++ {
		time.Sleep(20 * time.Second)
		if err := ec2.EC2svc.DeleteSecurityGroupByID(sg); err != nil {
			rErr = err
			if aerr, ok := err.(awserr.Error); ok {
				if aerr.Code() == "DependencyViolation" {
					continue
				}
			}
		} else { // success, no AWS err occured
			rErr = nil
		}
		break
	}
	return rErr
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

	if lb.CurrentPorts != nil && lb.CurrentManagedSG != nil {
		sort.Sort(lb.CurrentPorts)
		sort.Sort(lb.DesiredPorts)
		if !reflect.DeepEqual(lb.DesiredPorts, lb.CurrentPorts) {

			changes |= managedSecurityGroupsModified
		}
	} else {
		currentSecurityGroups := util.AWSStringSlice(lb.Current.SecurityGroups)
		desiredSecurityGroups := util.AWSStringSlice(lb.Desired.SecurityGroups)
		sort.Sort(currentSecurityGroups)
		sort.Sort(desiredSecurityGroups)
		if log.Prettify(currentSecurityGroups) != log.Prettify(desiredSecurityGroups) {
			changes |= securityGroupsModified
		}
	}

	sort.Sort(lb.CurrentTags)
	sort.Sort(lb.DesiredTags)
	if log.Prettify(lb.CurrentTags) != log.Prettify(lb.DesiredTags) {
		changes |= tagsModified
	}

	if lb.DesiredIdleTimeout > 0 && lb.CurrentIdleTimeout != nil && *lb.CurrentIdleTimeout != lb.DesiredIdleTimeout {
		changes |= connectionIdleTimeoutModified
	}
	currentAttributes := albelbv2.Attributes{Items: lb.CurrentAttributes}
	desiredAttributes := albelbv2.Attributes{Items: lb.DesiredAttributes}
	sort.Sort(currentAttributes)
	sort.Sort(desiredAttributes)
	if log.Prettify(currentAttributes) != log.Prettify(desiredAttributes) {
		changes |= attributesModified
	}

	return changes, true
}

// StripDesiredState removes the DesiredLoadBalancer from the LoadBalancer
func (l *LoadBalancer) StripDesiredState() {
	l.Desired = nil
	l.DesiredPorts = nil
	l.DesiredManagedSG = nil
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

func createLBName(namespace string, ingressName string, clustername string) string {
	hasher := md5.New()
	hasher.Write([]byte(namespace + ingressName))
	hash := hex.EncodeToString(hasher.Sum(nil))[:4]
	name := fmt.Sprintf("%s-%s-%s",
		clustername,
		strings.Replace(namespace, "-", "", -1),
		strings.Replace(ingressName, "-", "", -1),
	)
	if len(name) > 26 {
		name = name[:26]
	}
	name = name + "-" + hash
	return name
}
