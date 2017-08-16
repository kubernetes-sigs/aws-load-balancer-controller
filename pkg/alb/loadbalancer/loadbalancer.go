package loadbalancer

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	api "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/alb/listeners"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroups"
	"github.com/coreos/alb-ingress-controller/pkg/annotations"
	awsutil "github.com/coreos/alb-ingress-controller/pkg/util/aws"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
)

// LoadBalancer contains the overarching configuration for the ALB
type LoadBalancer struct {
	ID                  *string
	CurrentLoadBalancer *elbv2.LoadBalancer // current version of load balancer in AWS
	DesiredLoadBalancer *elbv2.LoadBalancer // desired version of load balancer in AWS
	TargetGroups        targetgroups.TargetGroups
	Listeners           listeners.Listeners
	CurrentTags         util.Tags
	DesiredTags         util.Tags
	Deleted             bool // flag representing the LoadBalancer instance was fully deleted.
	logger              *log.Logger
}

type loadBalancerChange uint

const (
	securityGroupsModified loadBalancerChange = 1 << iota
	subnetsModified
	tagsModified
	schemeModified
)

// NewLoadBalancer returns a new loadbalancer.LoadBalancer based on the parameters provided.
func NewLoadBalancer(clustername, namespace, ingressname string, logger *log.Logger, annotations *annotations.Annotations, tags util.Tags) *LoadBalancer {
	// TODO: LB name  must contain only alphanumeric characters or hyphens, and must
	// not begin or end with a hyphen.

	hasher := md5.New()
	hasher.Write([]byte(namespace + ingressname))
	hash := hex.EncodeToString(hasher.Sum(nil))[:4]

	name := fmt.Sprintf("%s-%s-%s",
		clustername,
		strings.Replace(namespace, "-", "", -1),
		strings.Replace(ingressname, "-", "", -1),
	)

	if len(name) > 26 {
		name = name[:26]
	}

	name = name + "-" + hash

	lb := &LoadBalancer{
		ID:          aws.String(name),
		DesiredTags: tags,
		DesiredLoadBalancer: &elbv2.LoadBalancer{
			AvailabilityZones: annotations.Subnets.AsAvailabilityZones(),
			LoadBalancerName:  aws.String(name),
			Scheme:            annotations.Scheme,
			SecurityGroups:    annotations.SecurityGroups,
			VpcId:             annotations.VPCID,
		},
		logger: logger,
	}

	return lb
}

// NewLoadBalancerFromAWSLoadBalancer returns a new loadbalancer.LoadBalancer based on an elbv2.LoadBalancer.
func NewLoadBalancerFromAWSLoadBalancer(loadBalancer *elbv2.LoadBalancer, tags util.Tags, clustername string, logger *log.Logger) (*LoadBalancer, error) {
	ingressName, ok := tags.Get("IngressName")
	if !ok {
		return nil, fmt.Errorf("The LoadBalancer %s does not have an IngressName tag, can't import", *loadBalancer.LoadBalancerName)
	}

	namespace, ok := tags.Get("Namespace")
	if !ok {
		return nil, fmt.Errorf("The LoadBalancer %s does not have an Namespace tag, can't import", *loadBalancer.LoadBalancerName)
	}

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

	lb := &LoadBalancer{
		ID:                  aws.String(name),
		CurrentTags:         tags,
		CurrentLoadBalancer: loadBalancer,
		logger:              logger,
	}

	return lb, nil
}

// Reconcile compares the current and desired state of this LoadBalancer instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS ELBV2 (ALB) to
// satisfy the ingress's current state.
func (lb *LoadBalancer) Reconcile(rOpts *ReconcileOptions) []error {
	var errors []error

	switch {
	case lb.DesiredLoadBalancer == nil: // lb should be deleted
		if lb.CurrentLoadBalancer == nil {
			break
		}
		lb.logger.Infof("Start ELBV2 (ALB) deletion.")
		if err := lb.delete(rOpts); err != nil {
			errors = append(errors, err)
			break
		}
		rOpts.Eventf(api.EventTypeNormal, "DELETE", "%s deleted", *lb.CurrentLoadBalancer.LoadBalancerName)
		lb.logger.Infof("Completed ELBV2 (ALB) deletion. Name: %s | ARN: %s",
			*lb.CurrentLoadBalancer.LoadBalancerName,
			*lb.CurrentLoadBalancer.LoadBalancerArn)

	case lb.CurrentLoadBalancer == nil: // lb doesn't exist and should be created
		lb.logger.Infof("Start ELBV2 (ALB) creation.")
		if err := lb.create(rOpts); err != nil {
			errors = append(errors, err)
			return errors
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s created", *lb.CurrentLoadBalancer.LoadBalancerName)
		lb.logger.Infof("Completed ELBV2 (ALB) creation. Name: %s | ARN: %s",
			*lb.CurrentLoadBalancer.LoadBalancerName,
			*lb.CurrentLoadBalancer.LoadBalancerArn)

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

	tgsOpts := targetgroups.NewReconcileOptions()
	tgsOpts.SetEventf(rOpts.Eventf)
	tgsOpts.SetTargetGroups(&lb.TargetGroups)
	tgsOpts.SetVpcID(lb.CurrentLoadBalancer.VpcId)
	if tgs, err := lb.TargetGroups.Reconcile(tgsOpts); err != nil {
		errors = append(errors, err)
	} else {
		lb.TargetGroups = tgs
	}

	listenersOpts := listeners.NewReconcileOptions()
	listenersOpts.SetEventf(rOpts.Eventf)
	listenersOpts.SetListeners(&lb.Listeners)
	listenersOpts.SetLoadBalancerArn(lb.CurrentLoadBalancer.LoadBalancerArn)
	listenersOpts.SetTargetGroups(&lb.TargetGroups)
	if ltnrs, err := lb.Listeners.Reconcile(listenersOpts); err != nil {
		errors = append(errors, err)
	} else {
		lb.Listeners = ltnrs
	}

	return errors
}

// create requests a new ELBV2 (ALB) is created in AWS.

func (lb *LoadBalancer) create(rOpts *ReconcileOptions) error {
	in := &elbv2.CreateLoadBalancerInput{
		Name:           lb.DesiredLoadBalancer.LoadBalancerName,
		Subnets:        util.AvailabilityZones(lb.DesiredLoadBalancer.AvailabilityZones).AsSubnets(),
		Scheme:         lb.DesiredLoadBalancer.Scheme,
		Tags:           lb.DesiredTags,
		SecurityGroups: lb.DesiredLoadBalancer.SecurityGroups,
	}

	o, err := awsutil.ALBsvc.CreateLoadBalancer(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating %s: %s", *in.Name, err.Error())
		lb.logger.Errorf("Failed to create ELBV2 (ALB): %s", err.Error())
		return err
	}

	lb.CurrentLoadBalancer = o.LoadBalancers[0]
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
				LoadBalancerArn: lb.CurrentLoadBalancer.LoadBalancerArn,
				SecurityGroups:  lb.DesiredLoadBalancer.SecurityGroups,
			}
			if _, err := awsutil.ALBsvc.SetSecurityGroups(in); err != nil {
				lb.logger.Errorf("Failed ELBV2 security groups modification: %s", err.Error())
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s security group modification failed: %s", *lb.CurrentLoadBalancer.LoadBalancerName, err.Error())
				return err
			}
			lb.CurrentLoadBalancer.SecurityGroups = lb.DesiredLoadBalancer.SecurityGroups
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s security group modified", *lb.CurrentLoadBalancer.LoadBalancerName)
			lb.logger.Infof("Completed ELBV2 security groups modification. SGs: %s",
				log.Prettify(lb.CurrentLoadBalancer.SecurityGroups))
		}

		// Modify Subnets
		if needsMod&subnetsModified != 0 {
			lb.logger.Infof("Start subnets modification.")
			in := &elbv2.SetSubnetsInput{
				LoadBalancerArn: lb.CurrentLoadBalancer.LoadBalancerArn,
				Subnets:         util.AvailabilityZones(lb.DesiredLoadBalancer.AvailabilityZones).AsSubnets(),
			}
			if _, err := awsutil.ALBsvc.SetSubnets(in); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s subnet modification failed: %s", *lb.CurrentLoadBalancer.LoadBalancerName, err.Error())
				return fmt.Errorf("Failure Setting ALB Subnets: %s", err)
			}
			lb.CurrentLoadBalancer.AvailabilityZones = lb.DesiredLoadBalancer.AvailabilityZones
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s subnets modified", *lb.CurrentLoadBalancer.LoadBalancerName)
			lb.logger.Infof("Completed subnets modification. Subnets are %s.",
				log.Prettify(lb.CurrentLoadBalancer.AvailabilityZones))
		}

		// Modify Tags
		if needsMod&tagsModified != 0 {
			lb.logger.Infof("Start ELBV2 tag modification.")
			if err := awsutil.ALBsvc.UpdateTags(lb.CurrentLoadBalancer.LoadBalancerArn, lb.CurrentTags, lb.DesiredTags); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "%s tag modification failed: %s", *lb.CurrentLoadBalancer.LoadBalancerName, err.Error())
				lb.logger.Errorf("Failed ELBV2 (ALB) tag modification: %s", err.Error())
			}
			lb.CurrentTags = lb.DesiredTags
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s tags modified", *lb.CurrentLoadBalancer.LoadBalancerName)
			lb.logger.Infof("Completed ELBV2 tag modification. Tags are %s.",
				log.Prettify(lb.CurrentTags))
		}

	} else {
		// Modification is needed, but required full replacement of ALB.
		lb.logger.Infof("Start ELBV2 full modification (delete and create).")
		rOpts.Eventf(api.EventTypeNormal, "REBUILD", "Impossible modification requested, rebuilding %s", *lb.CurrentLoadBalancer.LoadBalancerName)
		lb.delete(rOpts)
		// Since listeners and rules are deleted during lb deletion, ensure their current state is removed
		// as they'll no longer exist.
		lb.Listeners.StripCurrentState()
		lb.create(rOpts)
		lb.logger.Infof("Completed ELBV2 full modification (delete and create). Name: %s | ARN: %s",
			*lb.CurrentLoadBalancer.LoadBalancerName, *lb.CurrentLoadBalancer.LoadBalancerArn)

	}

	return nil
}

// delete Deletes the load balancer from AWS.
func (lb *LoadBalancer) delete(rOpts *ReconcileOptions) error {
	in := &elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: lb.CurrentLoadBalancer.LoadBalancerArn,
	}

	if _, err := awsutil.ALBsvc.DeleteLoadBalancer(in); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error deleting %s: %s", *lb.CurrentLoadBalancer.LoadBalancerName, err.Error())
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
	if lb.CurrentLoadBalancer == nil {
		return changes, true
	}

	if !util.DeepEqual(lb.CurrentLoadBalancer.Scheme, lb.DesiredLoadBalancer.Scheme) {
		changes |= schemeModified
		return changes, false
	}

	currentSubnets := util.AvailabilityZones(lb.CurrentLoadBalancer.AvailabilityZones).AsSubnets()
	desiredSubnets := util.AvailabilityZones(lb.DesiredLoadBalancer.AvailabilityZones).AsSubnets()
	sort.Sort(currentSubnets)
	sort.Sort(desiredSubnets)
	if log.Prettify(currentSubnets) != log.Prettify(desiredSubnets) {
		changes |= subnetsModified
	}

	currentSecurityGroups := util.AWSStringSlice(lb.CurrentLoadBalancer.SecurityGroups)
	desiredSecurityGroups := util.AWSStringSlice(lb.DesiredLoadBalancer.SecurityGroups)
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
	l.DesiredLoadBalancer = nil
	if l.Listeners != nil {
		l.Listeners.StripDesiredState()
	}
	if l.TargetGroups != nil {
		l.TargetGroups.StripDesiredState()
	}
	for _, listener := range l.Listeners {
		listener.Rules.StripDesiredState()
	}
}

type ReconcileOptions struct {
	Eventf       func(string, string, string, ...interface{})
	loadbalancer *LoadBalancer
}

func NewReconcileOptions() *ReconcileOptions {
	return &ReconcileOptions{}
}

func (r *ReconcileOptions) SetLoadBalancer(loadbalancer *LoadBalancer) *ReconcileOptions {
	r.loadbalancer = loadbalancer
	return r
}

func (r *ReconcileOptions) SetEventf(f func(string, string, string, ...interface{})) *ReconcileOptions {
	r.Eventf = f
	return r
}
