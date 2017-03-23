package controller

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

type TargetGroup struct {
	id                 *string
	CurrentTags        Tags
	DesiredTags        Tags
	CurrentTargets     AwsStringSlice
	DesiredTargets     AwsStringSlice
	CurrentTargetGroup *elbv2.TargetGroup
	DesiredTargetGroup *elbv2.TargetGroup
}

func NewTargetGroup(annotations *annotationsT, tags Tags, clustername, loadBalancerID *string, port *int64) *TargetGroup {
	hasher := md5.New()
	hasher.Write([]byte(*loadBalancerID))
	output := hex.EncodeToString(hasher.Sum(nil))

	id := fmt.Sprintf("%.12s-%.5d-%.5s-%.7s", *clustername, *port, *annotations.backendProtocol, output)

	targetGroup := &TargetGroup{
		id:          aws.String(id),
		DesiredTags: tags,
		DesiredTargetGroup: &elbv2.TargetGroup{
			HealthCheckPath:            annotations.healthcheckPath,
			HealthCheckIntervalSeconds: aws.Int64(30),
			HealthCheckPort:            aws.String("traffic-port"),
			HealthCheckProtocol:        annotations.backendProtocol,
			HealthCheckTimeoutSeconds:  aws.Int64(5),
			HealthyThresholdCount:      aws.Int64(5),
			// LoadBalancerArns:
			Matcher:                 &elbv2.Matcher{HttpCode: annotations.successCodes},
			Port:                    port,
			Protocol:                annotations.backendProtocol,
			TargetGroupName:         aws.String(id),
			UnhealthyThresholdCount: aws.Int64(2),
			// VpcId:
		},
	}

	return targetGroup
}

// Creates a new TargetGroup in AWS.
func (tg *TargetGroup) create(a *albIngress, lb *LoadBalancer) error {
	// Debug logger to introspect CreateTargetGroup request
	glog.Infof("%s: Create TargetGroup %s", a.Name(), *tg.id)

	// Target group in VPC for which ALB will route to
	targetParams := &elbv2.CreateTargetGroupInput{
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

	createTargetGroupOutput, err := elbv2svc.svc.CreateTargetGroup(targetParams)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateTargetGroup"}).Add(float64(1))
		return err
	}

	tg.CurrentTargetGroup = createTargetGroupOutput.TargetGroups[0]

	// Add tags
	if err = elbv2svc.setTags(tg.CurrentTargetGroup.TargetGroupArn, tg.DesiredTags); err != nil {
		return err
	}

	tg.CurrentTags = tg.DesiredTags

	// Register Targets
	if err = tg.registerTargets(a); err != nil {
		return err
	}

	return nil
}

// Modifies the attributes of an existing TargetGroup.
// albIngress is only passed along for logging
func (tg *TargetGroup) modify(a *albIngress, lb *LoadBalancer) error {
	if tg.CurrentTargetGroup == nil {
		glog.Info("%s: tg.modify called with empty TargetGroup, assuming we need to make it", a.Name())
		return tg.create(a, lb)

	}

	// check/change attributes
	if tg.needsModification() {
		glog.Infof("%s: Changing TargetGroup attributes", a.Name())
		params := &elbv2.ModifyTargetGroupInput{
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
		modifyTargetGroupOutput, err := elbv2svc.svc.ModifyTargetGroup(params)
		if err != nil {
			return fmt.Errorf("%s: Failure Modifying %s Target Group: %s", a.Name(), *tg.CurrentTargetGroup.TargetGroupArn, err)
		}
		tg.CurrentTargetGroup = modifyTargetGroupOutput.TargetGroups[0]
		// AmazonAPI doesn't return an empty HealthCheckPath.
		tg.CurrentTargetGroup.HealthCheckPath = tg.DesiredTargetGroup.HealthCheckPath
	}

	// check/change tags
	if *tg.CurrentTags.Hash() != *tg.DesiredTags.Hash() {
		glog.Infof("%s: Modifying %s tags", a.Name(), *tg.id)
		if err := elbv2svc.setTags(tg.CurrentTargetGroup.TargetGroupArn, tg.DesiredTags); err != nil {
			glog.Errorf("%s: Error setting tags on %s: %s", a.Name(), *tg.id, err)
		}
		tg.CurrentTags = tg.DesiredTags
	}

	// check/change targets
	if *tg.CurrentTargets.Hash() != *tg.DesiredTargets.Hash() {
		tg.registerTargets(a)
	}

	return nil
}

// Deletes a TargetGroup in AWS.
func (tg *TargetGroup) delete(a *albIngress) error {
	glog.Infof("%s: Delete TargetGroup %s", a.Name(), *tg.id)

	_, err := elbv2svc.svc.DeleteTargetGroup(&elbv2.DeleteTargetGroupInput{
		TargetGroupArn: tg.CurrentTargetGroup.TargetGroupArn,
	})
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DeleteTargetGroup"}).Add(float64(1))
		return err
	}

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
func (tg *TargetGroup) registerTargets(a *albIngress) error {
	glog.Infof("%s: Registering targets to %s", a.Name(), *tg.id)

	targets := []*elbv2.TargetDescription{}
	for _, target := range tg.DesiredTargets {
		targets = append(targets, &elbv2.TargetDescription{
			Id:   target,
			Port: tg.CurrentTargetGroup.Port,
		})
	}

	registerParams := &elbv2.RegisterTargetsInput{
		TargetGroupArn: tg.CurrentTargetGroup.TargetGroupArn,
		Targets:        targets,
	}

	_, err := elbv2svc.svc.RegisterTargets(registerParams)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "RegisterTargets"}).Add(float64(1))
		return err
	}

	tg.CurrentTargets = tg.DesiredTargets
	return nil
}

func (tg *TargetGroup) online(a *albIngress) bool {
	glog.Infof("%s: NOT IMPLEMENTED: Waiting for %s targets to come online", a.Name(), *tg.id)
	return true
}
