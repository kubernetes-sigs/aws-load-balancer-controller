package controller

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

type TargetGroup struct {
	clustername *string
	id          *string
	port        *int64
	targets     NodeSlice
	arn         *string
	TargetGroup *elbv2.TargetGroup
}

func NewTargetGroup(clustername *string, port *int64, nodes NodeSlice) *TargetGroup {
	targetGroup := &TargetGroup{
		port:        port,
		targets:     nodes,
		clustername: clustername,
	}
	targetGroup.id = aws.String(targetGroup.generateID())
	return targetGroup
}

// Creates a new TargetGroup in AWS.
func (tg *TargetGroup) create(a *albIngress, lb *LoadBalancer) error {
	// Debug logger to introspect CreateTargetGroup request
	glog.Infof("%s: Create TargetGroup %s", a.Name(), *tg.id)
	if noop {
		tg.arn = aws.String("somearn")
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

	tg.arn = createTargetGroupOutput.TargetGroups[0].TargetGroupArn
	tg.TargetGroup = createTargetGroupOutput.TargetGroups[0]

	// Add tags
	if err = tg.addTags(a, tg.arn); err != nil {
		return err
	}

	// Register Targets
	if err = tg.registerTargets(a); err != nil {
		return err
	}

	return nil
}

// Modifies the attributes of an existing ALB.
// albIngress is only passed along for logging
func (tg *TargetGroup) modify(a *albIngress, lb *LoadBalancer) (*TargetGroup, error) {
	newTargetGroup := NewTargetGroup(tg.clustername, tg.port, a.nodes)
	if *newTargetGroup.id == *tg.id {
		glog.Infof("%s: Target group %v has not changed", a.Name(), *tg.id)
		return tg, nil
	}

	glog.Infof("%s: Replacing existing %s target group %s", a.Name(), *lb.id, *tg.id)

	err := newTargetGroup.create(a, lb)
	if err != nil {
		return tg, err
	}

	glog.Infof("%s: New %s target group %s", a.Name(), *lb.id, *newTargetGroup.id)
	// should wait for newTargetGroup targets to be healthy

	tg.delete(a)

	return newTargetGroup, nil
}

// Deletes a TargetGroup in AWS.
func (tg *TargetGroup) delete(a *albIngress) error {
	glog.Infof("%s: Delete TargetGroup %s", a.Name(), *tg.id)
	if noop {
		return nil
	}

	_, err := elbv2svc.svc.DeleteTargetGroup(&elbv2.DeleteTargetGroupInput{
		TargetGroupArn: tg.arn,
	})
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DeleteTargetGroup"}).Add(float64(1))
		return err
	}

	return nil
}

// Registers Targets (ec2 instances) to a pre-existing TargetGroup in AWS
func (tg *TargetGroup) registerTargets(a *albIngress) error {
	glog.Infof("%s: Registering targets to %s", a.Name(), *tg.id)
	if noop {
		return nil
	}

	targets := []*elbv2.TargetDescription{}
	for _, target := range tg.targets {
		targets = append(targets, &elbv2.TargetDescription{
			Id:   target,
			Port: tg.port,
		})
	}

	registerParams := &elbv2.RegisterTargetsInput{
		TargetGroupArn: tg.arn,
		Targets:        targets,
	}

	_, err := elbv2svc.svc.RegisterTargets(registerParams)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "RegisterTargets"}).Add(float64(1))
		return err
	}

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

// hash will need to include anything we want to use to re-identify the TargetGroup
// when the newAlbIngressesFromIngress is called. its how we match existing to k8s objects
func (tg *TargetGroup) Hash() string {
	hasher := md5.New()
	hasher.Write([]byte(fmt.Sprintf("%v%v", *tg.targets.Hash(), *tg.port)))
	output := hex.EncodeToString(hasher.Sum(nil))
	return output
}

func (tg *TargetGroup) generateID() string {
	name := fmt.Sprintf("%s-%s", *tg.clustername, tg.Hash())
	return name[0:32]
}
