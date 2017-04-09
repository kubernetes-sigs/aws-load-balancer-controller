package alb

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/awsutil"
	"github.com/coreos/alb-ingress-controller/controller/config"
	"github.com/coreos/alb-ingress-controller/controller/util"
	"github.com/coreos/alb-ingress-controller/log"
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
}

// NewTargetGroup returns a new alb.TargetGroup based on the parameters provided.
func NewTargetGroup(annotations *config.AnnotationsT, tags util.Tags, clustername, loadBalancerID *string, port *int64, ingressID *string, svcName string) *TargetGroup {
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
			HealthCheckIntervalSeconds: aws.Int64(30),
			HealthCheckPort:            aws.String("traffic-port"),
			HealthCheckProtocol:        annotations.BackendProtocol,
			HealthCheckTimeoutSeconds:  aws.Int64(5),
			HealthyThresholdCount:      aws.Int64(5),
			// LoadBalancerArns:
			Matcher:                 &elbv2.Matcher{HttpCode: annotations.SuccessCodes},
			Port:                    port,
			Protocol:                annotations.BackendProtocol,
			TargetGroupName:         aws.String(id),
			UnhealthyThresholdCount: aws.Int64(2),
			// VpcId:
		},
	}

	return targetGroup
}

// SyncState compares the current and desired state of this TargetGroup instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS target group to
// satisfy the ingress's current state.
func (tg *TargetGroup) SyncState(lb *LoadBalancer) *TargetGroup {
	switch {
	// No DesiredState means target group should be deleted.
	case tg.DesiredTargetGroup == nil:
		log.Infof("Start TargetGroup deletion.", *tg.IngressID)
		tg.delete()

	// No CurrentState means target group doesn't exist in AWS and should be created.
	case tg.CurrentTargetGroup == nil:
		log.Infof("Start TargetGroup creation.", *tg.IngressID)
		tg.create(lb)

	// Current and Desired exist and need for modification should be evaluated.
	case tg.needsModification():
		log.Infof("Start TargetGroup modification.", *tg.IngressID)
		tg.modify(lb)

	default:
		log.Debugf("No TargetGroup modification required.", *tg.IngressID)
	}

	return tg
}

// Creates a new TargetGroup in AWS.
func (tg *TargetGroup) create(lb *LoadBalancer) error {
	// Debug logger to introspect CreateTargetGroup request

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
		log.Infof("Failed TargetGroup creation. Error: %s.", *tg.IngressID, err.Error())
		return err
	}
	tg.CurrentTargetGroup = o

	// Add tags
	if err = awsutil.ALBsvc.UpdateTags(tg.CurrentTargetGroup.TargetGroupArn, tg.CurrentTags, tg.DesiredTags); err != nil {
		log.Infof("Failed TargetGroup creation. Unable to add tags. Error: %s.",
			*tg.IngressID, err.Error())
		return err
	}
	tg.CurrentTags = tg.DesiredTags

	// Register Targets
	if err = tg.registerTargets(); err != nil {
		log.Infof("Failed TargetGroup creation. Unable to register targets. Error:  %s.",
			*tg.IngressID, err.Error())
		return err
	}

	log.Infof("Succeeded TargetGroup creation. ARN: %s | Name: %s.",
		*tg.IngressID, *tg.CurrentTargetGroup.TargetGroupArn, *tg.CurrentTargetGroup.TargetGroupName)
	return nil
}

// Modifies the attributes of an existing TargetGroup.
// ALBIngress is only passed along for logging
func (tg *TargetGroup) modify(lb *LoadBalancer) error {
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
			log.Errorf("Failed TargetGroup modification. Unable to modify tags. ARN: %s | Error: %s.",
				*tg.IngressID, *tg.CurrentTargetGroup.TargetGroupArn, err.Error())
		}
		tg.CurrentTags = tg.DesiredTags
	}

	// check/change targets
	if *tg.CurrentTargets.Hash() != *tg.DesiredTargets.Hash() {
		tg.registerTargets()
	}

	log.Infof("Succeeded TargetGroup modification. ARN: %s | Name: %s.",
		*tg.IngressID, *tg.CurrentTargetGroup.TargetGroupArn, *tg.CurrentTargetGroup.TargetGroupName)
	return nil
}

// Deletes a TargetGroup in AWS.
func (tg *TargetGroup) delete() error {
	in := elbv2.DeleteTargetGroupInput{TargetGroupArn: tg.CurrentTargetGroup.TargetGroupArn}
	if err := awsutil.ALBsvc.RemoveTargetGroup(in); err != nil {
		log.Errorf("Failed TargetGroup deletion. ARN: %s.", *tg.IngressID, *tg.CurrentTargetGroup.TargetGroupArn)
		return err
	}
	log.Infof("Completed TargetGroup deletion. ARN: %s.", *tg.IngressID, *tg.CurrentTargetGroup.TargetGroupArn)
	return nil
}

func (tg *TargetGroup) needsModification() bool {
	ctg := tg.CurrentTargetGroup
	dtg := tg.DesiredTargetGroup

	switch {
	// No target group set currently exists; modification required.
	case ctg == nil:
		return true
	case *ctg.HealthCheckIntervalSeconds != *dtg.HealthCheckIntervalSeconds:
		return true
	case *ctg.HealthCheckPath != *dtg.HealthCheckPath:
		return true
	case *ctg.HealthCheckPort != *dtg.HealthCheckPort:
		return true
	case *ctg.HealthCheckProtocol != *dtg.HealthCheckProtocol:
		return true
	case *ctg.HealthCheckTimeoutSeconds != *dtg.HealthCheckTimeoutSeconds:
		return true
	case *ctg.HealthyThresholdCount != *dtg.HealthyThresholdCount:
		return true
	case awsutil.Prettify(ctg.Matcher) != awsutil.Prettify(dtg.Matcher):
		return true
	case *ctg.UnhealthyThresholdCount != *dtg.UnhealthyThresholdCount:
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
