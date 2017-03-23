package controller

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
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

type TargetGroups []*TargetGroup

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

	for {
		glog.Infof("%s: Waiting for target group %s to be online", a.Name(), *tg.id)
		if tg.online(a) == true {
			break
		}
		time.Sleep(5 * time.Second)
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
	glog.Infof("%s: Changing TargetGroup attributes not yet implemented", a.Name())

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
	// TODO
	return true
}

func (t TargetGroups) find(tg *TargetGroup) int {
	for p, v := range t {
		if *v.id == *tg.id {
			return p
		}
	}
	return -1
}

func (t TargetGroups) modify(a *albIngress, lb *LoadBalancer) error {
	var tg TargetGroups

	for _, targetGroup := range lb.TargetGroups {
		if targetGroup.DesiredTargetGroup == nil {
			lb.Listeners = lb.Listeners.purgeTargetGroupArn(a, targetGroup.CurrentTargetGroup.TargetGroupArn)
			targetGroup.delete(a)
			continue
		}
		err := targetGroup.modify(a, lb)
		if err != nil {
			return err
		}
		tg = append(tg, targetGroup)
	}

	lb.TargetGroups = tg
	return nil
}

func (t TargetGroups) delete(a *albIngress) error {
	errors := false
	for _, targetGroup := range t {
		if err := targetGroup.delete(a); err != nil {
			glog.Infof("%s: Unable to delete target group %s: %s",
				a.Name(),
				*targetGroup.CurrentTargetGroup.TargetGroupArn,
				err)
			errors = true
		}
	}
	if errors {
		return fmt.Errorf("There were errors deleting target groups")
	}
	return nil
}
