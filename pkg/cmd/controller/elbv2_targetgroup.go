package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

type TargetGroup struct {
	id          string
	port        int32
	targets     []string
	TargetGroup *elbv2.TargetGroup
}

// Creates a new TargetGroup in AWS.
func (tg *TargetGroup) create(a *albIngress, lb *LoadBalancer) error {
	// Debug logger to introspect CreateTargetGroup request
	glog.Infof("%s: Create TargetGroup", a.Name())
	if noop {
		return nil
	}

	// Target group in VPC for which ALB will route to
	targetParams := &elbv2.CreateTargetGroupInput{
		Name: aws.String(tg.id),
		// Port:            aws.Int64(int64(a.nodePort)),
		Protocol:        aws.String("HTTP"),
		HealthCheckPath: a.annotations.healthcheckPath,
		Matcher:         &elbv2.Matcher{HttpCode: a.annotations.successCodes},
		VpcId:           aws.String(lb.vpcID),
	}

	// TODO tags

	_, err := elbv2svc.svc.CreateTargetGroup(targetParams)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateTargetGroup"}).Add(float64(1))
		return err
	}

	return nil
}

// Modifies the attributes of an existing ALB.
// albIngress is only passed along for logging
func (tg *TargetGroup) modify(a *albIngress, lb *LoadBalancer) error {
	needsModify := tg.checkModify(a, lb)

	if !needsModify {
		return nil
	}

	glog.Infof("%s: Modifying existing %s target group %s", a.Name(), lb.id, tg.id)
	glog.Infof("%s: NOT IMPLEMENTED!!!!", a.Name())

	// probably just always create new TG and then delete old one

	// 		mod := &elbv2.ModifyTargetGroupInput{
	// 			HealthCheckPath: a.annotations.healthcheckPath,
	// 			Matcher:         &elbv2.Matcher{HttpCode: a.annotations.successCodes},
	// 		}
	// 		_, err := elb.svc.ModifyTargetGroup(mod)
	// 		if err != nil {
	// 			AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "ModifyTargetGroup"}).Add(float64(1))
	// 		}
	// 		return err

	return nil
}

// Deletes a TargetGroup in AWS.
func (tg *TargetGroup) delete(a *albIngress) error {
	glog.Infof("%s: Delete TargetGroup %s", a.Name(), *tg.TargetGroup.TargetGroupArn)
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
func (tg *TargetGroup) registerTargets(a *albIngress) error {
	glog.Infof("%s: Registering targets to %s", a.Name(), tg.id)
	if noop {
		return nil
	}

	targets := []*elbv2.TargetDescription{}
	for _, target := range tg.targets {
		targets = append(targets, &elbv2.TargetDescription{
			Id:   aws.String(target),
			Port: aws.Int64(int64(tg.port)),
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

	return nil
}

func (tg *TargetGroup) checkModify(a *albIngress, lb *LoadBalancer) bool {
	switch {
	// TODO health check interval seconds changed
	// TODO health check path changed
	// TODO health check port changed
	// TODO health check protocol changed
	// TODO health check timeout changed
	// TODO healthy threshold count changed
	// TODO matcher changed
	// TODO name changed ?
	// TODO port changed
	// TODO protocol changed
	// TODO unhealthy threshhold count changed
	// TODO vpc id changed ?
	default:
		return true
	}
}
