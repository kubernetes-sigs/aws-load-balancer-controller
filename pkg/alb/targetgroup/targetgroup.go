package targetgroup

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"

	api "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/config"
	awsutil "github.com/coreos/alb-ingress-controller/pkg/util/aws"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
)

// TargetGroup contains the current/desired tags & targetgroup for the ALB
type TargetGroup struct {
	ID                 *string
	SvcName            string
	CurrentTags        util.Tags
	DesiredTags        util.Tags
	CurrentTargets     util.AWSStringSlice
	DesiredTargets     util.AWSStringSlice
	CurrentTargetGroup *elbv2.TargetGroup
	DesiredTargetGroup *elbv2.TargetGroup
	Deleted            bool
	logger             *log.Logger
}

// NewTargetGroup returns a new targetgroup.TargetGroup based on the parameters provided.
func NewTargetGroup(annotations *config.Annotations, tags util.Tags, clustername, loadBalancerID *string, port *int64, logger *log.Logger, svcName string) *TargetGroup {
	hasher := md5.New()
	hasher.Write([]byte(*loadBalancerID))
	output := hex.EncodeToString(hasher.Sum(nil))

	id := fmt.Sprintf("%.12s-%.5d-%.5s-%.7s", *clustername, *port, *annotations.BackendProtocol, output)

	// Add the service name tag to the Target group as it's needed when reassembling ingresses after
	// controller relaunch.
	tags = append(tags, &elbv2.Tag{
		Key: aws.String("ServiceName"), Value: aws.String(svcName)})

	// TODO: Quick fix as we can't have the loadbalancer and target groups share pointers to the same
	// tags. Each modify tags individually and can cause bad side-effects.
	newTagList := []*elbv2.Tag{}
	for _, tag := range tags {
		key := *tag.Key
		value := *tag.Value

		newTag := &elbv2.Tag{
			Key:   &key,
			Value: &value,
		}
		newTagList = append(newTagList, newTag)
	}

	targetGroup := &TargetGroup{
		ID:          aws.String(id),
		SvcName:     svcName,
		logger:      logger,
		DesiredTags: newTagList,
		DesiredTargetGroup: &elbv2.TargetGroup{
			HealthCheckPath:            annotations.HealthcheckPath,
			HealthCheckIntervalSeconds: annotations.HealthcheckIntervalSeconds,
			HealthCheckPort:            annotations.HealthcheckPort,
			HealthCheckProtocol:        annotations.BackendProtocol,
			HealthCheckTimeoutSeconds:  annotations.HealthcheckTimeoutSeconds,
			HealthyThresholdCount:      annotations.HealthyThresholdCount,
			// LoadBalancerArns:
			Matcher:                 &elbv2.Matcher{HttpCode: annotations.SuccessCodes},
			Port:                    port,
			Protocol:                annotations.BackendProtocol,
			TargetGroupName:         aws.String(id),
			UnhealthyThresholdCount: annotations.UnhealthyThresholdCount,
			// VpcId:
		},
	}

	return targetGroup
}

// NewTargetGroupFromAWSTargetGroup returns a new targetgroup.TargetGroup from an elbv2.TargetGroup.
func NewTargetGroupFromAWSTargetGroup(targetGroup *elbv2.TargetGroup, tags util.Tags, clustername, loadBalancerID string, logger *log.Logger) (*TargetGroup, error) {
	hasher := md5.New()
	hasher.Write([]byte(loadBalancerID))
	output := hex.EncodeToString(hasher.Sum(nil))

	id := fmt.Sprintf("%.12s-%.5d-%.5s-%.7s", clustername, *targetGroup.Port, *targetGroup.Protocol, output)

	svcName, ok := tags.Get("ServiceName")
	if !ok {
		return nil, fmt.Errorf("The Target Group %s does not have a Namespace tag, can't import", *targetGroup.TargetGroupArn)
	}

	return &TargetGroup{
		ID:                 aws.String(id),
		SvcName:            svcName,
		logger:             logger,
		CurrentTags:        tags,
		CurrentTargetGroup: targetGroup,
	}, nil
}

// Reconcile compares the current and desired state of this TargetGroup instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS target group to
// satisfy the ingress's current state.
func (tg *TargetGroup) Reconcile(rOpts *ReconcileOptions) error {
	switch {
	// No DesiredState means target group should be deleted.
	case tg.DesiredTargetGroup == nil:
		if tg.CurrentTargetGroup == nil {
			break
		}
		tg.logger.Infof("Start TargetGroup deletion.")
		if err := tg.delete(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "DELETE", "%s target group deleted", *tg.ID)
		tg.logger.Infof("Completed TargetGroup deletion.")

		// No CurrentState means target group doesn't exist in AWS and should be created.
	case tg.CurrentTargetGroup == nil:
		tg.logger.Infof("Start TargetGroup creation.")
		if err := tg.create(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s target group created", *tg.ID)
		tg.logger.Infof("Succeeded TargetGroup creation. ARN: %s | Name: %s.",
			*tg.CurrentTargetGroup.TargetGroupArn,
			*tg.CurrentTargetGroup.TargetGroupName)

		// Current and Desired exist and need for modification should be evaluated.
	case tg.needsModification():
		tg.logger.Infof("Start TargetGroup modification.")
		if err := tg.modify(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s target group modified", *tg.ID)
		tg.logger.Infof("Succeeded TargetGroup modification. ARN: %s | Name: %s.",
			*tg.CurrentTargetGroup.TargetGroupArn,
			*tg.CurrentTargetGroup.TargetGroupName)

	default:
		tg.logger.Debugf("No TargetGroup modification required.")
	}

	return nil
}

// Creates a new TargetGroup in AWS.
func (tg *TargetGroup) create(rOpts *ReconcileOptions) error {
	// Target group in VPC for which ALB will route to
	in := &elbv2.CreateTargetGroupInput{
		HealthCheckPath:            tg.DesiredTargetGroup.HealthCheckPath,
		HealthCheckIntervalSeconds: tg.DesiredTargetGroup.HealthCheckIntervalSeconds,
		HealthCheckPort:            tg.DesiredTargetGroup.HealthCheckPort,
		HealthCheckProtocol:        tg.DesiredTargetGroup.HealthCheckProtocol,
		HealthCheckTimeoutSeconds:  tg.DesiredTargetGroup.HealthCheckTimeoutSeconds,
		HealthyThresholdCount:      tg.DesiredTargetGroup.HealthyThresholdCount,
		Matcher:                    tg.DesiredTargetGroup.Matcher,
		Port:                       tg.DesiredTargetGroup.Port,
		Protocol:                   tg.DesiredTargetGroup.Protocol,
		Name:                       tg.DesiredTargetGroup.TargetGroupName,
		UnhealthyThresholdCount: tg.DesiredTargetGroup.UnhealthyThresholdCount,
		VpcId: rOpts.VpcID,
	}

	o, err := awsutil.ALBsvc.CreateTargetGroup(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating target group %s: %s", *tg.ID, err.Error())
		tg.logger.Infof("Failed TargetGroup creation: %s.", err.Error())
		return err
	}
	tg.CurrentTargetGroup = o.TargetGroups[0]

	// Add tags
	if err = awsutil.ALBsvc.UpdateTags(tg.CurrentTargetGroup.TargetGroupArn, tg.CurrentTags, tg.DesiredTags); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error tagging target group %s: %s", *tg.ID, err.Error())
		tg.logger.Infof("Failed TargetGroup creation. Unable to add tags: %s.", err.Error())
		return err
	}
	tg.CurrentTags = tg.DesiredTags

	// Register Targets
	if err = tg.registerTargets(); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error registering targets to target group %s: %s", *tg.ID, err.Error())
		tg.logger.Infof("Failed TargetGroup creation. Unable to register targets:  %s.", err.Error())
		return err
	}

	return nil
}

// Modifies the attributes of an existing TargetGroup.
// ALBIngress is only passed along for logging
func (tg *TargetGroup) modify(rOpts *ReconcileOptions) error {
	// check/change attributes
	if tg.needsModification() {
		in := &elbv2.ModifyTargetGroupInput{
			HealthCheckIntervalSeconds: tg.DesiredTargetGroup.HealthCheckIntervalSeconds,
			HealthCheckPath:            tg.DesiredTargetGroup.HealthCheckPath,
			HealthCheckPort:            tg.DesiredTargetGroup.HealthCheckPort,
			HealthCheckProtocol:        tg.DesiredTargetGroup.HealthCheckProtocol,
			HealthCheckTimeoutSeconds:  tg.DesiredTargetGroup.HealthCheckTimeoutSeconds,
			HealthyThresholdCount:      tg.DesiredTargetGroup.HealthyThresholdCount,
			Matcher:                    tg.DesiredTargetGroup.Matcher,
			TargetGroupArn:             tg.CurrentTargetGroup.TargetGroupArn,
			UnhealthyThresholdCount:    tg.DesiredTargetGroup.UnhealthyThresholdCount,
		}
		o, err := awsutil.ALBsvc.ModifyTargetGroup(in)
		if err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error modifying target group %s: %s", *tg.ID, err.Error())
			tg.logger.Errorf("Failed TargetGroup modification. ARN: %s | Error: %s.",
				*tg.CurrentTargetGroup.TargetGroupArn, err.Error())
			return err
		}
		tg.CurrentTargetGroup = o.TargetGroups[0]
		// AmazonAPI doesn't return an empty HealthCheckPath.
		tg.CurrentTargetGroup.HealthCheckPath = tg.DesiredTargetGroup.HealthCheckPath
	}

	// check/change tags
	if *tg.CurrentTags.Hash() != *tg.DesiredTags.Hash() {
		if err := awsutil.ALBsvc.UpdateTags(tg.CurrentTargetGroup.TargetGroupArn, tg.CurrentTags, tg.DesiredTags); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error changing tags on target group %s: %s", *tg.ID, err.Error())
			tg.logger.Errorf("Failed TargetGroup modification. Unable to modify tags. ARN: %s | Error: %s.",
				*tg.CurrentTargetGroup.TargetGroupArn, err.Error())
			return err
		}
		tg.CurrentTags = tg.DesiredTags
	}

	// check/change targets
	if *tg.CurrentTargets.Hash() != *tg.DesiredTargets.Hash() {
		if err := tg.registerTargets(); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error modifying targets in target group %s: %s", *tg.ID, err.Error())
			tg.logger.Infof("Failed TargetGroup modification. Unable to change targets: %s.", err.Error())
			return err
		}

	}

	return nil
}

// Deletes a TargetGroup in AWS.
func (tg *TargetGroup) delete(rOpts *ReconcileOptions) error {
	in := elbv2.DeleteTargetGroupInput{TargetGroupArn: tg.CurrentTargetGroup.TargetGroupArn}
	if err := awsutil.ALBsvc.RemoveTargetGroup(in); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error deleting target group %s: %s", *tg.ID, err.Error())
		tg.logger.Errorf("Failed TargetGroup deletion. ARN: %s.", *tg.CurrentTargetGroup.TargetGroupArn)
		return err
	}

	tg.Deleted = true
	return nil
}

func (tg *TargetGroup) needsModification() bool {
	ctg := tg.CurrentTargetGroup
	dtg := tg.DesiredTargetGroup

	switch {
	// No target group set currently exists; modification required.
	case ctg == nil:
		tg.logger.Debugf("Current Target Group is undefined")
		return true
	case !util.DeepEqual(ctg.HealthCheckIntervalSeconds, dtg.HealthCheckIntervalSeconds):
		tg.logger.Debugf("HealthCheckIntervalSeconds needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckIntervalSeconds), log.Prettify(dtg.HealthCheckIntervalSeconds))
		return true
	case !util.DeepEqual(ctg.HealthCheckPath, dtg.HealthCheckPath):
		tg.logger.Debugf("HealthCheckPath needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckPath), log.Prettify(dtg.HealthCheckPath))
		return true
	case !util.DeepEqual(ctg.HealthCheckPort, dtg.HealthCheckPort):
		tg.logger.Debugf("HealthCheckPort needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckPort), log.Prettify(dtg.HealthCheckPort))
		return true
	case !util.DeepEqual(ctg.HealthCheckProtocol, dtg.HealthCheckProtocol):
		tg.logger.Debugf("HealthCheckProtocol needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckProtocol), log.Prettify(dtg.HealthCheckProtocol))
		return true
	case !util.DeepEqual(ctg.HealthCheckTimeoutSeconds, dtg.HealthCheckTimeoutSeconds):
		tg.logger.Debugf("HealthCheckTimeoutSeconds needs to be changed (%v != %v)", log.Prettify(ctg.HealthCheckTimeoutSeconds), log.Prettify(dtg.HealthCheckTimeoutSeconds))
		return true
	case !util.DeepEqual(ctg.HealthyThresholdCount, dtg.HealthyThresholdCount):
		tg.logger.Debugf("HealthyThresholdCount needs to be changed (%v != %v)", log.Prettify(ctg.HealthyThresholdCount), log.Prettify(dtg.HealthyThresholdCount))
		return true
	case !util.DeepEqual(ctg.Matcher, dtg.Matcher):
		tg.logger.Debugf("Matcher needs to be changed (%v != %v)", log.Prettify(ctg.Matcher), log.Prettify(ctg.Matcher))
		return true
	case !util.DeepEqual(ctg.UnhealthyThresholdCount, dtg.UnhealthyThresholdCount):
		tg.logger.Debugf("UnhealthyThresholdCount needs to be changed (%v != %v)", log.Prettify(ctg.UnhealthyThresholdCount), log.Prettify(dtg.UnhealthyThresholdCount))
		return true
	case *tg.CurrentTargets.Hash() != *tg.DesiredTargets.Hash():
		tg.logger.Debugf("Targets need to be changed.")
		return true
	}
	// These fields require a rebuild and are enforced via TG name hash
	//	Port *int64 `min:"1" type:"integer"`
	//	Protocol *string `type:"string" enum:"ProtocolEnum"`

	return false
}

// Registers Targets (ec2 instances) to the CurrentTargetGroup, must be called when CurrentTargetGroup == DesiredTargetGroup
func (tg *TargetGroup) registerTargets() error {
	targets := []*elbv2.TargetDescription{}
	for _, target := range tg.DesiredTargets {
		targets = append(targets, &elbv2.TargetDescription{
			Id:   target,
			Port: tg.CurrentTargetGroup.Port,
		})
	}

	in := &elbv2.RegisterTargetsInput{
		TargetGroupArn: tg.CurrentTargetGroup.TargetGroupArn,
		Targets:        targets,
	}

	if _, err := awsutil.ALBsvc.RegisterTargets(in); err != nil {
		return err
	}

	tg.CurrentTargets = tg.DesiredTargets
	return nil
}

// TODO: Must be implemented
func (tg *TargetGroup) online() bool {
	return true
}

type ReconcileOptions struct {
	Eventf func(string, string, string, ...interface{})
	VpcID  *string
}

func NewReconcileOptions() *ReconcileOptions {
	return &ReconcileOptions{}
}

func (r *ReconcileOptions) SetVpcID(vpcid *string) *ReconcileOptions {
	r.VpcID = vpcid
	return r
}

func (r *ReconcileOptions) SetEventf(f func(string, string, string, ...interface{})) *ReconcileOptions {
	r.Eventf = f
	return r
}
