package lb

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albwafregional"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/ls"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	api "k8s.io/api/core/v1"
)

type NewDesiredLoadBalancerOptions struct {
	ExistingLoadBalancer *LoadBalancer
	Logger               *log.Logger
	Store                store.Storer
	Ingress              *extensions.Ingress
	CommonTags           util.ELBv2Tags
}

// NewDesiredLoadBalancer returns a new loadbalancer.LoadBalancer based on the opts provided.
func NewDesiredLoadBalancer(o *NewDesiredLoadBalancerOptions) (newLoadBalancer *LoadBalancer, err error) {
	name := createLBName(o.Ingress.Namespace, o.Ingress.Name, o.Store.GetConfig().ALBNamePrefix)

	lbTags := o.CommonTags.Copy()

	vpc, err := albec2.EC2svc.GetVPCID()
	if err != nil {
		return nil, err
	}

	annos, err := o.Store.GetIngressAnnotations(k8s.MetaNamespaceKey(o.Ingress))
	if err != nil {
		return nil, err
	}

	newLoadBalancer = &LoadBalancer{
		id:         name,
		attributes: attributes{desired: annos.LoadBalancer.Attributes},
		tags:       tags{desired: lbTags},
		options: options{
			desired: opts{
				webACLId: annos.LoadBalancer.WebACLId,
			},
		},
		lb: lb{
			desired: &elbv2.LoadBalancer{
				AvailabilityZones: annos.LoadBalancer.Subnets.AsAvailabilityZones(),
				LoadBalancerName:  aws.String(name),
				Scheme:            annos.LoadBalancer.Scheme,
				IpAddressType:     annos.LoadBalancer.IPAddressType,
				SecurityGroups:    annos.LoadBalancer.SecurityGroups,
				VpcId:             vpc,
			},
		},
		logger: o.Logger,
	}

	lsps := portList{}
	for _, port := range annos.LoadBalancer.Ports {
		lsps = append(lsps, port.Port)
	}

	if len(annos.LoadBalancer.SecurityGroups) == 0 {
		newLoadBalancer.options.desired.ports = lsps
		newLoadBalancer.options.desired.inboundCidrs = annos.LoadBalancer.InboundCidrs
	}

	var existingtgs tg.TargetGroups
	var existingls ls.Listeners
	existinglb := o.ExistingLoadBalancer

	if existinglb != nil {
		// we had an existing LoadBalancer in ingress, so just copy the desired state over
		existinglb.lb.desired = newLoadBalancer.lb.desired
		existinglb.tags.desired = newLoadBalancer.tags.desired
		existinglb.attributes.desired = newLoadBalancer.attributes.desired
		existinglb.options.desired.webACLId = newLoadBalancer.options.desired.webACLId
		if len(o.ExistingLoadBalancer.lb.desired.SecurityGroups) == 0 {
			existinglb.options.desired.ports = lsps
			existinglb.options.desired.inboundCidrs = annos.LoadBalancer.InboundCidrs
		}

		newLoadBalancer = existinglb
		existingtgs = existinglb.targetgroups
		existingls = existinglb.listeners
	}

	// Assemble the target groups
	newLoadBalancer.targetgroups, err = tg.NewDesiredTargetGroups(&tg.NewDesiredTargetGroupsOptions{
		Ingress:              o.Ingress,
		LoadBalancerID:       newLoadBalancer.id,
		ExistingTargetGroups: existingtgs,
		Store:                o.Store,
		CommonTags:           o.CommonTags,
		Logger:               o.Logger,
	})

	if err != nil {
		return newLoadBalancer, err
	}

	// Assemble the listeners
	newLoadBalancer.listeners, err = ls.NewDesiredListeners(&ls.NewDesiredListenersOptions{
		Ingress:           o.Ingress,
		Store:             o.Store,
		ExistingListeners: existingls,
		TargetGroups:      newLoadBalancer.targetgroups,
		Logger:            o.Logger,
	})

	return newLoadBalancer, err
}

type NewCurrentLoadBalancerOptions struct {
	LoadBalancer *elbv2.LoadBalancer
	TargetGroups map[string][]*elbv2.TargetGroup
	Logger       *log.Logger
}

// NewCurrentLoadBalancer returns a new loadbalancer.LoadBalancer based on an elbv2.LoadBalancer.
func NewCurrentLoadBalancer(o *NewCurrentLoadBalancerOptions) (newLoadBalancer *LoadBalancer, err error) {
	attrs, err := albelbv2.ELBV2svc.DescribeLoadBalancerAttributesFiltered(o.LoadBalancer.LoadBalancerArn)
	if err != nil {
		return newLoadBalancer, fmt.Errorf("Failed to retrieve attributes from ELBV2 in AWS. Error: %s", err.Error())
	}

	var managedSG *string
	var managedInstanceSG *string
	managedSGInboundCidrs := []*string{}
	managedSGPorts := []int64{}
	if len(o.LoadBalancer.SecurityGroups) == 1 {
		tags, err := albec2.EC2svc.DescribeSGTags(o.LoadBalancer.SecurityGroups[0])
		if err != nil {
			return newLoadBalancer, fmt.Errorf("Failed to describe security group tags of load balancer. Error: %s", err.Error())
		}

		for _, tag := range tags {
			// If the subnet is labeled as managed by ALB, capture it as the managedSG
			if *tag.Key == albec2.ManagedByKey && *tag.Value == albec2.ManagedByValue {
				managedSG = o.LoadBalancer.SecurityGroups[0]
				managedSGPorts, err = albec2.EC2svc.DescribeSGPorts(o.LoadBalancer.SecurityGroups[0])
				if err != nil {
					return newLoadBalancer, fmt.Errorf("Failed to describe ports of managed security group. Error: %s", err.Error())
				}

				managedSGInboundCidrs, err = albec2.EC2svc.DescribeSGInboundCidrs(o.LoadBalancer.SecurityGroups[0])
				if err != nil {
					return newLoadBalancer, fmt.Errorf("Failed to describe ingress ipv4 ranges of managed security group. Error: %s", err.Error())
				}
			}
		}
		// when a alb-managed SG existed, we must find a correlated instance SG
		if managedSG != nil {
			managedInstanceSG, err = albec2.EC2svc.DescribeSGByPermissionGroup(managedSG)
			if err != nil {
				return newLoadBalancer, fmt.Errorf("Failed to find related managed instance SG. Was it deleted from AWS? Error: %s", err.Error())
			}
		}
	}

	// Check WAF
	webACLResult, err := albwafregional.WAFRegionalsvc.GetWebACLSummary(o.LoadBalancer.LoadBalancerArn)
	if err != nil {
		return newLoadBalancer, fmt.Errorf("Failed to get associated Web ACL. Error: %s", err.Error())
	}
	var webACLId *string
	if webACLResult != nil {
		webACLId = webACLResult.WebACLId
	}

	resourceTags, err := albrgt.RGTsvc.GetClusterResources()
	if err != nil {
		return newLoadBalancer, fmt.Errorf("Failed to get AWS tags. Error: %s", err.Error())
	}

	newLoadBalancer = &LoadBalancer{
		id:         *o.LoadBalancer.LoadBalancerName,
		tags:       tags{current: resourceTags.LoadBalancers[*o.LoadBalancer.LoadBalancerArn]},
		lb:         lb{current: o.LoadBalancer},
		logger:     o.Logger,
		attributes: attributes{current: attrs},
		options: options{current: opts{
			managedSG:         managedSG,
			inboundCidrs:      managedSGInboundCidrs,
			ports:             managedSGPorts,
			managedInstanceSG: managedInstanceSG,
			webACLId:          webACLId,
		},
		},
	}

	// Assemble target groups
	targetGroups := o.TargetGroups[*o.LoadBalancer.LoadBalancerArn]

	newLoadBalancer.targetgroups, err = tg.NewCurrentTargetGroups(&tg.NewCurrentTargetGroupsOptions{
		TargetGroups:   targetGroups,
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
// results in no action, the creation, the deletion, or the modification of an AWS ELBV2 to
// satisfy the ingress's current state.
func (l *LoadBalancer) Reconcile(rOpts *ReconcileOptions) []error {
	var errors []error
	lbc := l.lb.current
	lbd := l.lb.desired

	switch {
	case lbd == nil: // lb should be deleted
		if lbc == nil {
			break
		}
		l.logger.Infof("Start ELBV2 deletion.")
		if err := l.delete(rOpts); err != nil {
			errors = append(errors, err)
			break
		}
		rOpts.Eventf(api.EventTypeNormal, "DELETE", "%s deleted", *lbc.LoadBalancerName)
		l.logger.Infof("Completed ELBV2 deletion. Name: %s | ARN: %s",
			*lbc.LoadBalancerName,
			*lbc.LoadBalancerArn)

	case lbc == nil: // lb doesn't exist and should be created
		l.logger.Infof("Start ELBV2 creation.")
		if err := l.create(rOpts); err != nil {
			errors = append(errors, err)
			return errors
		}
		lbc = l.lb.current
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s created", *lbc.LoadBalancerName)
		l.logger.Infof("Completed ELBV2 creation. Name: %s | ARN: %s",
			*lbc.LoadBalancerName,
			*lbc.LoadBalancerArn)

	default: // check for diff between lb current and desired, modify if necessary
		if err := l.modify(rOpts); err != nil {
			errors = append(errors, err)
			break
		}
	}

	tgsOpts := &tg.ReconcileOptions{
		Store:             rOpts.Store,
		Eventf:            rOpts.Eventf,
		VpcID:             l.lb.current.VpcId,
		ManagedSGInstance: l.options.current.managedInstanceSG,
		IgnoreDeletes:     true,
	}

	// Creates target groups
	tgs, err := l.targetgroups.Reconcile(tgsOpts)
	if err != nil {
		errors = append(errors, err)
	} else {
		l.targetgroups = tgs
	}

	lsOpts := &ls.ReconcileOptions{
		Eventf:          rOpts.Eventf,
		LoadBalancerArn: lbc.LoadBalancerArn,
		TargetGroups:    l.targetgroups,
		Ingress:         rOpts.Ingress,
		Store:           rOpts.Store,
	}
	if ltnrs, err := l.listeners.Reconcile(lsOpts); err != nil {
		errors = append(errors, err)
	} else {
		l.listeners = ltnrs
	}

	// Does not consider TG used for listener default action
	for _, listener := range l.listeners {
		unusedTGs := listener.GetRules().FindUnusedTGs(l.targetgroups, listener.DefaultActionArn())
		unusedTGs.StripDesiredState()
	}

	// removes target groups
	tgsOpts.IgnoreDeletes = false
	tgs, err = l.targetgroups.Reconcile(tgsOpts)
	if err != nil {
		errors = append(errors, err)
	} else {
		l.targetgroups = tgs
	}

	return errors
}

// reconcileExistingManagedSG checks AWS for an existing SG with that matches the description of what would
// otherwise be created. If an SG is found, it will run an update to ensure the rules are up to date.
func (l *LoadBalancer) reconcileExistingManagedSG() error {
	if len(l.options.desired.ports) < 1 {
		return fmt.Errorf("No ports specified on ingress. Ingress resource may be misconfigured")
	}
	vpcID, err := albec2.EC2svc.GetVPCID()
	if err != nil {
		return err
	}

	sgID, instanceSG, err := albec2.EC2svc.UpdateSGIfNeeded(vpcID, aws.String(l.id), l.options.current.ports, l.options.desired.ports, l.options.current.inboundCidrs, l.options.desired.inboundCidrs)
	if err != nil {
		return err
	}

	// sgID could be nil, if an existing SG didn't exist or it could have a pointer to an sgID in it.
	l.options.desired.managedSG = sgID
	l.options.desired.managedInstanceSG = instanceSG
	return nil
}

// create requests a new ELBV2 is created in AWS.
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
			vpcID, err := albec2.EC2svc.GetVPCID()
			if err != nil {
				return err
			}
			instSG, err := albec2.EC2svc.CreateNewInstanceSG(aws.String(l.id), l.options.desired.managedSG, vpcID)
			if err != nil {
				return err
			}
			l.options.desired.managedInstanceSG = instSG
		}
	}

	// when sgs are not known, attempt to create them
	if len(sgs) < 1 {
		vpcID, err := albec2.EC2svc.GetVPCID()
		if err != nil {
			return err
		}
		newSG, newInstSG, err := albec2.EC2svc.CreateSecurityGroupFromPorts(vpcID, aws.String(l.id), l.options.desired.ports, l.options.desired.inboundCidrs)
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
		l.logger.Errorf("Failed to create ELBV2: %s", err.Error())
		return err
	}

	// lb created. set to current
	l.lb.current = o.LoadBalancers[0]

	if len(l.attributes.desired) > 0 {
		newAttributes := &elbv2.ModifyLoadBalancerAttributesInput{
			LoadBalancerArn: l.lb.current.LoadBalancerArn,
			Attributes:      l.attributes.desired,
		}

		_, err = albelbv2.ELBV2svc.ModifyLoadBalancerAttributes(newAttributes)
		if err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error adding attributes to %s: %s", *in.Name, err.Error())
			l.logger.Errorf("Failed to add ELBV2 attributes: %s", err.Error())
			return err
		}
	}

	if l.options.desired.webACLId != nil {
		_, err = albwafregional.WAFRegionalsvc.Associate(l.lb.current.LoadBalancerArn, l.options.desired.webACLId)
		if err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s Web ACL (%s) association failed: %s", *l.lb.current.LoadBalancerName, l.options.desired.webACLId, err.Error())
			l.logger.Errorf("Failed setting Web ACL (%s) association: %s", l.options.desired.webACLId, err.Error())
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
	if needsMod == 0 {
		return nil
	}

	if canMod {
		// Modify Security Groups
		if needsMod&securityGroupsModified != 0 {
			l.logger.Infof("Modifying ELBV2 security groups.")
			if _, err := albelbv2.ELBV2svc.SetSecurityGroups(&elbv2.SetSecurityGroupsInput{
				LoadBalancerArn: l.lb.current.LoadBalancerArn,
				SecurityGroups:  l.lb.desired.SecurityGroups,
			}); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s security group modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
				return fmt.Errorf("Failed ELBV2 security groups modification: %s", err.Error())
			}
			l.lb.current.SecurityGroups = l.lb.desired.SecurityGroups
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s security group modified", *l.lb.current.LoadBalancerName)
		}

		// Modify ALB-managed security groups
		if needsMod&portsModified != 0 || needsMod&inboundCidrsModified != 0 {
			l.logger.Infof("Modifying ELBV2-managed security groups.")
			if err := l.reconcileExistingManagedSG(); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s security group modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
				return fmt.Errorf("Failed ELBV2-managed security groups modification: %s", err.Error())
			}
			l.options.current.inboundCidrs = l.options.desired.inboundCidrs
			l.options.current.ports = l.options.desired.ports
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s managed security groups modified", *l.lb.current.LoadBalancerName)
		}

		// Modify Subnets
		if needsMod&subnetsModified != 0 {
			l.logger.Infof("Modifying ELBV2 subnets to %v.", log.Prettify(l.lb.current.AvailabilityZones))
			if _, err := albelbv2.ELBV2svc.SetSubnets(&elbv2.SetSubnetsInput{
				LoadBalancerArn: l.lb.current.LoadBalancerArn,
				Subnets:         util.AvailabilityZones(l.lb.desired.AvailabilityZones).AsSubnets(),
			}); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s subnet modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
				return fmt.Errorf("Failed setting ELBV2 subnets: %s", err)
			}
			l.lb.current.AvailabilityZones = l.lb.desired.AvailabilityZones
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s subnets modified", *l.lb.current.LoadBalancerName)
		}

		// Modify IP address type
		if needsMod&ipAddressTypeModified != 0 {
			l.logger.Infof("Modifying IP address type modification to %v.", *l.lb.current.IpAddressType)
			if _, err := albelbv2.ELBV2svc.SetIpAddressType(&elbv2.SetIpAddressTypeInput{
				LoadBalancerArn: l.lb.current.LoadBalancerArn,
				IpAddressType:   l.lb.desired.IpAddressType,
			}); err != nil {
				rOpts.Eventf(api.EventTypeNormal, "ERROR", "%s ip address type modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
				return fmt.Errorf("Failed setting ELBV2 IP address type: %s", err)
			}
			l.lb.current.IpAddressType = l.lb.desired.IpAddressType
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s ip address type modified", *l.lb.current.LoadBalancerName)
		}

		// Modify Tags
		if needsMod&tagsModified != 0 {
			l.logger.Infof("Modifying ELBV2 tags to %v.", log.Prettify(l.tags.desired))
			if err := albelbv2.ELBV2svc.UpdateTags(l.lb.current.LoadBalancerArn, l.tags.current, l.tags.desired); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s tag modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
				return fmt.Errorf("Failed ELBV2 tag modification: %s", err.Error())
			}
			l.tags.current = l.tags.desired
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s tags modified", *l.lb.current.LoadBalancerName)
		}

		// Modify Attributes
		if needsMod&attributesModified != 0 {
			l.logger.Infof("Modifying ELBV2 attributes to %v.", log.Prettify(l.attributes.desired))
			if _, err := albelbv2.ELBV2svc.ModifyLoadBalancerAttributes(&elbv2.ModifyLoadBalancerAttributesInput{
				LoadBalancerArn: l.lb.current.LoadBalancerArn,
				Attributes:      l.attributes.desired,
			}); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s attributes modification failed: %s", *l.lb.current.LoadBalancerName, err.Error())
				return fmt.Errorf("Failed modifying attributes: %s", err)
			}
			l.attributes.current = l.attributes.desired
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s attributes modified", *l.lb.current.LoadBalancerName)
		}

		// Modify Web ACL
		if needsMod&webACLAssociationModified != 0 {
			if l.options.desired.webACLId != nil { // Associate
				l.logger.Infof("Associating %v Web ACL.", *l.options.desired.webACLId)
				if _, err := albwafregional.WAFRegionalsvc.Associate(l.lb.current.LoadBalancerArn, l.options.desired.webACLId); err != nil {
					rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s Web ACL (%s) association failed: %s", *l.lb.current.LoadBalancerName, *l.options.desired.webACLId, err.Error())
					return fmt.Errorf("Failed associating Web ACL: %s", err.Error())
				}
				l.options.current.webACLId = l.options.desired.webACLId
				rOpts.Eventf(api.EventTypeNormal, "MODIFY", "Web ACL association updated to %s", *l.options.desired.webACLId)
			} else { // Disassociate
				l.logger.Infof("Disassociating Web ACL.")
				if _, err := albwafregional.WAFRegionalsvc.Disassociate(l.lb.current.LoadBalancerArn); err != nil {
					rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s Web ACL disassociation failed: %s", *l.lb.current.LoadBalancerName, err.Error())
					return fmt.Errorf("Failed removing Web ACL association: %s", err.Error())
				}
				l.options.current.webACLId = l.options.desired.webACLId
				rOpts.Eventf(api.EventTypeNormal, "MODIFY", "Web ACL disassociated")
			}
		}

	} else {
		// Modification is needed, but required full replacement of ALB.
		// TODO improve this process, it generally fails some deletions and completes in the next sync
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
	if l.options.current.webACLId != nil {
		if _, err := albwafregional.WAFRegionalsvc.Disassociate(l.lb.current.LoadBalancerArn); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error disassociating Web ACL for %s: %s", *l.lb.current.LoadBalancerName, err.Error())
			return fmt.Errorf("Failed disassociation of ELBV2 Web ACL: %s.", err.Error())
		}
	}

	in := &elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: l.lb.current.LoadBalancerArn,
	}

	if _, err := albelbv2.ELBV2svc.DeleteLoadBalancer(in); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error deleting %s: %s", *l.lb.current.LoadBalancerName, err.Error())
		return fmt.Errorf("Failed deletion of ELBV2: %s.", err.Error())
	}

	// if the alb controller was managing a SG we must:
	// - Remove the InstanceSG from all instances known to targetgroups
	// - Delete the InstanceSG
	// - Delete the ALB's SG
	// Deletions are attempted as best effort, if it fails we log the error but don't
	// fail the overall reconcile
	if l.options.current.managedSG != nil {
		if err := albec2.EC2svc.DisassociateSGFromInstanceIfNeeded(l.targetgroups[0].CurrentTargets().InstanceIds(rOpts.Store), l.options.current.managedInstanceSG); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "WARN", "Failed disassociating sgs from instances: %s", err.Error())
			return fmt.Errorf("Failed disassociating managed SG: %s.", err.Error())
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
		if err := albec2.EC2svc.DeleteSecurityGroupByID(*sg); err != nil {
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
	}

	currentSubnets := util.AvailabilityZones(clb.AvailabilityZones).AsSubnets()
	desiredSubnets := util.AvailabilityZones(dlb.AvailabilityZones).AsSubnets()
	sort.Sort(currentSubnets)
	sort.Sort(desiredSubnets)
	if log.Prettify(currentSubnets) != log.Prettify(desiredSubnets) {
		l.logger.Debugf("AvailabilityZones needs to be changed (%v != %v)", log.Prettify(currentSubnets), log.Prettify(desiredSubnets))
		changes |= subnetsModified
	}

	if l.tags.needsModification() {
		l.logger.Debugf("Tags need to be changed")
		changes |= tagsModified
	}

	if l.attributes.needsModification() {
		l.logger.Debugf("Attributes need to be changed")
		changes |= attributesModified
	}

	if copts.managedSG == nil && util.AWSStringSlice(clb.SecurityGroups).Hash() != util.AWSStringSlice(dlb.SecurityGroups).Hash() {
		l.logger.Debugf("SecurityGroups needs to be changed (%v != %v)", log.Prettify(clb.SecurityGroups), log.Prettify(dlb.SecurityGroups))
		changes |= securityGroupsModified
	}

	if c := l.options.needsModification(); c != 0 {
		changes |= c

		if changes&portsModified != 0 {
			l.logger.Debugf("Ports needs to be changed (%v != %v)", log.Prettify(copts.ports), log.Prettify(dopts.ports))
		}

		if changes&inboundCidrsModified != 0 {
			l.logger.Debugf("InboundCidrs needs to be changed (%v != %v)", log.Prettify(copts.inboundCidrs), log.Prettify(dopts.inboundCidrs))
		}

		if changes&webACLAssociationModified != 0 {
			l.logger.Debugf("WAF needs to be changed: (%v != %v)", log.Prettify(copts.webACLId), log.Prettify(dopts.webACLId))
		}
	}
	return changes, true
}

// StripDesiredState removes the DesiredLoadBalancer from the LoadBalancer
func (l *LoadBalancer) StripDesiredState() {
	l.lb.desired = nil
	l.options.desired.ports = nil
	l.options.desired.managedSG = nil
	l.options.desired.webACLId = nil
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
	if l.lb.current == nil {
		return nil
	}
	return l.lb.current.DNSName
}
