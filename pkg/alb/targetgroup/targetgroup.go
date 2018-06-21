package targetgroup

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/annotations"
	"github.com/coreos/alb-ingress-controller/pkg/aws/ec2"
	albelbv2 "github.com/coreos/alb-ingress-controller/pkg/aws/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
	api "k8s.io/api/core/v1"
)

type Targets struct {
	Current util.AWSStringSlice
	Desired util.AWSStringSlice
}

type Tags struct {
	Current util.Tags
	Desired util.Tags
}

// TargetGroup contains the current/desired tags & targetgroup for the ALB
type TargetGroup struct {
	ID                string
	SvcName           string
	Tags              Tags
	CurrentAttributes albelbv2.TargetGroupAttributes
	DesiredAttributes albelbv2.TargetGroupAttributes
	Current           *elbv2.TargetGroup
	Desired           *elbv2.TargetGroup
	Targets           Targets
	Deleted           bool
	logger            *log.Logger
}

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

type tgChange uint

const (
	paramsModified tgChange = 1 << iota
	targetsModified
	tagsModified
	attributesModified
)

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
		Tags: Tags{
			Desired: newTagList,
		},
		Targets: Targets{
			Desired: o.Targets,
		},
		Desired: &elbv2.TargetGroup{
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
		DesiredAttributes: o.Annotations.TargetGroupAttributes,
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
		Tags: Tags{
			Current: o.Tags,
		},
		Current: o.TargetGroup,
	}, nil
}

// Reconcile compares the current and desired state of this TargetGroup instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS target group to
// satisfy the ingress's current state.
func (tg *TargetGroup) Reconcile(rOpts *ReconcileOptions) error {
	switch {
	// No DesiredState means target group may not be needed.
	// However, target groups aren't deleted until after rules are created
	// Ensuring we know what target groups are truly no longer in use.
	case tg.Desired == nil:
		tg.Deleted = true
		return nil

		// No CurrentState means target group doesn't exist in AWS and should be created.
	case tg.Current == nil:
		tg.logger.Infof("Start TargetGroup creation.")
		if err := tg.create(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s target group created", tg.ID)
		tg.logger.Infof("Succeeded TargetGroup creation. ARN: %s | Name: %s.",
			*tg.Current.TargetGroupArn,
			*tg.Current.TargetGroupName)
	default:
		// Current and Desired exist and need for modification should be evaluated.
		if m := tg.needsModification(); m != 0 {
			tg.logger.Infof("Start TargetGroup modification.")
			if err := tg.modify(m, rOpts); err != nil {
				return err
			}
			rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s target group modified", tg.ID)
			tg.logger.Infof("Succeeded TargetGroup modification. ARN: %s | Name: %s.",
				*tg.Current.TargetGroupArn,
				*tg.Current.TargetGroupName)
		} else {
			tg.logger.Debugf("No TargetGroup modification required.")
		}
	}

	return nil
}

// Creates a new TargetGroup in AWS.
func (tg *TargetGroup) create(rOpts *ReconcileOptions) error {
	// Target group in VPC for which ALB will route to
	in := &elbv2.CreateTargetGroupInput{
		HealthCheckPath:            tg.Desired.HealthCheckPath,
		HealthCheckIntervalSeconds: tg.Desired.HealthCheckIntervalSeconds,
		HealthCheckPort:            tg.Desired.HealthCheckPort,
		HealthCheckProtocol:        tg.Desired.HealthCheckProtocol,
		HealthCheckTimeoutSeconds:  tg.Desired.HealthCheckTimeoutSeconds,
		HealthyThresholdCount:      tg.Desired.HealthyThresholdCount,
		Matcher:                    tg.Desired.Matcher,
		Port:                       tg.Desired.Port,
		Protocol:                   tg.Desired.Protocol,
		Name:                       tg.Desired.TargetGroupName,
		UnhealthyThresholdCount: tg.Desired.UnhealthyThresholdCount,
		VpcId: rOpts.VpcID,
	}

	o, err := albelbv2.ELBV2svc.CreateTargetGroup(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating target group %s: %s", tg.ID, err.Error())
		tg.logger.Infof("Failed TargetGroup creation: %s.", err.Error())
		return err
	}
	tg.Current = o.TargetGroups[0]

	// Add tags
	if err = albelbv2.ELBV2svc.UpdateTags(tg.Current.TargetGroupArn, tg.Tags.Current, tg.Tags.Desired); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error tagging target group %s: %s", tg.ID, err.Error())
		tg.logger.Infof("Failed TargetGroup creation. Unable to add tags: %s.", err.Error())
		return err
	}
	tg.Tags.Current = tg.Tags.Desired

	// Register Targets
	if err = tg.registerTargets(tg.Targets.Desired, rOpts); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error registering targets to target group %s: %s", tg.ID, err.Error())
		tg.logger.Infof("Failed TargetGroup creation. Unable to register targets:  %s.", err.Error())
		return err
	}

	// Add TargetGroup attributes
	attributes := &elbv2.ModifyTargetGroupAttributesInput{
		Attributes:     tg.DesiredAttributes,
		TargetGroupArn: tg.Current.TargetGroupArn,
	}

	if _, err := albelbv2.ELBV2svc.ModifyTargetGroupAttributes(attributes); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error adding attributes to target group %s: %s", tg.ID, err.Error())
		tg.logger.Infof("Failed TargetGroup creation. Unable to add target group attributes: %s.", err.Error())
		return err
	}
	tg.CurrentAttributes = tg.DesiredAttributes

	return nil
}

// Modifies the attributes of an existing TargetGroup.
// ALBIngress is only passed along for logging
func (tg *TargetGroup) modify(m tgChange, rOpts *ReconcileOptions) error {
	if m&paramsModified != 0 {
		in := &elbv2.ModifyTargetGroupInput{
			HealthCheckIntervalSeconds: tg.Desired.HealthCheckIntervalSeconds,
			HealthCheckPath:            tg.Desired.HealthCheckPath,
			HealthCheckPort:            tg.Desired.HealthCheckPort,
			HealthCheckProtocol:        tg.Desired.HealthCheckProtocol,
			HealthCheckTimeoutSeconds:  tg.Desired.HealthCheckTimeoutSeconds,
			HealthyThresholdCount:      tg.Desired.HealthyThresholdCount,
			Matcher:                    tg.Desired.Matcher,
			TargetGroupArn:             tg.Current.TargetGroupArn,
			UnhealthyThresholdCount:    tg.Desired.UnhealthyThresholdCount,
		}
		o, err := albelbv2.ELBV2svc.ModifyTargetGroup(in)
		if err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error modifying target group %s: %s", tg.ID, err.Error())
			tg.logger.Errorf("Failed TargetGroup modification. ARN: %s | Error: %s.",
				*tg.Current.TargetGroupArn, err.Error())
			return err
		}
		tg.Current = o.TargetGroups[0]
		// AmazonAPI doesn't return an empty HealthCheckPath.
		tg.Current.HealthCheckPath = tg.Desired.HealthCheckPath
	}

	// check/change tags
	if m&tagsModified != 0 {
		if err := albelbv2.ELBV2svc.UpdateTags(tg.Current.TargetGroupArn, tg.Tags.Current, tg.Tags.Desired); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error changing tags on target group %s: %s", tg.ID, err.Error())
			tg.logger.Errorf("Failed TargetGroup modification. Unable to modify tags. ARN: %s | Error: %s.",
				*tg.Current.TargetGroupArn, err.Error())
			return err
		}
		tg.Tags.Current = tg.Tags.Desired
	}

	if m&targetsModified != 0 {
		additions := util.Difference(tg.Targets.Desired, tg.Targets.Current)
		removals := util.Difference(tg.Targets.Current, tg.Targets.Desired)

		// check/change targets
		if len(additions) > 0 {
			if err := tg.registerTargets(additions, rOpts); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error adding targets to target group %s: %s", tg.ID, err.Error())
				tg.logger.Infof("Failed TargetGroup modification. Unable to add targets: %s.", err.Error())
				return err
			}
		}
		if len(removals) > 0 {
			if err := tg.deregisterTargets(removals, rOpts); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error removing targets from target group %s: %s", tg.ID, err.Error())
				tg.logger.Infof("Failed TargetGroup modification. Unable to remove targets: %s.", err.Error())
				return err
			}
		}
		tg.Targets.Current = tg.Targets.Desired
	}

	if m&attributesModified != 0 {
		attributes := &elbv2.ModifyTargetGroupAttributesInput{
			Attributes:     tg.DesiredAttributes,
			TargetGroupArn: tg.Current.TargetGroupArn,
		}
		if _, err := albelbv2.ELBV2svc.ModifyTargetGroupAttributes(attributes); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error modifying attributes in target group %s: %s", tg.ID, err.Error())
			tg.logger.Infof("Failed TargetGroup modification. Unable to change attributes: %s.", err.Error())
			return err
		}
		tg.CurrentAttributes = tg.DesiredAttributes
	}

	return nil
}

// DeleteTG deletes a TargetGroup from AWS.
func DeleteTG(tg *TargetGroup) error {
	in := elbv2.DeleteTargetGroupInput{TargetGroupArn: tg.Current.TargetGroupArn}
	if err := albelbv2.ELBV2svc.RemoveTargetGroup(in); err != nil {
		return err
	}
	return nil
}

func (tg *TargetGroup) needsModification() tgChange {
	var changes tgChange

	ctg := tg.Current
	dtg := tg.Desired

	// No target group set currently exists; modification required.
	if ctg == nil {
		tg.logger.Debugf("Current Target Group is undefined")
		return changes
	}

	if !util.DeepEqual(ctg.HealthCheckIntervalSeconds, dtg.HealthCheckIntervalSeconds) {
		tg.logger.Debugf("HealthCheckIntervalSeconds needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckIntervalSeconds), log.Prettify(dtg.HealthCheckIntervalSeconds))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.HealthCheckPath, dtg.HealthCheckPath) {
		tg.logger.Debugf("HealthCheckPath needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckPath), log.Prettify(dtg.HealthCheckPath))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.HealthCheckPort, dtg.HealthCheckPort) {
		tg.logger.Debugf("HealthCheckPort needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckPort), log.Prettify(dtg.HealthCheckPort))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.HealthCheckProtocol, dtg.HealthCheckProtocol) {
		tg.logger.Debugf("HealthCheckProtocol needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckProtocol), log.Prettify(dtg.HealthCheckProtocol))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.HealthCheckTimeoutSeconds, dtg.HealthCheckTimeoutSeconds) {
		tg.logger.Debugf("HealthCheckTimeoutSeconds needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckTimeoutSeconds), log.Prettify(dtg.HealthCheckTimeoutSeconds))
		changes |= paramsModified
	}
	if !util.DeepEqual(ctg.HealthyThresholdCount, dtg.HealthyThresholdCount) {
		tg.logger.Debugf("HealthyThresholdCount needs to be changed (%v != %v)", log.Prettify(ctg.HealthyThresholdCount), log.Prettify(dtg.HealthyThresholdCount))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.Matcher, dtg.Matcher) {
		tg.logger.Debugf("Matcher needs to be changed (%v != %v)", log.Prettify(ctg.Matcher), log.Prettify(ctg.Matcher))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.UnhealthyThresholdCount, dtg.UnhealthyThresholdCount) {
		tg.logger.Debugf("UnhealthyThresholdCount needs to be changed (%v != %v)", log.Prettify(ctg.UnhealthyThresholdCount), log.Prettify(dtg.UnhealthyThresholdCount))
		changes |= paramsModified
	}

	if *tg.Targets.Current.Hash() != *tg.Targets.Desired.Hash() {
		tg.logger.Debugf("Targets need to be changed.")
		changes |= targetsModified
	}

	if *tg.Tags.Current.Hash() != *tg.Tags.Desired.Hash() {
		tg.logger.Debugf("Tags need to be changed")
		changes |= tagsModified
	}

	if !reflect.DeepEqual(tg.CurrentAttributes.Sorted(), tg.DesiredAttributes.Sorted()) {
		tg.logger.Debugf("Attributes need to be changed")
		changes |= attributesModified
	}

	return changes
	// These fields require a rebuild and are enforced via TG name hash
	//	Port *int64 `min:"1" type:"integer"`
	//	Protocol *string `type:"string" enum:"ProtocolEnum"`
}

// Registers Targets (ec2 instances) to TargetGroup, must be called when Current != Desired
func (tg *TargetGroup) registerTargets(additions util.AWSStringSlice, rOpts *ReconcileOptions) error {
	targets := []*elbv2.TargetDescription{}
	for _, target := range additions {
		targets = append(targets, &elbv2.TargetDescription{
			Id:   target,
			Port: tg.Current.Port,
		})
	}

	in := &elbv2.RegisterTargetsInput{
		TargetGroupArn: tg.Current.TargetGroupArn,
		Targets:        targets,
	}

	if _, err := albelbv2.ELBV2svc.RegisterTargets(in); err != nil {
		return err
	}

	tg.Targets.Current = tg.Targets.Desired

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
func (tg *TargetGroup) deregisterTargets(removals util.AWSStringSlice, rOpts *ReconcileOptions) error {
	targets := []*elbv2.TargetDescription{}
	for _, target := range removals {
		targets = append(targets, &elbv2.TargetDescription{
			Id:   target,
			Port: tg.Current.Port,
		})
	}

	in := &elbv2.DeregisterTargetsInput{
		TargetGroupArn: tg.Current.TargetGroupArn,
		Targets:        targets,
	}

	if _, err := albelbv2.ELBV2svc.DeregisterTargets(in); err != nil {
		return err
	}

	tg.Targets.Current = tg.Targets.Desired

	// when managing security groups, ensure sg is disassociated with instance
	if rOpts.ManagedSGInstance != nil {
		err := ec2.EC2svc.DisassociateSGFromInstanceIfNeeded(removals, rOpts.ManagedSGInstance)
		if err != nil {
			return err
		}
	}

	return nil
}

type ReconcileOptions struct {
	Eventf            func(string, string, string, ...interface{})
	VpcID             *string
	ManagedSGInstance *string
}
