package tg

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/ec2"
	albelbv2 "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	api "k8s.io/api/core/v1"
)

type NewDesiredTargetGroupOptions struct {
	Annotations    *annotations.Annotations
	Tags           util.Tags
	ALBNamePrefix  string
	LoadBalancerID string
	Port           int64
	Logger         *log.Logger
	SvcName        string
	Targets        util.AWSStringSlice
}

// NewDesiredTargetGroup returns a new targetgroup.TargetGroup based on the parameters provided.
func NewDesiredTargetGroup(o *NewDesiredTargetGroupOptions) *TargetGroup {
	hasher := md5.New()
	hasher.Write([]byte(o.LoadBalancerID))
	output := hex.EncodeToString(hasher.Sum(nil))

	id := fmt.Sprintf("%.12s-%.5d-%.5s-%.7s", o.ALBNamePrefix, o.Port, *o.Annotations.BackendProtocol, output)

	// Add the service name tag to the Target group as it's needed when reassembling ingresses after
	// controller relaunch.
	o.Tags = append(o.Tags, &elbv2.Tag{
		Key: aws.String("ServiceName"), Value: aws.String(o.SvcName)})

	// TODO: Quick fix as we can't have the loadbalancer and target groups share pointers to the same
	// tags. Each modify tags individually and can cause bad side-effects.
	newTagList := []*elbv2.Tag{}
	for _, tag := range o.Tags {
		key := *tag.Key
		value := *tag.Value

		newTag := &elbv2.Tag{
			Key:   &key,
			Value: &value,
		}
		newTagList = append(newTagList, newTag)
	}

	return &TargetGroup{
		ID:      id,
		SvcName: o.SvcName,
		logger:  o.Logger,
		tags:    tags{desired: newTagList},
		targets: targets{desired: o.Targets},
		tg: tg{
			desired: &elbv2.TargetGroup{
				HealthCheckPath:            o.Annotations.HealthcheckPath,
				HealthCheckIntervalSeconds: o.Annotations.HealthcheckIntervalSeconds,
				HealthCheckPort:            o.Annotations.HealthcheckPort,
				HealthCheckProtocol:        o.Annotations.BackendProtocol,
				HealthCheckTimeoutSeconds:  o.Annotations.HealthcheckTimeoutSeconds,
				HealthyThresholdCount:      o.Annotations.HealthyThresholdCount,
				// LoadBalancerArns:
				Matcher:                 &elbv2.Matcher{HttpCode: o.Annotations.SuccessCodes},
				Port:                    aws.Int64(o.Port),
				Protocol:                o.Annotations.BackendProtocol,
				TargetGroupName:         aws.String(id),
				UnhealthyThresholdCount: o.Annotations.UnhealthyThresholdCount,
				// VpcId:
			},
		},
		attributes: attributes{desired: o.Annotations.TargetGroupAttributes},
	}
}

type NewCurrentTargetGroupOptions struct {
	TargetGroup    *elbv2.TargetGroup
	Tags           util.Tags
	ALBNamePrefix  string
	LoadBalancerID string
	Logger         *log.Logger
}

// NewCurrentTargetGroup returns a new targetgroup.TargetGroup from an elbv2.TargetGroup.
func NewCurrentTargetGroup(o *NewCurrentTargetGroupOptions) (*TargetGroup, error) {
	hasher := md5.New()
	hasher.Write([]byte(o.LoadBalancerID))
	output := hex.EncodeToString(hasher.Sum(nil))

	id := fmt.Sprintf("%.12s-%.5d-%.5s-%.7s", o.ALBNamePrefix, *o.TargetGroup.Port, *o.TargetGroup.Protocol, output)

	svcName, ok := o.Tags.Get("ServiceName")
	if !ok {
		return nil, fmt.Errorf("The Target Group %s does not have a Namespace tag, can't import", *o.TargetGroup.TargetGroupArn)
	}

	return &TargetGroup{
		ID:      id,
		SvcName: svcName,
		logger:  o.Logger,
		tags:    tags{current: o.Tags},
		tg:      tg{current: o.TargetGroup},
	}, nil
}

// Reconcile compares the current and desired state of this TargetGroup instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS target group to
// satisfy the ingress's current state.
func (t *TargetGroup) Reconcile(rOpts *ReconcileOptions) error {
	switch {
	// No DesiredState means target group may not be needed.
	// However, target groups aren't deleted until after rules are created
	// Ensuring we know what target groups are truly no longer in use.
	case t.tg.desired == nil:
		t.deleted = true
		return nil

		// No CurrentState means target group doesn't exist in AWS and should be created.
	case t.tg.current == nil:
		t.logger.Infof("Start TargetGroup creation.")
		if err := t.create(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s target group created", t.ID)
		t.logger.Infof("Succeeded TargetGroup creation. ARN: %s | Name: %s.",
			*t.tg.current.TargetGroupArn,
			*t.tg.current.TargetGroupName)
	default:
		// Current and Desired exist and need for modification should be evaluated.
		if mods := t.needsModification(); mods != 0 {
			t.logger.Infof("Start TargetGroup modification.")
			if err := t.modify(mods, rOpts); err != nil {
				return err
			}
			rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s target group modified", t.ID)
			t.logger.Infof("Succeeded TargetGroup modification. ARN: %s | Name: %s.",
				*t.tg.current.TargetGroupArn,
				*t.tg.current.TargetGroupName)
		} else {
			t.logger.Debugf("No TargetGroup modification required.")
		}
	}

	return nil
}

// Creates a new TargetGroup in AWS.
func (t *TargetGroup) create(rOpts *ReconcileOptions) error {
	// Target group in VPC for which ALB will route to
	desired := t.tg.desired
	in := &elbv2.CreateTargetGroupInput{
		HealthCheckPath:            desired.HealthCheckPath,
		HealthCheckIntervalSeconds: desired.HealthCheckIntervalSeconds,
		HealthCheckPort:            desired.HealthCheckPort,
		HealthCheckProtocol:        desired.HealthCheckProtocol,
		HealthCheckTimeoutSeconds:  desired.HealthCheckTimeoutSeconds,
		HealthyThresholdCount:      desired.HealthyThresholdCount,
		Matcher:                    desired.Matcher,
		Port:                       desired.Port,
		Protocol:                   desired.Protocol,
		Name:                       desired.TargetGroupName,
		UnhealthyThresholdCount: desired.UnhealthyThresholdCount,
		VpcId: rOpts.VpcID,
	}

	o, err := albelbv2.ELBV2svc.CreateTargetGroup(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating target group %s: %s", t.ID, err.Error())
		t.logger.Infof("Failed TargetGroup creation: %s.", err.Error())
		return err
	}
	t.tg.current = o.TargetGroups[0]

	// Add tags
	if err = albelbv2.ELBV2svc.UpdateTags(t.CurrentARN(), t.tags.current, t.tags.desired); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error tagging target group %s: %s", t.ID, err.Error())
		t.logger.Infof("Failed TargetGroup creation. Unable to add tags: %s.", err.Error())
		return err
	}
	t.tags.current = t.tags.desired

	// Register Targets
	if err = t.registerTargets(t.targets.desired, rOpts); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error registering targets to target group %s: %s", t.ID, err.Error())
		t.logger.Infof("Failed TargetGroup creation. Unable to register targets:  %s.", err.Error())
		return err
	}

	// Add TargetGroup attributes
	attributes := &elbv2.ModifyTargetGroupAttributesInput{
		Attributes:     t.attributes.desired,
		TargetGroupArn: t.CurrentARN(),
	}

	if _, err := albelbv2.ELBV2svc.ModifyTargetGroupAttributes(attributes); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error adding attributes to target group %s: %s", t.ID, err.Error())
		t.logger.Infof("Failed TargetGroup creation. Unable to add target group attributes: %s.", err.Error())
		return err
	}
	t.attributes.current = t.attributes.desired

	return nil
}

// Modifies the attributes of an existing TargetGroup.
// ALBIngress is only passed along for logging
func (t *TargetGroup) modify(mods tgChange, rOpts *ReconcileOptions) error {
	desired := t.tg.desired
	if mods&paramsModified != 0 {
		in := &elbv2.ModifyTargetGroupInput{
			HealthCheckIntervalSeconds: desired.HealthCheckIntervalSeconds,
			HealthCheckPath:            desired.HealthCheckPath,
			HealthCheckPort:            desired.HealthCheckPort,
			HealthCheckProtocol:        desired.HealthCheckProtocol,
			HealthCheckTimeoutSeconds:  desired.HealthCheckTimeoutSeconds,
			HealthyThresholdCount:      desired.HealthyThresholdCount,
			Matcher:                    desired.Matcher,
			TargetGroupArn:             t.CurrentARN(),
			UnhealthyThresholdCount:    desired.UnhealthyThresholdCount,
		}
		o, err := albelbv2.ELBV2svc.ModifyTargetGroup(in)
		if err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error modifying target group %s: %s", t.ID, err.Error())
			t.logger.Errorf("Failed TargetGroup modification. ARN: %s | Error: %s.",
				*t.CurrentARN(), err.Error())
			return err
		}
		t.tg.current = o.TargetGroups[0]
		// AmazonAPI doesn't return an empty HealthCheckPath.
		t.tg.current.HealthCheckPath = desired.HealthCheckPath
	}

	// check/change tags
	if mods&tagsModified != 0 {
		if err := albelbv2.ELBV2svc.UpdateTags(t.CurrentARN(), t.tags.current, t.tags.desired); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error changing tags on target group %s: %s", t.ID, err.Error())
			t.logger.Errorf("Failed TargetGroup modification. Unable to modify tags. ARN: %s | Error: %s.",
				*t.CurrentARN(), err.Error())
			return err
		}
		t.tags.current = t.tags.desired
	}

	if mods&targetsModified != 0 {
		additions := util.Difference(t.targets.desired, t.targets.current)
		removals := util.Difference(t.targets.current, t.targets.desired)

		// check/change targets
		if len(additions) > 0 {
			if err := t.registerTargets(additions, rOpts); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error adding targets to target group %s: %s", t.ID, err.Error())
				t.logger.Infof("Failed TargetGroup modification. Unable to add targets: %s.", err.Error())
				return err
			}
		}
		if len(removals) > 0 {
			if err := t.deregisterTargets(removals, rOpts); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error removing targets from target group %s: %s", t.ID, err.Error())
				t.logger.Infof("Failed TargetGroup modification. Unable to remove targets: %s.", err.Error())
				return err
			}
		}
		t.targets.current = t.targets.desired
	}

	if mods&attributesModified != 0 {
		aOpts := &elbv2.ModifyTargetGroupAttributesInput{
			Attributes:     t.attributes.desired,
			TargetGroupArn: t.CurrentARN(),
		}
		if _, err := albelbv2.ELBV2svc.ModifyTargetGroupAttributes(aOpts); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error modifying attributes in target group %s: %s", t.ID, err.Error())
			t.logger.Infof("Failed TargetGroup modification. Unable to change attributes: %s.", err.Error())
			return err
		}
		t.attributes.current = t.attributes.desired
	}

	return nil
}

func (t *TargetGroup) needsModification() tgChange {
	var changes tgChange

	ctg := t.tg.current
	dtg := t.tg.desired

	// No target group set currently exists; modification required.
	if ctg == nil {
		t.logger.Debugf("Current Target Group is undefined")
		return changes
	}

	if !util.DeepEqual(ctg.HealthCheckIntervalSeconds, dtg.HealthCheckIntervalSeconds) {
		t.logger.Debugf("HealthCheckIntervalSeconds needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckIntervalSeconds), log.Prettify(dtg.HealthCheckIntervalSeconds))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.HealthCheckPath, dtg.HealthCheckPath) {
		t.logger.Debugf("HealthCheckPath needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckPath), log.Prettify(dtg.HealthCheckPath))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.HealthCheckPort, dtg.HealthCheckPort) {
		t.logger.Debugf("HealthCheckPort needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckPort), log.Prettify(dtg.HealthCheckPort))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.HealthCheckProtocol, dtg.HealthCheckProtocol) {
		t.logger.Debugf("HealthCheckProtocol needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckProtocol), log.Prettify(dtg.HealthCheckProtocol))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.HealthCheckTimeoutSeconds, dtg.HealthCheckTimeoutSeconds) {
		t.logger.Debugf("HealthCheckTimeoutSeconds needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckTimeoutSeconds), log.Prettify(dtg.HealthCheckTimeoutSeconds))
		changes |= paramsModified
	}
	if !util.DeepEqual(ctg.HealthyThresholdCount, dtg.HealthyThresholdCount) {
		t.logger.Debugf("HealthyThresholdCount needs to be changed (%v != %v)", log.Prettify(ctg.HealthyThresholdCount), log.Prettify(dtg.HealthyThresholdCount))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.Matcher, dtg.Matcher) {
		t.logger.Debugf("Matcher needs to be changed (%v != %v)", log.Prettify(ctg.Matcher), log.Prettify(ctg.Matcher))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.UnhealthyThresholdCount, dtg.UnhealthyThresholdCount) {
		t.logger.Debugf("UnhealthyThresholdCount needs to be changed (%v != %v)", log.Prettify(ctg.UnhealthyThresholdCount), log.Prettify(dtg.UnhealthyThresholdCount))
		changes |= paramsModified
	}

	if t.targets.current.Hash() != t.targets.desired.Hash() {
		t.logger.Debugf("Targets need to be changed.")
		changes |= targetsModified
	}

	if t.tags.current.Hash() != t.tags.desired.Hash() {
		t.logger.Debugf("Tags need to be changed")
		changes |= tagsModified
	}

	if !reflect.DeepEqual(t.attributes.current.Sorted(), t.attributes.desired.Sorted()) {
		t.logger.Debugf("Attributes need to be changed")
		changes |= attributesModified
	}

	return changes
	// These fields require a rebuild and are enforced via TG name hash
	//	Port *int64 `min:"1" type:"integer"`
	//	Protocol *string `type:"string" enum:"ProtocolEnum"`
}

// Registers Targets (ec2 instances) to TargetGroup, must be called when Current != Desired
func (t *TargetGroup) registerTargets(additions util.AWSStringSlice, rOpts *ReconcileOptions) error {
	targets := []*elbv2.TargetDescription{}
	for _, target := range additions {
		targets = append(targets, &elbv2.TargetDescription{
			Id:   target,
			Port: t.tg.current.Port,
		})
	}

	in := &elbv2.RegisterTargetsInput{
		TargetGroupArn: t.CurrentARN(),
		Targets:        targets,
	}

	if _, err := albelbv2.ELBV2svc.RegisterTargets(in); err != nil {
		return err
	}

	t.targets.current = t.targets.desired

	// when managing security groups, ensure sg is associated with instance
	if rOpts.ManagedSGInstance != nil {
		err := ec2.EC2svc.AssociateSGToInstanceIfNeeded(additions, rOpts.ManagedSGInstance)
		if err != nil {
			return err
		}
	}

	return nil
}

// Deregisters Targets (ec2 instances) from the TargetGroup, must be called when Current != Desired
func (t *TargetGroup) deregisterTargets(removals util.AWSStringSlice, rOpts *ReconcileOptions) error {
	targets := []*elbv2.TargetDescription{}
	for _, target := range removals {
		targets = append(targets, &elbv2.TargetDescription{
			Id:   target,
			Port: t.tg.current.Port,
		})
	}

	in := &elbv2.DeregisterTargetsInput{
		TargetGroupArn: t.CurrentARN(),
		Targets:        targets,
	}

	if _, err := albelbv2.ELBV2svc.DeregisterTargets(in); err != nil {
		return err
	}

	t.targets.current = t.targets.desired

	// when managing security groups, ensure sg is disassociated with instance
	if rOpts.ManagedSGInstance != nil {
		err := ec2.EC2svc.DisassociateSGFromInstanceIfNeeded(removals, rOpts.ManagedSGInstance)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *TargetGroup) CurrentARN() *string {
	if t.tg.current == nil {
		return nil
	}
	return t.tg.current.TargetGroupArn
}

func (t *TargetGroup) CurrentTargets() util.AWSStringSlice {
	return t.targets.current
}
