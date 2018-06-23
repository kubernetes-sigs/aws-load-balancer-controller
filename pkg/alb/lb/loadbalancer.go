package lb

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"time"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/alb/ls"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/ec2"
	albelbv2 "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/waf"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	api "k8s.io/api/core/v1"
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
	IngressRules         []extensions.IngressRule
	GetServiceNodePort   func(string, int32) (*int64, error)
	GetNodes             func() util.AWSStringSlice
}

// NewDesiredLoadBalancer returns a new loadbalancer.LoadBalancer based on the opts provided.
func NewDesiredLoadBalancer(o *NewDesiredLoadBalancerOptions) (newLoadBalancer *LoadBalancer, err error) {
	// TODO: LB name must not begin with a hyphen.
	name := createLBName(o.Namespace, o.IngressName, o.ALBNamePrefix)

	newLoadBalancer = &LoadBalancer{
		id:         name,
		attributes: attributes{desired: o.Annotations.Attributes},
		tags:       tags{desired: o.Tags},
		options: options{
			desired: opts{
				idleTimeout: o.Annotations.ConnectionIdleTimeout,
				wafACLID:    o.Annotations.WafACLID,
			},
		},
		lb: lb{
			desired: &elbv2.LoadBalancer{
				AvailabilityZones: o.Annotations.Subnets.AsAvailabilityZones(),
				LoadBalancerName:  aws.String(name),
				Scheme:            o.Annotations.Scheme,
				IpAddressType:     o.Annotations.IPAddressType,
				SecurityGroups:    o.Annotations.SecurityGroups,
				VpcId:             o.Annotations.VPCID,
			},
		},
		logger: o.Logger,
	}

	lsps := portList{}
	for _, port := range o.Annotations.Ports {
		lsps = append(lsps, port.Port)
	}

	if len(newLoadBalancer.lb.desired.SecurityGroups) == 0 {
		newLoadBalancer.options.desired.ports = lsps
		newLoadBalancer.options.desired.inboundCidrs = o.Annotations.InboundCidrs
	}

	var existingtgs tg.TargetGroups
	var existingls ls.Listeners
	existinglb := o.ExistingLoadBalancer

	if existinglb != nil {
		// we had an existing LoadBalancer in ingress, so just copy the desired state over
		existinglb.lb.desired = newLoadBalancer.lb.desired
		existinglb.tags.desired = newLoadBalancer.tags.desired
		existinglb.options.desired.idleTimeout = newLoadBalancer.options.desired.idleTimeout
		existinglb.options.desired.wafACLID = newLoadBalancer.options.desired.wafACLID
		if len(o.ExistingLoadBalancer.lb.desired.SecurityGroups) == 0 {
			existinglb.options.desired.ports = lsps
			existinglb.options.desired.inboundCidrs = o.Annotations.InboundCidrs
		}

		newLoadBalancer = existinglb
		existingtgs = existinglb.targetgroups
		existingls = existinglb.listeners
	}

	// Assemble the target groups
	newLoadBalancer.targetgroups, err = tg.NewDesiredTargetGroups(&tg.NewDesiredTargetGroupsOptions{
		IngressRules:         o.IngressRules,
		LoadBalancerID:       newLoadBalancer.id,
		ExistingTargetGroups: existingtgs,
		Annotations:          o.Annotations,
		ALBNamePrefix:        o.ALBNamePrefix,
		Namespace:            o.Namespace,
		Tags:                 o.Tags,
		Logger:               o.Logger,
		GetServiceNodePort:   o.GetServiceNodePort,
		GetNodes:             o.GetNodes,
	})

	if err != nil {
		return newLoadBalancer, err
	}

	// Assemble the listeners
	newLoadBalancer.listeners, err = ls.NewDesiredListeners(&ls.NewDesiredListenersOptions{
		IngressRules:      o.IngressRules,
		ExistingListeners: existingls,
		Annotations:       o.Annotations,
		Logger:            o.Logger,
	})

	return newLoadBalancer, err
}

type NewCurrentLoadBalancerOptions struct {
	LoadBalancer          *elbv2.LoadBalancer
	Tags                  util.Tags
	ALBNamePrefix         string
	Logger                *log.Logger
	ManagedSG             *string
	ManagedInstanceSG     *string
	ManagedSGPorts        []int64
	ManagedSGInboundCidrs []*string
	ConnectionIdleTimeout *int64
	WafACLID              *string
}

// NewCurrentLoadBalancer returns a new loadbalancer.LoadBalancer based on an elbv2.LoadBalancer.
func NewCurrentLoadBalancer(o *NewCurrentLoadBalancerOptions) (newLoadBalancer *LoadBalancer, err error) {
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

	newLoadBalancer = &LoadBalancer{
		id:     name,
		tags:   tags{current: o.Tags},
		lb:     lb{current: o.LoadBalancer},
		logger: o.Logger,
		options: options{current: opts{
			managedSG:         o.ManagedSG,
			inboundCidrs:      o.ManagedSGInboundCidrs,
			ports:             o.ManagedSGPorts,
			managedInstanceSG: o.ManagedInstanceSG,
			idleTimeout:       o.ConnectionIdleTimeout,
			wafACLID:          o.WafACLID,
		},
		},
	}

	// Assemble target groups
	targetGroups, err := albelbv2.ELBV2svc.DescribeTargetGroupsForLoadBalancer(o.LoadBalancer.LoadBalancerArn)
	if err != nil {
		return newLoadBalancer, err
	}

	newLoadBalancer.targetgroups, err = tg.NewCurrentTargetGroups(&tg.NewCurrentTargetGroupsOptions{
		TargetGroups:   targetGroups,
		ALBNamePrefix:  o.ALBNamePrefix,
		LoadBalancerID: newLoadBalancer.id,
		Logger:         o.Logger,
	})
	if err != nil {
		return newLoadBalancer, err
	}

	// Assemble listeners
	listeners, err := albelbv2.ELBV2svc.DescribeListenersForLoadBalancer(o.LoadBalancer.LoadBalancerArn)
	if err != nil {
		return newLoadBalancer, err
	}

	newLoadBalancer.listeners, err = ls.NewCurrentListeners(&ls.NewCurrentListenersOptions{
		TargetGroups: newLoadBalancer.targetgroups,
		Listeners:    listeners,
		Logger:       o.Logger,
	})
	if err != nil {
		return newLoadBalancer, err
	}

	return newLoadBalancer, err
}

// Reconcile compares the current and desired state of this LoadBalancer instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS ELBV2 (ALB) to
// satisfy the ingress's current state.
// TODO evaluate consistency of returning errors
func (l *LoadBalancer) Reconcile(rOpts *ReconcileOptions) []error {
	var errors []error
	lbc := l.lb.current
	lbd := l.lb.desired

	switch {
	case lbd == nil: // lb should be deleted
		if lbc == nil {
			break
		}
		l.logger.Infof("Start ELBV2 (ALB) deletion.")
		if err := l.delete(rOpts); err != nil {
			errors = append(errors, err)
			break
		}
		rOpts.Eventf(api.EventTypeNormal, "DELETE", "%s deleted", *lbc.LoadBalancerName)
		l.logger.Infof("Completed ELBV2 (ALB) deletion. Name: %s | ARN: %s",
			*lbc.LoadBalancerName,
			*lbc.LoadBalancerArn)

	case lbc == nil: // lb doesn't exist and should be created
		l.logger.Infof("Start ELBV2 (ALB) creation.")
		if err := l.create(rOpts); err != nil {
			errors = append(errors, err)
			return errors
		}
		lbc = l.lb.current
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s created", *lbc.LoadBalancerName)
		l.logger.Infof("Completed ELBV2 (ALB) creation. Name: %s | ARN: %s",
			*lbc.LoadBalancerName,
			*lbc.LoadBalancerArn)

	default: // check for diff between lb current and desired, modify if necessary
		needsModification, _ := l.needsModification()
		if needsModification == 0 {
			l.logger.Debugf("No modification of ELBV2 (ALB) required.")
			break
		}

		l.logger.Infof("Start ELBV2 (ALB) modification.")
		if err := l.modify(rOpts); err != nil {
			errors = append(errors, err)
			break
		}
		l.logger.Infof("Completed ELBV2 (ALB) modification.")
	}

	tgsOpts := &tg.ReconcileOptions{
		Eventf:            rOpts.Eventf,
		VpcID:             l.lb.current.VpcId,
		ManagedSGInstance: l.options.current.managedInstanceSG,
	}

	tgs, deletedTG, err := l.targetgroups.Reconcile(tgsOpts)
	if err != nil {
		errors = append(errors, err)
		return errors
	}
	l.targetgroups = tgs

	lsOpts := &ls.ReconcileOptions{
		Eventf:          rOpts.Eventf,
		LoadBalancerArn: lbc.LoadBalancerArn,
		TargetGroups:    l.targetgroups,
	}
	if ltnrs, err := l.listeners.Reconcile(lsOpts); err != nil {
		errors = append(errors, err)
	} else {
		l.listeners = ltnrs
	}

	// REFACTOR!
	// This chunk of code has some questionable logic and we should probably move
	// the TG clean up out of here and into tg. I also dont think that lb.listeners < 1 is a valid check
	//
	// Return now if listeners are already deleted, signifies has already been destructed and
	// TG clean-up, based on rules below does not need to occur.
	if len(l.listeners) < 1 {
		for _, t := range deletedTG {
			if err := albelbv2.ELBV2svc.RemoveTargetGroup(t.CurrentARN()); err != nil {
				errors = append(errors, err)
				return errors
			}
			index, _ := l.targetgroups.FindById(t.ID)
			l.targetgroups = append(l.targetgroups[:index], l.targetgroups[index+1:]...)
		}
		return errors
	}
	unusedTGs := l.listeners[0].GetRules().FindUnusedTGs(l.targetgroups)
	for _, t := range unusedTGs {
		if err := albelbv2.ELBV2svc.RemoveTargetGroup(t.CurrentARN()); err != nil {
			errors = append(errors, err)
			return errors
		}
		index, _ := l.targetgroups.FindById(t.ID)
		l.targetgroups = append(l.targetgroups[:index], l.targetgroups[index+1:]...)
	}
	// END REFACTOR

	return errors
}

// reconcileExistingManagedSG checks AWS for an existing SG with that matches the description of what would
// otherwise be created. If an SG is found, it will run an update to ensure the rules are up to date.
func (l *LoadBalancer) reconcileExistingManagedSG() error {
	if len(l.options.desired.ports) < 1 {
		return fmt.Errorf("No ports specified on ingress. Ingress resource may be misconfigured")
	}
	vpcID, err := ec2.EC2svc.GetVPCID()
	if err != nil {
		return err
	}

	sgID, instanceSG, err := ec2.EC2svc.UpdateSGIfNeeded(vpcID, aws.String(l.id), l.options.current.ports, l.options.desired.ports, l.options.current.inboundCidrs, l.options.desired.inboundCidrs)
	if err != nil {
		return err
	}

	// sgID could be nil, if an existing SG didn't exist or it could have a pointer to an sgID in it.
	l.options.desired.managedSG = sgID
	l.options.desired.managedInstanceSG = instanceSG
	return nil
}

// create requests a new ELBV2 (ALB) is created in AWS.
func (l *LoadBalancer) create(rOpts *ReconcileOptions) error {

	// TODO: This whole thing can become a resolveSGs func
	var sgs util.AWSStringSlice
	// check if desired securitygroups are already expressed through annotations
	if len(l.lb.desired.SecurityGroups) > 0 {
		sgs = l.lb.desired.SecurityGroups
	} else {
		l.reconcileExistingManagedSG()
	}
	if l.options.desired.managedSG != nil {
		sgs = append(sgs, l.options.desired.managedSG)

		if l.options.desired.managedInstanceSG == nil {
			vpcID, err := ec2.EC2svc.GetVPCID()
			if err != nil {
				return err
			}
			instSG, err := ec2.EC2svc.CreateNewInstanceSG(aws.String(l.id), l.options.desired.managedSG, vpcID)
			if err != nil {
				return err
			}
			l.options.desired.managedInstanceSG = instSG
		}
	}

	// when sgs are not known, attempt to create them
	if len(sgs) < 1 {
		vpcID, err := ec2.EC2svc.GetVPCID()
		if err != nil {
			return err
		}
		newSG, newInstSG, err := ec2.EC2svc.CreateSecurityGroupFromPorts(vpcID, aws.String(l.id), l.options.desired.ports, l.options.desired.inboundCidrs)
		if err != nil {
			return err
		}
		sgs = append(sgs, newSG)
		l.options.desired.managedSG = newSG
		l.options.desired.managedInstanceSG = newInstSG
	}

	desired := l.lb.desired
	in := &elbv2.CreateLoadBalancerInput{
		Name:           desired.LoadBalancerName,
		Subnets:        util.AvailabilityZones(desired.AvailabilityZones).AsSubnets(),
		Scheme:         desired.Scheme,
		IpAddressType:  desired.IpAddressType,
		Tags:           l.tags.desired,
		SecurityGroups: sgs,
	}

	o, err := albelbv2.ELBV2svc.CreateLoadBalancer(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating %s: %s", *in.Name, err.Error())
		l.logger.Errorf("Failed to create ELBV2 (ALB): %s", err.Error())
		return err
	}

	// lb created. set to current
	l.lb.current = o.LoadBalancers[0]

	// options.desired.idleTimeout is 0 when no annotation was set, thus no modification should be attempted
	// this will result in using the AWS default
	if l.options.desired.idleTimeout != nil && *l.options.desired.idleTimeout > 0 {
		if err := albelbv2.ELBV2svc.SetIdleTimeout(l.lb.current.LoadBalancerArn, *l.options.desired.idleTimeout); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s tag modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
			l.logger.Errorf("Failed ELBV2 (ALB) tag modification: %s", err.Error())
			return err
		}
		l.options.current.idleTimeout = l.options.desired.idleTimeout
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "Set ALB's connection idle timeout to %d", *l.options.current.idleTimeout)
		l.logger.Infof("Connection idle timeout set to %d", *l.options.current.idleTimeout)
	}

	if len(l.attributes.desired) > 0 {
		newAttributes := &elbv2.ModifyLoadBalancerAttributesInput{
			LoadBalancerArn: l.lb.current.LoadBalancerArn,
			Attributes:      l.attributes.desired,
		}

		_, err = albelbv2.ELBV2svc.ModifyLoadBalancerAttributes(newAttributes)
		if err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error adding attributes to %s: %s", *in.Name, err.Error())
			l.logger.Errorf("Failed to modify ELBV2 attributes (ALB): %s", err.Error())
			return err
		}
	}

	if l.options.desired.wafACLID != nil {
		_, err = waf.WAFRegionalsvc.Associate(l.lb.current.LoadBalancerArn, l.options.desired.wafACLID)
		if err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s WAF (%s) association failed: %s", *l.lb.current.LoadBalancerName, l.options.desired.wafACLID, err.Error())
			l.logger.Errorf("Failed ELBV2 (ALB) WAF (%s) association: %s", l.options.desired.wafACLID, err.Error())
			return err
		}
	}

	// when a desired managed sg was present, it was used and should be set as the new options.current.managedSG.
	if l.options.desired.managedSG != nil {
		l.options.current.managedSG = l.options.desired.managedSG
		l.options.current.managedInstanceSG = l.options.desired.managedInstanceSG
		l.options.current.inboundCidrs = l.options.desired.inboundCidrs
		l.options.current.ports = l.options.desired.ports
	}
	return nil
}

// modify modifies the attributes of an existing ALB in AWS.
func (l *LoadBalancer) modify(rOpts *ReconcileOptions) error {
	needsMod, canMod := l.needsModification()
	if canMod {

		// Modify Security Groups
		if needsMod&securityGroupsModified != 0 {
			l.logger.Infof("Start ELBV2 security groups modification.")
			in := &elbv2.SetSecurityGroupsInput{
				LoadBalancerArn: l.lb.current.LoadBalancerArn,
				SecurityGroups:  l.lb.desired.SecurityGroups,
			}
			if _, err := albelbv2.ELBV2svc.SetSecurityGroups(in); err != nil {
				l.logger.Errorf("Failed ELBV2 security groups modification: %s", err.Error())
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s security group modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
				return err
			}
			l.lb.current.SecurityGroups = l.lb.desired.SecurityGroups
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s security group modified", *l.lb.current.LoadBalancerName)
			l.logger.Infof("Completed ELBV2 security groups modification. SGs: %s",
				log.Prettify(l.lb.current.SecurityGroups))
		}

		// Modify ALB-managed security groups
		if needsMod&managedSecurityGroupsModified != 0 {
			l.logger.Infof("Start ELBV2-managed security groups modification.")
			if err := l.reconcileExistingManagedSG(); err != nil {
				l.logger.Errorf("Failed ELBV2-managed security groups modification: %s", err.Error())
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s security group modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
				return err
			}
			l.options.current.inboundCidrs = l.options.desired.inboundCidrs
			l.options.current.ports = l.options.desired.ports
		}

		// Modify Subnets
		if needsMod&subnetsModified != 0 {
			l.logger.Infof("Start subnets modification.")
			in := &elbv2.SetSubnetsInput{
				LoadBalancerArn: l.lb.current.LoadBalancerArn,
				Subnets:         util.AvailabilityZones(l.lb.desired.AvailabilityZones).AsSubnets(),
			}
			if _, err := albelbv2.ELBV2svc.SetSubnets(in); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s subnet modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
				return fmt.Errorf("Failure Setting ALB Subnets: %s", err)
			}
			l.lb.current.AvailabilityZones = l.lb.desired.AvailabilityZones
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s subnets modified", *l.lb.current.LoadBalancerName)
			l.logger.Infof("Completed subnets modification. Subnets are %s.",
				log.Prettify(l.lb.current.AvailabilityZones))
		}

		// Modify IP address type
		if needsMod&ipAddressTypeModified != 0 {
			l.logger.Infof("Start IP address type modification.")
			in := &elbv2.SetIpAddressTypeInput{
				LoadBalancerArn: l.lb.current.LoadBalancerArn,
				IpAddressType:   l.lb.desired.IpAddressType,
			}
			if _, err := albelbv2.ELBV2svc.SetIpAddressType(in); err != nil {
				return fmt.Errorf("Failure Setting ALB IpAddressType: %s", err)
			}
			l.lb.current.IpAddressType = l.lb.desired.IpAddressType
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s ip address type modified", *l.lb.current.LoadBalancerName)
			l.logger.Infof("Completed IP address type modification. Type is %s.", *l.lb.current.LoadBalancerName, *l.lb.current.IpAddressType)
		}

		// Modify Tags
		if needsMod&tagsModified != 0 {
			l.logger.Infof("Start ELBV2 tag modification.")
			if err := albelbv2.ELBV2svc.UpdateTags(l.lb.current.LoadBalancerArn, l.tags.current, l.tags.desired); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s tag modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
				l.logger.Errorf("Failed ELBV2 (ALB) tag modification: %s", err.Error())
			}
			l.tags.current = l.tags.desired
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s tags modified", *l.lb.current.LoadBalancerName)
			l.logger.Infof("Completed ELBV2 tag modification. Tags are %s.", log.Prettify(l.tags.current))
		}

		// Modify Connection Idle Timeout
		if needsMod&connectionIdleTimeoutModified != 0 {
			if l.options.desired.idleTimeout != nil {
				if err := albelbv2.ELBV2svc.SetIdleTimeout(l.lb.current.LoadBalancerArn, *l.options.desired.idleTimeout); err != nil {
					rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s tag modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
					l.logger.Errorf("Failed ELBV2 (ALB) tag modification: %s", err.Error())
					return err
				}
				l.options.current.idleTimeout = l.options.desired.idleTimeout
				rOpts.Eventf(api.EventTypeNormal, "MODIFY", "Connection idle timeout updated to %d", *l.options.current.idleTimeout)
				l.logger.Infof("Connection idle timeout updated to %d", *l.options.current.idleTimeout)
			}
		}

		// Modify Attributes
		if needsMod&attributesModified != 0 {
			l.logger.Infof("Start ELBV2 tag modification.")
			if err := albelbv2.ELBV2svc.UpdateAttributes(l.lb.current.LoadBalancerArn, l.attributes.desired); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s tag modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
				l.logger.Errorf("Failed ELBV2 (ALB) tag modification: %s", err.Error())
				return fmt.Errorf("Failure adding ALB attributes: %s", err)
			}
			l.attributes.current = l.attributes.desired
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s attributes modified", *l.lb.current.LoadBalancerName)
			l.logger.Infof("Completed ELBV2 tag modification. Attributes are %s.", log.Prettify(l.attributes.current))
		}

		if needsMod&wafAssociationModified != 0 {
			if l.options.desired.wafACLID != nil {
				if _, err := waf.WAFRegionalsvc.Associate(l.lb.current.LoadBalancerArn, l.options.desired.wafACLID); err != nil {
					rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s Waf (%s) association failed: %s", *l.lb.current.LoadBalancerName, *l.options.desired.wafACLID, err.Error())
					l.logger.Errorf("Failed ELBV2 (ALB) Waf (%s) association failed: %s", *l.options.desired.wafACLID, err.Error())
				} else {
					l.options.current.wafACLID = l.options.desired.wafACLID
					rOpts.Eventf(api.EventTypeNormal, "MODIFY", "WAF Association updated to %s", *l.options.desired.wafACLID)
					l.logger.Infof("WAF Association updated %s", *l.options.desired.wafACLID)
				}
			} else if l.options.current.wafACLID != nil {
				if _, err := waf.WAFRegionalsvc.Disassociate(l.lb.current.LoadBalancerArn); err != nil {
					rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s Waf disassociation failed: %s", *l.lb.current.LoadBalancerName, err.Error())
					l.logger.Errorf("Failed ELBV2 (ALB) Waf disassociation failed: %s", err.Error())
				} else {
					l.options.current.wafACLID = l.options.desired.wafACLID
					rOpts.Eventf(api.EventTypeNormal, "MODIFY", "WAF Disassociated")
					l.logger.Infof("WAF Disassociated")
				}
			}
		}

	} else {
		// Modification is needed, but required full replacement of ALB.
		l.logger.Infof("Start ELBV2 full modification (delete and create).")
		rOpts.Eventf(api.EventTypeNormal, "REBUILD", "Impossible modification requested, rebuilding %s", *l.lb.current.LoadBalancerName)
		l.delete(rOpts)
		// Since listeners and rules are deleted during lb deletion, ensure their current state is removed
		// as they'll no longer exist.
		l.listeners.StripCurrentState()
		l.create(rOpts)
		l.logger.Infof("Completed ELBV2 full modification (delete and create). Name: %s | ARN: %s",
			*l.lb.current.LoadBalancerName, *l.lb.current.LoadBalancerArn)

	}

	return nil
}

// delete Deletes the load balancer from AWS.
func (l *LoadBalancer) delete(rOpts *ReconcileOptions) error {

	// we need to disassociate the WAF before deletion
	if l.options.current.wafACLID != nil {
		if _, err := waf.WAFRegionalsvc.Disassociate(l.lb.current.LoadBalancerArn); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error disassociating WAF for %s: %s", *l.lb.current.LoadBalancerName, err.Error())
			l.logger.Errorf("Failed disassociation of ELBV2 (ALB) WAF: %s.", err.Error())
			return err
		}
	}

	in := &elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: l.lb.current.LoadBalancerArn,
	}

	if _, err := albelbv2.ELBV2svc.DeleteLoadBalancer(in); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error deleting %s: %s", *l.lb.current.LoadBalancerName, err.Error())
		l.logger.Errorf("Failed deletion of ELBV2 (ALB): %s.", err.Error())
		return err
	}

	// if the alb controller was managing a SG we must:
	// - Remove the InstanceSG from all instances known to targetgroups
	// - Delete the InstanceSG
	// - Delete the ALB's SG
	// Deletions are attempted as best effort, if it fails we log the error but don't
	// fail the overall reconcile
	if l.options.current.managedSG != nil {
		if err := ec2.EC2svc.DisassociateSGFromInstanceIfNeeded(l.targetgroups[0].CurrentTargets(), l.options.current.managedInstanceSG); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "WARN", "Failed disassociating sgs from instances: %s", err.Error())
			l.logger.Warnf("Failed in deletion of managed SG: %s.", err.Error())
		}
		if err := attemptSGDeletion(l.options.current.managedInstanceSG); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "WARN", "Failed deleting %s: %s", *l.options.current.managedInstanceSG, err.Error())
			l.logger.Warnf("Failed in deletion of managed SG: %s. Continuing remaining deletions, may leave orphaned SGs in AWS.", err.Error())
		} else { // only attempt this SG deletion if the above passed, otherwise it will fail due to depenencies.
			if err := attemptSGDeletion(l.options.current.managedSG); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "WARN", "Failed deleting %s: %s", *l.options.current.managedSG, err.Error())
				l.logger.Warnf("Failed in deletion of managed SG: %s. Continuing remaining deletions, may leave orphaned SG in AWS.", err.Error())
			}
		}

	}

	l.deleted = true
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
func (l *LoadBalancer) needsModification() (loadBalancerChange, bool) {
	var changes loadBalancerChange

	clb := l.lb.current
	dlb := l.lb.desired
	copts := l.options.current
	dopts := l.options.desired

	// In the case that the LB does not exist yet
	if clb == nil {
		l.logger.Debugf("Current Load Balancer is undefined")
		return changes, true
	}

	if !util.DeepEqual(clb.Scheme, dlb.Scheme) {
		l.logger.Debugf("Scheme needs to be changed (%v != %v)", log.Prettify(clb.Scheme), log.Prettify(dlb.Scheme))
		changes |= schemeModified
		return changes, false
	}

	if !util.DeepEqual(clb.IpAddressType, dlb.IpAddressType) {
		l.logger.Debugf("IpAddressType needs to be changed (%v != %v)", log.Prettify(clb.IpAddressType), log.Prettify(dlb.IpAddressType))
		changes |= ipAddressTypeModified
		return changes, true
	}

	currentSubnets := util.AvailabilityZones(clb.AvailabilityZones).AsSubnets()
	desiredSubnets := util.AvailabilityZones(dlb.AvailabilityZones).AsSubnets()
	sort.Sort(currentSubnets)
	sort.Sort(desiredSubnets)
	if log.Prettify(currentSubnets) != log.Prettify(desiredSubnets) {
		l.logger.Debugf("AvailabilityZones needs to be changed (%v != %v)", log.Prettify(currentSubnets), log.Prettify(desiredSubnets))
		changes |= subnetsModified
	}

	if copts.ports != nil && copts.managedSG != nil {
		if util.AWSStringSlice(copts.inboundCidrs).Hash() != util.AWSStringSlice(dopts.inboundCidrs).Hash() {
			l.logger.Debugf("InboundCidrs needs to be changed (%v != %v)", log.Prettify(copts.inboundCidrs), log.Prettify(dopts.inboundCidrs))
			changes |= managedSecurityGroupsModified
		}

		sort.Sort(l.options.current.ports)
		sort.Sort(l.options.desired.ports)
		if !reflect.DeepEqual(l.options.desired.ports, l.options.current.ports) {
			l.logger.Debugf("Ports needs to be changed (%v != %v)", log.Prettify(copts.ports), log.Prettify(dopts.ports))
			changes |= managedSecurityGroupsModified
		}
	} else {
		if util.AWSStringSlice(clb.SecurityGroups).Hash() != util.AWSStringSlice(dlb.SecurityGroups).Hash() {
			l.logger.Debugf("SecurityGroups needs to be changed (%v != %v)", log.Prettify(clb.SecurityGroups), log.Prettify(dlb.SecurityGroups))
			changes |= securityGroupsModified
		}
	}

	if l.tags.current.Hash() != l.tags.desired.Hash() {
		l.logger.Debugf("Tags need to be changed")
		changes |= tagsModified
	}

	if dopts.idleTimeout != nil && copts.idleTimeout != nil &&
		*dopts.idleTimeout > 0 && *copts.idleTimeout != *dopts.idleTimeout {
		l.logger.Debugf("IdleTimeout needs to be changed (%v != %v)", log.Prettify(copts.idleTimeout), log.Prettify(dopts.idleTimeout))
		changes |= connectionIdleTimeoutModified
	}

	if log.Prettify(l.attributes.current.Sorted()) != log.Prettify(l.attributes.desired.Sorted()) {
		l.logger.Debugf("Attributes need to be changed")
		changes |= attributesModified
	}

	if dopts.wafACLID != nil && copts.wafACLID == nil ||
		dopts.wafACLID == nil && copts.wafACLID != nil ||
		(copts.wafACLID != nil && dopts.wafACLID != nil && *copts.wafACLID != *dopts.wafACLID) {
		l.logger.Debugf("WAF needs to be changed: (%v != %v)", log.Prettify(copts.wafACLID), log.Prettify(dopts.wafACLID))
		changes |= wafAssociationModified
	}
	return changes, true
}

// StripDesiredState removes the DesiredLoadBalancer from the LoadBalancer
func (l *LoadBalancer) StripDesiredState() {
	l.lb.desired = nil
	l.options.desired.ports = nil
	l.options.desired.managedSG = nil
	l.options.desired.wafACLID = nil
	if l.listeners != nil {
		l.listeners.StripDesiredState()
	}
	if l.targetgroups != nil {
		l.targetgroups.StripDesiredState()
	}
}

func createLBName(namespace string, ingressName string, clustername string) string {
	hasher := md5.New()
	hasher.Write([]byte(namespace + ingressName))
	hash := hex.EncodeToString(hasher.Sum(nil))[:4]

	r, _ := regexp.Compile("[[:^alnum:]]")
	name := fmt.Sprintf("%s-%s-%s",
		r.ReplaceAllString(clustername, "-"),
		r.ReplaceAllString(namespace, ""),
		r.ReplaceAllString(ingressName, ""),
	)
	if len(name) > 26 {
		name = name[:26]
	}
	name = name + "-" + hash
	return name
}

// Hostname returns the AWS hostname of the load balancer
func (l *LoadBalancer) Hostname() *string {
	return l.lb.current.DNSName
}

// IsDeleted returns if the load balancer has been deleted
func (l *LoadBalancer) IsDeleted() bool {
	return l.deleted
}
