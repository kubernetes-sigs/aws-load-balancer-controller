package alb

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
	IngressID          *string
	SvcName            string
	CurrentTags        util.Tags
	DesiredTags        util.Tags
	CurrentTargets     util.AWSStringSlice
	DesiredTargets     util.AWSStringSlice
	CurrentTargetGroup *elbv2.TargetGroup
	DesiredTargetGroup *elbv2.TargetGroup
	deleted            bool
}

// NewTargetGroup returns a new alb.TargetGroup based on the parameters provided.
func NewTargetGroup(annotations *config.Annotations, tags util.Tags, clustername, loadBalancerID *string, port *int64, ingressID *string, svcName string) *TargetGroup {
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
		IngressID:   ingressID,
		ID:          aws.String(id),
		SvcName:     svcName,
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
		log.Infof("Start TargetGroup deletion.", *tg.IngressID)
		if err := tg.delete(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "DELETE", "%s target group deleted", *tg.ID)
		log.Infof("Completed TargetGroup deletion.", *tg.IngressID)

		// No CurrentState means target group doesn't exist in AWS and should be created.
	case tg.CurrentTargetGroup == nil:
		log.Infof("Start TargetGroup creation.", *tg.IngressID)
		if err := tg.create(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s target group created", *tg.ID)
		log.Infof("Succeeded TargetGroup creation. ARN: %s | Name: %s.",
			*tg.IngressID, *tg.CurrentTargetGroup.TargetGroupArn,
			*tg.CurrentTargetGroup.TargetGroupName)

		// Current and Desired exist and need for modification should be evaluated.
	case tg.needsModification():
		log.Infof("Start TargetGroup modification.", *tg.IngressID)
		if err := tg.modify(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s target group modified", *tg.ID)
		log.Infof("Succeeded TargetGroup modification. ARN: %s | Name: %s.",
			*tg.IngressID, *tg.CurrentTargetGroup.TargetGroupArn,
			*tg.CurrentTargetGroup.TargetGroupName)

	default:
		log.Debugf("No TargetGroup modification required.", *tg.IngressID)
	}

	return nil
}

// Creates a new TargetGroup in AWS.
func (tg *TargetGroup) create(rOpts *ReconcileOptions) error {
	lb := rOpts.loadbalancer
	// Target group in VPC for which ALB will route to
	in := elbv2.CreateTargetGroupInput{
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
		VpcId: lb.CurrentLoadBalancer.VpcId,
	}

	o, err := awsutil.ALBsvc.AddTargetGroup(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating target group %s: %s", *tg.ID, err.Error())
		log.Infof("Failed TargetGroup creation. Error: %s.", *tg.IngressID, err.Error())
		return err
	}
	tg.CurrentTargetGroup = o

	// Add tags
	if err = awsutil.ALBsvc.UpdateTags(tg.CurrentTargetGroup.TargetGroupArn, tg.CurrentTags, tg.DesiredTags); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error tagging target group %s: %s", *tg.ID, err.Error())
		log.Infof("Failed TargetGroup creation. Unable to add tags. Error: %s.",
			*tg.IngressID, err.Error())
		return err
	}
	tg.CurrentTags = tg.DesiredTags

	// Register Targets
	if err = tg.registerTargets(); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error registering targets to target group %s: %s", *tg.ID, err.Error())
		log.Infof("Failed TargetGroup creation. Unable to register targets. Error:  %s.",
			*tg.IngressID, err.Error())
		return err
	}

	return nil
}

// Modifies the attributes of an existing TargetGroup.
// ALBIngress is only passed along for logging
func (tg *TargetGroup) modify(rOpts *ReconcileOptions) error {
	// check/change attributes
	if tg.needsModification() {
		in := elbv2.ModifyTargetGroupInput{
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
			log.Errorf("Failed TargetGroup modification. ARN: %s | Error: %s.",
				*tg.IngressID, *tg.CurrentTargetGroup.TargetGroupArn, err.Error())
			return err
		}
		tg.CurrentTargetGroup = o
		// AmazonAPI doesn't return an empty HealthCheckPath.
		tg.CurrentTargetGroup.HealthCheckPath = tg.DesiredTargetGroup.HealthCheckPath
	}

	// check/change tags
	if *tg.CurrentTags.Hash() != *tg.DesiredTags.Hash() {
		if err := awsutil.ALBsvc.UpdateTags(tg.CurrentTargetGroup.TargetGroupArn, tg.CurrentTags, tg.DesiredTags); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error changing tags on target group %s: %s", *tg.ID, err.Error())
			log.Errorf("Failed TargetGroup modification. Unable to modify tags. ARN: %s | Error: %s.",
				*tg.IngressID, *tg.CurrentTargetGroup.TargetGroupArn, err.Error())
			return err
		}
		tg.CurrentTags = tg.DesiredTags
	}

	// check/change targets
	if *tg.CurrentTargets.Hash() != *tg.DesiredTargets.Hash() {
		if err := tg.registerTargets(); err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error modifying targets in target group %s: %s", *tg.ID, err.Error())
			log.Infof("Failed TargetGroup modification. Unable to change targets. Error: %s.",
				*tg.IngressID, err.Error())
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
		log.Errorf("Failed TargetGroup deletion. ARN: %s.", *tg.IngressID, *tg.CurrentTargetGroup.TargetGroupArn)
		return err
	}

	tg.deleted = true
	return nil
}

func (tg *TargetGroup) needsModification() bool {
	ctg := tg.CurrentTargetGroup
	dtg := tg.DesiredTargetGroup

	switch {
	// No target group set currently exists; modification required.
	case ctg == nil:
		log.Debugf("Current Target Group is undefined", *tg.IngressID)
		return true
	case !util.DeepEqual(ctg.HealthCheckIntervalSeconds, dtg.HealthCheckIntervalSeconds):
		log.Debugf("HealthCheckIntervalSeconds needs to be changed (%v != %v)", *tg.IngressID, awsutil.Prettify(ctg.HealthCheckIntervalSeconds), awsutil.Prettify(dtg.HealthCheckIntervalSeconds))
		return true
	case !util.DeepEqual(ctg.HealthCheckPath, dtg.HealthCheckPath):
		log.Debugf("HealthCheckPath needs to be changed (%v != %v)", *tg.IngressID, awsutil.Prettify(ctg.HealthCheckPath), awsutil.Prettify(dtg.HealthCheckPath))
		return true
	case !util.DeepEqual(ctg.HealthCheckPort, dtg.HealthCheckPort):
		log.Debugf("HealthCheckPort needs to be changed (%v != %v)", *tg.IngressID, awsutil.Prettify(ctg.HealthCheckPort), awsutil.Prettify(dtg.HealthCheckPort))
		return true
	case !util.DeepEqual(ctg.HealthCheckProtocol, dtg.HealthCheckProtocol):
		log.Debugf("HealthCheckProtocol needs to be changed (%v != %v)", *tg.IngressID, awsutil.Prettify(ctg.HealthCheckProtocol), awsutil.Prettify(dtg.HealthCheckProtocol))
		return true
	case !util.DeepEqual(ctg.HealthCheckTimeoutSeconds, dtg.HealthCheckTimeoutSeconds):
		log.Debugf("HealthCheckTimeoutSeconds needs to be changed (%v != %v)", *tg.IngressID, awsutil.Prettify(ctg.HealthCheckTimeoutSeconds), awsutil.Prettify(dtg.HealthCheckTimeoutSeconds))
		return true
	case !util.DeepEqual(ctg.HealthyThresholdCount, dtg.HealthyThresholdCount):
		log.Debugf("HealthyThresholdCount needs to be changed (%v != %v)", *tg.IngressID, awsutil.Prettify(ctg.HealthyThresholdCount), awsutil.Prettify(dtg.HealthyThresholdCount))
		return true
	case !util.DeepEqual(ctg.Matcher, dtg.Matcher):
		log.Debugf("Matcher needs to be changed (%v != %v)", *tg.IngressID, awsutil.Prettify(ctg.Matcher), awsutil.Prettify(ctg.Matcher))
		return true
	case !util.DeepEqual(ctg.UnhealthyThresholdCount, dtg.UnhealthyThresholdCount):
		log.Debugf("UnhealthyThresholdCount needs to be changed (%v != %v)", *tg.IngressID, awsutil.Prettify(ctg.UnhealthyThresholdCount), awsutil.Prettify(dtg.UnhealthyThresholdCount))
		return true
	case *tg.CurrentTargets.Hash() != *tg.DesiredTargets.Hash():
		log.Debugf("Targets need to be changed.", *tg.IngressID)
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

	in := elbv2.RegisterTargetsInput{
		TargetGroupArn: tg.CurrentTargetGroup.TargetGroupArn,
		Targets:        targets,
	}

	if err := awsutil.ALBsvc.RegisterTargets(in); err != nil {
		return err
	}

	tg.CurrentTargets = tg.DesiredTargets
	return nil
}

// TODO: Must be implemented
func (tg *TargetGroup) online() bool {
	return true
}
