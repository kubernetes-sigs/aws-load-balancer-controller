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
	clustername    *string
	id             *string
	loadBalancerID *string
	protocol       *string
	port           *int64
	deleted        bool
	targets        AwsStringSlice
	TargetGroup    *elbv2.TargetGroup
}

type TargetGroups []*TargetGroup

func NewTargetGroup(clustername, protocol, loadBalancerID *string, port *int64) *TargetGroup {
	targetGroup := &TargetGroup{
		loadBalancerID: loadBalancerID,
		protocol:       protocol,
		port:           port,
		targets:        AwsStringSlice{},
		clustername:    clustername,
	}
	targetGroup.id = aws.String(targetGroup.generateID())
	return targetGroup
}

// Creates a new TargetGroup in AWS.
func (tg *TargetGroup) create(a *albIngress, lb *LoadBalancer) error {
	// Debug logger to introspect CreateTargetGroup request
	glog.Infof("%s: Create TargetGroup %s", a.Name(), *tg.id)
	if noop {
		tg.TargetGroup = &elbv2.TargetGroup{TargetGroupArn: aws.String("somearn")}
		return nil
	}

	// Target group in VPC for which ALB will route to
	targetParams := &elbv2.CreateTargetGroupInput{
		Name:            tg.id,
		Port:            tg.port,
		Protocol:        aws.String("HTTP"),
		HealthCheckPath: a.annotations.healthcheckPath,
		Matcher:         &elbv2.Matcher{HttpCode: a.annotations.successCodes},
		VpcId:           lb.vpcID,
	}

	createTargetGroupOutput, err := elbv2svc.svc.CreateTargetGroup(targetParams)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateTargetGroup"}).Add(float64(1))
		return err
	}

	tg.TargetGroup = createTargetGroupOutput.TargetGroups[0]

	// Add tags
	if err = tg.addTags(a, tg.TargetGroup.TargetGroupArn); err != nil {
		return err
	}

	// Register Targets
	if err = tg.registerTargets(a, a.nodes); err != nil {
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
	if tg.TargetGroup == nil {
		glog.Info("tg.modify called with empty TargetGroup, assuming we need to make it")
		return tg.create(a, lb)

	}
	// check/change attributes

	// check/change targets
	if *tg.targets.Hash() != *a.nodes.Hash() {
		tg.registerTargets(a, a.nodes)
	}

	return nil
}

// Deletes a TargetGroup in AWS.
func (tg *TargetGroup) delete(a *albIngress) error {
	glog.Infof("%s: Delete TargetGroup %s", a.Name(), *tg.id)
	if noop {
		return nil
	}

	_, err := elbv2svc.svc.DeleteTargetGroup(&elbv2.DeleteTargetGroupInput{
		TargetGroupArn: tg.TargetGroup.TargetGroupArn,
	})
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DeleteTargetGroup"}).Add(float64(1))
		return err
	}

	return nil
}

// Registers Targets (ec2 instances) to a pre-existing TargetGroup in AWS
func (tg *TargetGroup) registerTargets(a *albIngress, newTargets AwsStringSlice) error {
	glog.Infof("%s: Registering targets to %s", a.Name(), *tg.id)
	if noop {
		return nil
	}

	targets := []*elbv2.TargetDescription{}
	for _, target := range newTargets {
		targets = append(targets, &elbv2.TargetDescription{
			Id:   target,
			Port: tg.port,
		})
	}

	registerParams := &elbv2.RegisterTargetsInput{
		TargetGroupArn: tg.TargetGroup.TargetGroupArn,
		Targets:        targets,
	}

	_, err := elbv2svc.svc.RegisterTargets(registerParams)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "RegisterTargets"}).Add(float64(1))
		return err
	}

	tg.targets = newTargets
	return nil
}

func (tg *TargetGroup) addTags(a *albIngress, arn *string) error {
	// glog.Infof("%s: Adding %v tags to %s", a.Name(), awsutil.Prettify(a.Tags()), *tg.id)
	if noop {
		return nil
	}

	tagParams := &elbv2.AddTagsInput{
		ResourceArns: []*string{arn},
		Tags:         a.Tags(),
	}

	if _, err := elbv2svc.svc.AddTags(tagParams); err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "AddTags"}).Add(float64(1))
		return err
	}

	return nil
}

// unique id for target group. this assumes the only things that require a rebuild
// of the target group is the protocol and the port
// needs to be unique to the load balancer it is made for
func (tg *TargetGroup) generateID() string {
	hasher := md5.New()
	hasher.Write([]byte(*tg.loadBalancerID))
	output := hex.EncodeToString(hasher.Sum(nil))

	name := fmt.Sprintf("%.12s-%.5d-%.5s-%.7s", *tg.clustername, *tg.port, *tg.protocol, output)

	return name
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
		if targetGroup.deleted {
			lb.Listeners = lb.Listeners.purgeTargetGroupArn(a, targetGroup.TargetGroup.TargetGroupArn)
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
				*targetGroup.TargetGroup.TargetGroupArn,
				err)
			errors = true
		}
	}
	if errors {
		return fmt.Errorf("There were errors deleting target groups")
	}
	return nil
}
