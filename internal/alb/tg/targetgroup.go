package tg

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
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
	CommonTags     *tags.Tags
	Store          store.Storer
	LoadBalancerID string
	Backend        *extensions.IngressBackend
}

// NewDesiredTargetGroup returns a new targetgroup.TargetGroup based on the parameters provided.
func NewDesiredTargetGroup(o *NewDesiredTargetGroupOptions) (*TargetGroup, error) {
	id, err := o.generateID()
	if err != nil {
		return nil, err
	}
	tgTags := o.CommonTags.Copy()
	tgTags.Tags[tags.ServiceName] = o.Backend.ServiceName
	tgTags.Tags[tags.ServicePort] = o.Backend.ServicePort.String()

	// Assemble Attributes
	attributes, err := NewAttributes(o.Annotations.TargetGroup.Attributes)
	if err != nil {
		return nil, err
	}

	return &TargetGroup{
		ID:         id,
		SvcName:    o.Backend.ServiceName,
		SvcPort:    o.Backend.ServicePort,
		TargetType: aws.StringValue(o.Annotations.TargetGroup.TargetType),
		tags:       tgTags,
		targets:    NewTargets(aws.StringValue(o.Annotations.TargetGroup.TargetType), o.Ingress, o.Backend),
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
		attributes: attributes,
	}, nil
}

func (o *NewDesiredTargetGroupOptions) generateID() (string, error) {
	hasher := md5.New()
	hasher.Write([]byte(o.LoadBalancerID))
	if o.Backend == nil {
		return "", fmt.Errorf("generateID called without a backend service. this should not happen")
	}
	if o.Annotations == nil {
		return "", fmt.Errorf("generateID called without annotations. this should not happen")
	}
	hasher.Write([]byte(o.Backend.ServiceName))
	hasher.Write([]byte(o.Backend.ServicePort.String()))
	hasher.Write([]byte(aws.StringValue(o.Annotations.TargetGroup.BackendProtocol)))
	hasher.Write([]byte(aws.StringValue(o.Annotations.TargetGroup.TargetType)))

	return fmt.Sprintf("%.12s-%.19s", o.Store.GetConfig().ALBNamePrefix, hex.EncodeToString(hasher.Sum(nil))), nil
}

type NewDesiredTargetGroupFromBackendOptions struct {
	Backend              *extensions.IngressBackend
	CommonTags           *tags.Tags
	LoadBalancerID       string
	Store                store.Storer
	Ingress              *extensions.Ingress
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

	// Start with a new target group with a new Desired state.
	targetGroup, err := NewDesiredTargetGroup(&NewDesiredTargetGroupOptions{
		Ingress:        o.Ingress,
		Annotations:    tgAnnotations,
		CommonTags:     o.CommonTags,
		Store:          o.Store,
		LoadBalancerID: o.LoadBalancerID,
		Backend:        o.Backend,
	})
	if err != nil {
		return nil, err
	}

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

	return &TargetGroup{
		ID:         *o.TargetGroup.TargetGroupName,
		SvcName:    svcName,
		SvcPort:    svcPort,
		targets:    &Targets{},
		attributes: &Attributes{},
		tags:       &tags.Tags{},
		tg:         tg{current: o.TargetGroup},
	}, nil
}

// Reconcile compares the current and desired state of this TargetGroup instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS target group to
// satisfy the ingress's current state.
func (t *TargetGroup) Reconcile(ctx context.Context, rOpts *ReconcileOptions) error {
	switch {
	// No DesiredState means target group may not be needed.
	// However, target groups aren't deleted until after rules are created
	// Ensuring we know what target groups are truly no longer in use.
	case t.tg.desired == nil && !rOpts.IgnoreDeletes:
		albctx.GetLogger(ctx).Infof("Start TargetGroup deletion. ARN: %s | Name: %s.",
			*t.tg.current.TargetGroupArn,
			*t.tg.current.TargetGroupName)
		if err := t.delete(ctx, rOpts); err != nil {
			return err
		}
		albctx.GetEventf(ctx)(api.EventTypeNormal, "DELETE", "%v target group deleted", t.ID)
		albctx.GetLogger(ctx).Infof("Completed TargetGroup deletion. ARN: %s | Name: %s.",
			*t.tg.current.TargetGroupArn,
			*t.tg.current.TargetGroupName)

	case t.tg.desired == nil && rOpts.IgnoreDeletes:
		return nil

		// No CurrentState means target group doesn't exist in AWS and should be created.
	case t.tg.current == nil:
		albctx.GetLogger(ctx).Infof("Start TargetGroup creation.")
		if err := t.create(ctx, rOpts); err != nil {
			return err
		}
		albctx.GetEventf(ctx)(api.EventTypeNormal, "CREATE", "%s target group created", t.ID)
		albctx.GetLogger(ctx).Infof("Succeeded TargetGroup creation. ARN: %s | Name: %s.",
			*t.tg.current.TargetGroupArn,
			*t.tg.current.TargetGroupName)
	default:
		// Current and Desired exist and need for modification should be evaluated.
		if mods := t.needsModification(ctx); mods != 0 {
			if err := t.modify(ctx, mods, rOpts); err != nil {
				return err
			}
			albctx.GetEventf(ctx)(api.EventTypeNormal, "MODIFY", "%s target group modified", t.ID)
		}
	}

	if t.tg.desired != nil {
		t.attributes.TgArn = aws.StringValue(t.tg.current.TargetGroupArn)
		err := rOpts.TgAttributesController.Reconcile(ctx, t.attributes)
		if err != nil {
			return fmt.Errorf("failed configuration of target group attributes due to %s", err.Error())
		}
		t.targets.TgArn = aws.StringValue(t.tg.current.TargetGroupArn)
		err = rOpts.TgTargetsController.Reconcile(ctx, t.targets)
		if err != nil {
			return fmt.Errorf("failed configuration of target group targets due to %s", err.Error())
		}
		t.tags.Arn = aws.StringValue(t.tg.current.TargetGroupArn)
		err = rOpts.TagsController.Reconcile(ctx, t.tags)
		if err != nil {
			return fmt.Errorf("failed configuration of target group tags due to %s", err.Error())
		}

	}
	return nil
}

// Creates a new TargetGroup in AWS.
func (t *TargetGroup) create(ctx context.Context, rOpts *ReconcileOptions) error {
	// Target group in VPC for which ALB will route to
	desired := t.tg.desired
	vpc, err := albec2.EC2svc.GetVPCID()
	if err != nil {
		return err
	}
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
		VpcId:                      vpc,
	}

	o, err := albelbv2.ELBV2svc.CreateTargetGroup(in)
	if err != nil {
		albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "Error creating target group %s: %s", t.ID, err.Error())
		return fmt.Errorf("Failed TargetGroup creation: %s.", err.Error())
	}
	t.tg.current = o.TargetGroups[0]

	return nil
}

// Modifies the attributes of an existing TargetGroup.
// ALBIngress is only passed along for logging
func (t *TargetGroup) modify(ctx context.Context, mods tgChange, rOpts *ReconcileOptions) error {
	desired := t.tg.desired
	if mods&paramsModified != 0 {
		albctx.GetLogger(ctx).Infof("Modifying target group parameters.")
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
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "Error modifying target group %s: %s", t.ID, err.Error())
			return fmt.Errorf("Failed TargetGroup modification. ARN: %s | Error: %s",
				*t.CurrentARN(), err.Error())
		}
		t.tg.current = o.TargetGroups[0]
		// AmazonAPI doesn't return an empty HealthCheckPath.
		t.tg.current.HealthCheckPath = desired.HealthCheckPath
	}

	return nil
}

// delete a TargetGroup.
func (t *TargetGroup) delete(ctx context.Context, rOpts *ReconcileOptions) error {
	if err := albelbv2.ELBV2svc.RemoveTargetGroup(t.CurrentARN()); err != nil {
		albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "Error deleting %v target group: %s", t.ID, err.Error())
		return err
	}
	t.deleted = true
	return nil
}

func (t *TargetGroup) needsModification(ctx context.Context) tgChange {
	var changes tgChange

	ctg := t.tg.current
	dtg := t.tg.desired

	// No target group set currently exists; modification required.
	if ctg == nil {
		albctx.GetLogger(ctx).Debugf("Current Target Group is undefined")
		return changes
	}

	if dtg == nil {
		// albctx.GetLogger(ctx).Debugf("Desired Target Group is undefined")
		return changes
	}

	if !util.DeepEqual(ctg.HealthCheckIntervalSeconds, dtg.HealthCheckIntervalSeconds) {
		albctx.GetLogger(ctx).Debugf("HealthCheckIntervalSeconds needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckIntervalSeconds), log.Prettify(dtg.HealthCheckIntervalSeconds))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.HealthCheckPath, dtg.HealthCheckPath) {
		albctx.GetLogger(ctx).Debugf("HealthCheckPath needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckPath), log.Prettify(dtg.HealthCheckPath))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.HealthCheckPort, dtg.HealthCheckPort) {
		albctx.GetLogger(ctx).Debugf("HealthCheckPort needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckPort), log.Prettify(dtg.HealthCheckPort))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.HealthCheckProtocol, dtg.HealthCheckProtocol) {
		albctx.GetLogger(ctx).Debugf("HealthCheckProtocol needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckProtocol), log.Prettify(dtg.HealthCheckProtocol))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.HealthCheckTimeoutSeconds, dtg.HealthCheckTimeoutSeconds) {
		albctx.GetLogger(ctx).Debugf("HealthCheckTimeoutSeconds needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckTimeoutSeconds), log.Prettify(dtg.HealthCheckTimeoutSeconds))
		changes |= paramsModified
	}
	if !util.DeepEqual(ctg.HealthyThresholdCount, dtg.HealthyThresholdCount) {
		albctx.GetLogger(ctx).Debugf("HealthyThresholdCount needs to be changed (%v != %v)", log.Prettify(ctg.HealthyThresholdCount), log.Prettify(dtg.HealthyThresholdCount))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.Matcher, dtg.Matcher) {
		albctx.GetLogger(ctx).Debugf("Matcher needs to be changed (%v != %v)", log.Prettify(ctg.Matcher), log.Prettify(ctg.Matcher))
		changes |= paramsModified
	}

	if !util.DeepEqual(ctg.UnhealthyThresholdCount, dtg.UnhealthyThresholdCount) {
		albctx.GetLogger(ctx).Debugf("UnhealthyThresholdCount needs to be changed (%v != %v)", log.Prettify(ctg.UnhealthyThresholdCount), log.Prettify(dtg.UnhealthyThresholdCount))
		changes |= paramsModified
	}

	return changes
}

func (t *TargetGroup) CurrentARN() *string {
	if t.tg.current == nil {
		return nil
	}
	return t.tg.current.TargetGroupArn
}

func (t *TargetGroup) TargetDescriptions() []*elbv2.TargetDescription {
	return t.targets.Targets
}

func (t *TargetGroup) StripDesiredState() {
	t.tags = nil
	t.tg.desired = nil
	t.targets = nil
}

func (t *TargetGroup) copyDesiredState(s *TargetGroup) {
	t.attributes = s.attributes
	t.tags = s.tags
	t.targets = s.targets

	t.tg.desired = s.tg.desired
	t.TargetType = s.TargetType
}
