package tg

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"reflect"

	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/backend"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

// The port used when creating targetGroup serves as a default value for targets registered without port specified.
// there are cases that a single targetGroup contains different ports, e.g. backend service targets multiple deployments with targetPort
// as "http", but "http" points to 80 or 8080 in different deployment.
// So we justed used a dummy(but valid) port number when creating targetGroup, and register targets with port number explicitly.
// see https://docs.aws.amazon.com/sdk-for-go/api/service/elbv2/#CreateTargetGroupInput
const targetGroupDefaultPort = 1

type NewDesiredTargetGroupOptions struct {
	Annotations    *annotations.Service
	Ingress        *extensions.Ingress
	CommonTags     util.ELBv2Tags
	Store          store.Storer
	LoadBalancerID string
	Logger         *log.Logger
	SvcName        string
	SvcPort        intstr.IntOrString
	Targets        albelbv2.TargetDescriptions
}

// NewDesiredTargetGroup returns a new targetgroup.TargetGroup based on the parameters provided.
func NewDesiredTargetGroup(o *NewDesiredTargetGroupOptions) *TargetGroup {
	id := o.generateID()

	tgTags := o.CommonTags.Copy()
	tgTags = append(tgTags, &elbv2.Tag{
		Key: aws.String("kubernetes.io/service-name"), Value: aws.String(o.SvcName),
	})
	tgTags = append(tgTags, &elbv2.Tag{
		Key: aws.String("kubernetes.io/service-port"), Value: aws.String(o.SvcPort.String()),
	})

	return &TargetGroup{
		ID:         id,
		SvcName:    o.SvcName,
		SvcPort:    o.SvcPort,
		TargetType: *o.Annotations.TargetGroup.TargetType,
		logger:     o.Logger,
		tags:       tags{desired: tgTags},
		targets:    targets{desired: o.Targets},
		tg: tg{
			desired: &elbv2.TargetGroup{
				HealthCheckPath:            o.Annotations.HealthCheck.Path,
				HealthCheckIntervalSeconds: o.Annotations.HealthCheck.IntervalSeconds,
				HealthCheckPort:            o.Annotations.HealthCheck.Port,
				HealthCheckProtocol:        o.Annotations.HealthCheck.Protocol,
				HealthCheckTimeoutSeconds:  o.Annotations.HealthCheck.TimeoutSeconds,
				HealthyThresholdCount:      o.Annotations.TargetGroup.HealthyThresholdCount,
				// LoadBalancerArns:
				Matcher:                 &elbv2.Matcher{HttpCode: o.Annotations.TargetGroup.SuccessCodes},
				Port:                    aws.Int64(targetGroupDefaultPort),
				Protocol:                o.Annotations.TargetGroup.BackendProtocol,
				TargetGroupName:         aws.String(id),
				TargetType:              o.Annotations.TargetGroup.TargetType,
				UnhealthyThresholdCount: o.Annotations.TargetGroup.UnhealthyThresholdCount,
				// VpcId:
			},
		},
		attributes: attributes{desired: o.Annotations.TargetGroup.Attributes},
	}
}

func (o *NewDesiredTargetGroupOptions) generateID() string {
	hasher := md5.New()
	hasher.Write([]byte(o.LoadBalancerID))
	hasher.Write([]byte(o.SvcName))
	hasher.Write([]byte(o.SvcPort.String()))
	hasher.Write([]byte(aws.StringValue(o.Annotations.TargetGroup.BackendProtocol)))
	hasher.Write([]byte(aws.StringValue(o.Annotations.TargetGroup.TargetType)))

	return fmt.Sprintf("%.12s-%.19s", o.Store.GetConfig().ALBNamePrefix, hex.EncodeToString(hasher.Sum(nil)))
}

type NewDesiredTargetGroupFromBackendOptions struct {
	Backend              *extensions.IngressBackend
	CommonTags           util.ELBv2Tags
	LoadBalancerID       string
	Store                store.Storer
	Ingress              *extensions.Ingress
	Logger               *log.Logger
	ExistingTargetGroups TargetGroups
}

func NewDesiredTargetGroupFromBackend(o *NewDesiredTargetGroupFromBackendOptions) (*TargetGroup, error) {
	serviceKey := fmt.Sprintf("%s/%s", o.Ingress.Namespace, o.Backend.ServiceName)

	annos, err := o.Store.GetIngressAnnotations(k8s.MetaNamespaceKey(o.Ingress))
	if err != nil {
		return nil, err
	}

	tgAnnotations, err := o.Store.GetServiceAnnotations(serviceKey, annos)
	if err != nil {
		return nil, fmt.Errorf(fmt.Sprintf("Error getting Service annotations, %v", err.Error()))
	}

	endpointResolver := backend.NewEndpointResolver(o.Store, *tgAnnotations.TargetGroup.TargetType)
	targets, err := endpointResolver.Resolve(o.Ingress, o.Backend)
	if err != nil {
		return nil, err
	}

	if *tgAnnotations.TargetGroup.TargetType == elbv2.TargetTypeEnumIp {
		err := targets.PopulateAZ()
		if err != nil {
			return nil, err
		}
	}
	// Start with a new target group with a new Desired state.
	targetGroup := NewDesiredTargetGroup(&NewDesiredTargetGroupOptions{
		Ingress:        o.Ingress,
		Annotations:    tgAnnotations,
		CommonTags:     o.CommonTags,
		Store:          o.Store,
		LoadBalancerID: o.LoadBalancerID,
		Logger:         o.Logger,
		SvcName:        o.Backend.ServiceName,
		SvcPort:        o.Backend.ServicePort,
		Targets:        targets,
	})

	// If this target group is already defined, copy the current state to our new TG
	if i, _ := o.ExistingTargetGroups.FindById(targetGroup.ID); i >= 0 {
		o.ExistingTargetGroups[i].copyDesiredState(targetGroup)
		return o.ExistingTargetGroups[i], nil
	}

	return targetGroup, nil
}

type NewCurrentTargetGroupOptions struct {
	TargetGroup    *elbv2.TargetGroup
	LoadBalancerID string
	Logger         *log.Logger
}

// NewCurrentTargetGroup returns a new targetgroup.TargetGroup from an elbv2.TargetGroup.
func NewCurrentTargetGroup(o *NewCurrentTargetGroupOptions) (*TargetGroup, error) {
	resourceTags, err := albrgt.RGTsvc.GetClusterResources()
	if err != nil {
		return nil, fmt.Errorf("Failed to get AWS tags. Error: %s", err.Error())
	}

	tgTags := resourceTags.TargetGroups[*o.TargetGroup.TargetGroupArn]

	svcName, svcPort, err := tgTags.ServiceNameAndPort()
	if err != nil {
		return nil, fmt.Errorf("The Target Group %s does not have the proper tags, can't import: %s", *o.TargetGroup.TargetGroupArn, err.Error())
	}

	attrs, err := albelbv2.ELBV2svc.DescribeTargetGroupAttributesFiltered(o.TargetGroup.TargetGroupArn)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve attributes for Target Group. Error: %s", err.Error())
	}

	o.Logger.Infof("Fetching Targets for Target Group %s", *o.TargetGroup.TargetGroupArn)

	currentTargets, err := albelbv2.ELBV2svc.DescribeTargetGroupTargetsForArn(o.TargetGroup.TargetGroupArn)
	if err != nil {
		return nil, err
	}

	return &TargetGroup{
		ID:         *o.TargetGroup.TargetGroupName,
		SvcName:    svcName,
		SvcPort:    svcPort,
		logger:     o.Logger,
		targets:    targets{current: currentTargets},
		attributes: attributes{current: attrs},
		tags:       tags{current: tgTags},
		tg:         tg{current: o.TargetGroup},
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
	case t.tg.desired == nil && !rOpts.IgnoreDeletes:
		t.logger.Infof("Start TargetGroup deletion. ARN: %s | Name: %s.",
			*t.tg.current.TargetGroupArn,
			*t.tg.current.TargetGroupName)
		if err := t.delete(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "DELETE", "%v target group deleted", t.ID)
		t.logger.Infof("Completed TargetGroup deletion. ARN: %s | Name: %s.",
			*t.tg.current.TargetGroupArn,
			*t.tg.current.TargetGroupName)

	case t.tg.desired == nil && rOpts.IgnoreDeletes:
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
			if err := t.modify(mods, rOpts); err != nil {
				return err
			}
			rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s target group modified", t.ID)
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
		TargetType:                 desired.TargetType,
		UnhealthyThresholdCount:    desired.UnhealthyThresholdCount,
		VpcId:                      rOpts.VpcID,
	}

	o, err := albelbv2.ELBV2svc.CreateTargetGroup(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating target group %s: %s", t.ID, err.Error())
		return fmt.Errorf("Failed TargetGroup creation: %s.", err.Error())
	}
	t.tg.current = o.TargetGroups[0]

	// Add tags
	if err = albelbv2.ELBV2svc.UpdateTags(t.CurrentARN(), t.tags.current, t.tags.desired); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error tagging target group %s: %s", t.ID, err.Error())
		return fmt.Errorf("Failed TargetGroup creation. Unable to add tags: %s.", err.Error())
	}
	t.tags.current = t.tags.desired

	// Register Targets
	if err = t.registerTargets(t.targets.desired, rOpts); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error registering targets to target group %s: %s", t.ID, err.Error())
		return fmt.Errorf("Failed TargetGroup creation. Unable to register targets:  %s.", err.Error())
	}
	t.targets.current = t.targets.desired

	if len(t.attributes.desired) > 0 {
		// Add TargetGroup attributes
		attributes := &elbv2.ModifyTargetGroupAttributesInput{
			Attributes:     t.attributes.desired,
			TargetGroupArn: t.CurrentARN(),
		}

		if _, err := albelbv2.ELBV2svc.ModifyTargetGroupAttributes(attributes); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error adding attributes to target group %s: %s", t.ID, err.Error())
			return fmt.Errorf("Failed TargetGroup creation. Unable to add target group attributes: %s.", err.Error())
		}
		t.attributes.current = t.attributes.desired
	}

	return nil
}

// Modifies the attributes of an existing TargetGroup.
// ALBIngress is only passed along for logging
func (t *TargetGroup) modify(mods tgChange, rOpts *ReconcileOptions) error {
	desired := t.tg.desired
	if mods&paramsModified != 0 {
		t.logger.Infof("Modifying target group parameters.")
		o, err := albelbv2.ELBV2svc.ModifyTargetGroup(&elbv2.ModifyTargetGroupInput{
			HealthCheckIntervalSeconds: desired.HealthCheckIntervalSeconds,
			HealthCheckPath:            desired.HealthCheckPath,
			HealthCheckPort:            desired.HealthCheckPort,
			HealthCheckProtocol:        desired.HealthCheckProtocol,
			HealthCheckTimeoutSeconds:  desired.HealthCheckTimeoutSeconds,
			HealthyThresholdCount:      desired.HealthyThresholdCount,
			Matcher:                    desired.Matcher,
			TargetGroupArn:             t.CurrentARN(),
			UnhealthyThresholdCount:    desired.UnhealthyThresholdCount,
		})
		if err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error modifying target group %s: %s", t.ID, err.Error())
			return fmt.Errorf("Failed TargetGroup modification. ARN: %s | Error: %s",
				*t.CurrentARN(), err.Error())
		}
		t.tg.current = o.TargetGroups[0]
		// AmazonAPI doesn't return an empty HealthCheckPath.
		t.tg.current.HealthCheckPath = desired.HealthCheckPath
	}

	// check/change tags
	if mods&tagsModified != 0 {
		t.logger.Infof("Modifying target group tags.")
		if err := albelbv2.ELBV2svc.UpdateTags(t.CurrentARN(), t.tags.current, t.tags.desired); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error changing tags on target group %s: %s", t.ID, err.Error())
			return fmt.Errorf("Failed TargetGroup modification. Unable to modify tags. ARN: %s | Error: %s",
				*t.CurrentARN(), err.Error())
		}
		t.tags.current = t.tags.desired
	}

	if mods&targetsModified != 0 {
		additions := t.targets.desired.Difference(t.targets.current)
		removals := t.targets.current.Difference(t.targets.desired)

		t.logger.Infof("Modifying target group targets. Adding (%v) and removing (%v)", additions.String(), removals.String())

		// check/change targets
		if len(additions) > 0 {
			if err := t.registerTargets(additions, rOpts); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error adding targets to target group %s: %s", t.ID, err.Error())
				return fmt.Errorf("Failed TargetGroup modification. Unable to add targets: %s", err.Error())
			}
		}
		if len(removals) > 0 {
			if err := t.deregisterTargets(removals, rOpts); err != nil {
				rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error removing targets from target group %s: %s", t.ID, err.Error())
				return fmt.Errorf("Failed TargetGroup modification. Unable to remove targets: %s", err.Error())
			}
		}
		t.targets.current = t.targets.desired
	}

	if mods&attributesModified != 0 {
		t.logger.Infof("Modifying target group attributes.")
		aOpts := &elbv2.ModifyTargetGroupAttributesInput{
			Attributes:     t.attributes.desired,
			TargetGroupArn: t.CurrentARN(),
		}
		if _, err := albelbv2.ELBV2svc.ModifyTargetGroupAttributes(aOpts); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error modifying attributes in target group %s: %s", t.ID, err.Error())
			return fmt.Errorf("Failed TargetGroup modification. Unable to change attributes: %s", err.Error())
		}
		t.attributes.current = t.attributes.desired
	}

	return nil
}

// delete a TargetGroup.
func (t *TargetGroup) delete(rOpts *ReconcileOptions) error {
	if err := albelbv2.ELBV2svc.RemoveTargetGroup(t.CurrentARN()); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error deleting %v target group: %s", t.ID, err.Error())
		return err
	}
	t.deleted = true
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

	if dtg == nil {
		// t.logger.Debugf("Desired Target Group is undefined")
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

	if !reflect.DeepEqual(t.attributes.current.Filtered().Sorted(), t.attributes.desired.Filtered().Sorted()) {
		t.logger.Debugf("Attributes need to be changed")
		changes |= attributesModified
	}

	return changes
}

// Registers Targets (ec2 instances) to TargetGroup, must be called when Current != Desired
func (t *TargetGroup) registerTargets(additions albelbv2.TargetDescriptions, rOpts *ReconcileOptions) error {
	if len(additions) == 0 {
		return nil
	}

	in := &elbv2.RegisterTargetsInput{
		TargetGroupArn: t.CurrentARN(),
		Targets:        additions,
	}

	if _, err := albelbv2.ELBV2svc.RegisterTargets(in); err != nil {
		// Flush the cached health of the TG so that on the next iteration it will get fresh data, these change often
		return err
	}

	return nil
}

// Deregisters Targets (ec2 instances) from the TargetGroup, must be called when Current != Desired
func (t *TargetGroup) deregisterTargets(removals albelbv2.TargetDescriptions, rOpts *ReconcileOptions) error {
	if len(removals) == 0 {
		return nil
	}
	in := &elbv2.DeregisterTargetsInput{
		TargetGroupArn: t.CurrentARN(),
		Targets:        removals,
	}

	if _, err := albelbv2.ELBV2svc.DeregisterTargets(in); err != nil {
		return err
	}

	return nil
}

func (t *TargetGroup) CurrentARN() *string {
	if t.tg.current == nil {
		return nil
	}
	return t.tg.current.TargetGroupArn
}

func (t *TargetGroup) DesiredTargets() albelbv2.TargetDescriptions {
	return t.targets.desired
}

func (t *TargetGroup) CurrentTargets() albelbv2.TargetDescriptions {
	return t.targets.current
}

func (t *TargetGroup) StripDesiredState() {
	t.tags.desired = nil
	t.tg.desired = nil
	t.targets.desired = nil
	t.attributes.desired = nil
}

func (t *TargetGroup) copyDesiredState(s *TargetGroup) {
	t.tags.desired = s.tags.desired
	t.attributes.desired = s.attributes.desired
	t.targets.desired = s.targets.desired
	t.tg.desired = s.tg.desired
}
