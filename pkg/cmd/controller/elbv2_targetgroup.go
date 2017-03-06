package controller

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

// Creates a new TargetGroup in AWS.
func (elb *ELBV2) createOrModifyTargetGroup(a *albIngress) error {
	targetGroups, err := elb.getTargetGroup(a)
	if err != nil && err.(awserr.Error).Code() != "TargetGroupNotFound" {
		return err
	}

	if len(targetGroups) > 0 {
		targetGroup := targetGroups[0]
		a.targetGroupArn = *targetGroup.TargetGroupArn

		if elb.canModifyTargetGroup(a, targetGroup) {
			mod := &elbv2.ModifyTargetGroupInput{
				HealthCheckPath: a.annotations.healthcheckPath,
				Matcher:         &elbv2.Matcher{HttpCode: a.annotations.successCodes},
			}
			_, err := elb.ModifyTargetGroup(mod)
			if err != nil {
				AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "ModifyTargetGroup"}).Add(float64(1))
			}
			return err
		}

		glog.Info("Found an existing TargetGroup that can not be changed, deleting")
		err := elb.deleteTargetGroup(*targetGroup.TargetGroupArn)
		if err != nil {
			return err
		}
		for {
			glog.Infof("Waiting for TargetGroup %s to finish deleting..", *targetGroup.TargetGroupArn)
			targetGroups, err := elb.getTargetGroup(a)
			if err != nil && err.(awserr.Error).Code() != "TargetGroupNotFound" {
				return err
			}
			if len(targetGroups) == 0 {
				break
			}
			time.Sleep(5 * time.Second)
		}
	}

	glog.Info("Creating Target Group")
	return elb.createTargetGroup(a)
}

// Creates a new TargetGroup in AWS.
func (elb *ELBV2) createTargetGroup(a *albIngress) error {
	// Target group in VPC for which ALB will route to
	targetParams := &elbv2.CreateTargetGroupInput{
		Name:            aws.String(a.id),
		Port:            aws.Int64(int64(a.nodePort)),
		Protocol:        aws.String("HTTP"),
		HealthCheckPath: a.annotations.healthcheckPath,
		Matcher:         &elbv2.Matcher{HttpCode: a.annotations.successCodes},
		VpcId:           aws.String(a.vpcID),
	}

	// Debug logger to introspect CreateTargetGroup request
	glog.Infof("Create TargetGroup request sent:\n%s", targetParams)
	tGroupResp, err := elb.CreateTargetGroup(targetParams)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateTargetGroup"}).Add(float64(1))
		return err
	}

	a.targetGroupArn = *tGroupResp.TargetGroups[0].TargetGroupArn
	return nil
}

func (elb *ELBV2) getTargetGroup(a *albIngress) ([]*elbv2.TargetGroup, error) {
	describeTargetGroupsOutput, err := elb.DescribeTargetGroups(&elbv2.DescribeTargetGroupsInput{
		Names: []*string{aws.String(a.id)},
	})
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DescribeTargetGroups"}).Add(float64(1))
		return nil, err
	}
	return describeTargetGroupsOutput.TargetGroups, nil
}

func (elb *ELBV2) canModifyTargetGroup(a *albIngress, targetGroup *elbv2.TargetGroup) bool {
	switch {
	case int64(a.nodePort) != *targetGroup.Port:
		return false
	case "HTTP" != *targetGroup.Protocol: // We only do HTTP target groups?
		return false
	}
	return true
}

// Deletes all TargetGroups on an ALB.
func (elb *ELBV2) deleteTargetGroups(a *albIngress) error {
	describeParams := &elbv2.DescribeTargetGroupsInput{
		LoadBalancerArn: aws.String(a.loadBalancerArn),
	}
	glog.Infof("Describe TargetGroup request sent:\n%s", describeParams)
	describeResp, err := elb.DescribeTargetGroups(describeParams)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DescribeTargetGroups"}).Add(float64(1))
		return err
	}

	if len(describeResp.TargetGroups) == 0 {
		return nil
	}

	for i := range describeResp.TargetGroups {
		err := elb.deleteTargetGroup(*describeResp.TargetGroups[i].TargetGroupArn)
		if err != nil {
			return err
		}
	}

	return nil
}

// Deletes a TargetGroup in AWS.
func (elb *ELBV2) deleteTargetGroup(arn string) error {
	glog.Infof("Delete TargetGroup request sent:\n%s", arn)
	_, err := elb.DeleteTargetGroup(&elbv2.DeleteTargetGroupInput{
		TargetGroupArn: aws.String(arn),
	})
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DeleteTargetGroup"}).Add(float64(1))
		return err
	}
	return nil
}

// Registers Targets (ec2 instances) to a pre-existing TargetGroup in AWS
func (elb *ELBV2) registerTargets(a *albIngress) error {
	targets := []*elbv2.TargetDescription{}
	for _, target := range a.nodeIds {
		targets = append(targets, &elbv2.TargetDescription{
			Id:   aws.String(target),
			Port: aws.Int64(int64(a.nodePort)),
		})
	}

	registerParams := &elbv2.RegisterTargetsInput{
		TargetGroupArn: aws.String(a.targetGroupArn),
		Targets:        targets,
	}

	glog.Infof("RegisterTargets request sent:\n%s", registerParams)
	_, err := elb.RegisterTargets(registerParams)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "RegisterTargets"}).Add(float64(1))
		return err
	}

	return nil
}
