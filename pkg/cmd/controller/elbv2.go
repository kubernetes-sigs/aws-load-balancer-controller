package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"fmt"
	"errors"
)

// ELBV2 is our extension to AWS's elbv2.ELBV2
type ELBV2 struct {
	*elbv2.ELBV2
	// using an ec2 client to resolve which VPC a subnet
	// belongs to
	*ec2.EC2
	// LB created; otherwise nil
	*elbv2.LoadBalancer
}

const (
	subnet1Key = "ticketmaster.com/ingress.subnet.1"
	subnet2Key = "ticketmaster.com/ingress.subnet.2"
)

func newELBV2(awsconfig *aws.Config) *ELBV2 {
	session, err := session.NewSession(awsconfig)
	if err != nil {
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	// Temporary for tests
	// TODO: Auto-resolve
	region :="us-west-1"
	session.Config.Region = &region

	elbv2 := ELBV2{
		elbv2.New(session),
		ec2.New(session),
		nil,
	}
	return &elbv2
}

// initial function to test creation of ALB
// WIP
// TODO: This massive method will be refactored ;)
func (elb *ELBV2) createALB(a *albIngress) error {
	// Verify subnet keys are present before starting ALB creation.
	if a.annotations[subnet1Key] == "" || a.annotations[subnet2Key] == "" {
		return errors.New("One or both ALB subnet annotations missing. Canceling ALB creation.")
	}
	// this should automatically be resolved up stack
	// TODO: Remove once resolving correctly
	a.clusterName = "TEMPCLUSTERNAME"
	albName := fmt.Sprintf("%s-%s", a.clusterName, a.serviceName)

	tGroupResp, err := elb.createTargetGroup(a, &albName)
	if err != nil { return err }
	err = elb.registerTargets(a, tGroupResp)
	if err != nil { return err }

	alb := &elbv2.CreateLoadBalancerInput{
		Name: &albName,
		Subnets: []*string{aws.String(a.annotations[subnet1Key]), aws.String(a.annotations[subnet2Key])},
	}

	resp, err := elb.CreateLoadBalancer(alb)
	if err != nil {
		fmt.Printf("ALB CREATION FAILED: %s", err.Error())
		return err
	}
	fmt.Printf("ALB CREATION SUCCEEDED: %s", resp)
	elb.LoadBalancer = resp.LoadBalancers[0]
	return nil
}

func (elb *ELBV2) createTargetGroup(a *albIngress, albName *string) (*elbv2.CreateTargetGroupOutput, error) {
	descRequest := &ec2.DescribeSubnetsInput { SubnetIds: []*string{aws.String(a.annotations[subnet1Key])}, }
	subnetInfo, err := elb.EC2.DescribeSubnets(descRequest)


	if err != nil {
		fmt.Printf("Failed to lookup vpcID before creating target group: %s", err.Error())
		return nil, err
	}

	vpcID := subnetInfo.Subnets[0].VpcId

	// Target group in VPC for which ALB will route to
	targetParams := &elbv2.CreateTargetGroupInput{
		Name: albName,
		Port: aws.Int64(int64(a.nodePort)),
		Protocol: aws.String("HTTP"),
		VpcId: vpcID,
	}

	tGroupResp, err := elb.CreateTargetGroup(targetParams)

	if err != nil {
		fmt.Printf("Target Group failed to create: %s", err.Error())
		return nil, err
	}
	fmt.Printf("Target Group CREATION SUCCEEDED: %s", tGroupResp)
	return tGroupResp, err
}

func (elb *ELBV2) registerTargets(a *albIngress, tGroupResp *elbv2.CreateTargetGroupOutput) error {

	descRequest := &ec2.DescribeSubnetsInput { SubnetIds: []*string{aws.String(a.annotations[subnet1Key])}, }
	subnetInfo, err := elb.EC2.DescribeSubnets(descRequest)
	// ugly hack to get all instanceIds for a VPC;
	// we'll eventually go through k8s api
	// TODO: Remove all this in favor of introspecting instanceIds from
	//       albIngress struct
	ec2InstanceFilter := &ec2.Filter{
		Name: aws.String("vpc-id"),
		Values: []*string{subnetInfo.Subnets[0].VpcId},
	}
	descInstParams := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{ec2InstanceFilter},
	}
	instances, err := elb.EC2.DescribeInstances(descInstParams)

	// Instance registration for target group
	targets := []*elbv2.TargetDescription{}
	fmt.Printf("are there node ids?!?!?: %s", instances.Reservations[0].Instances)
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			targets = append(targets, &elbv2.TargetDescription{Id: instance.InstanceId, Port: aws.Int64(int64(a.nodePort))})
		}
	}
	registerParams := &elbv2.RegisterTargetsInput{
		TargetGroupArn: tGroupResp.TargetGroups[0].TargetGroupArn,
		Targets: targets,
	}
	fmt.Printf("Targets to register were: %s", registerParams)
	rTargetResp, err := elb.RegisterTargets(registerParams)
	if err != nil {
		fmt.Printf("Failed to register targets to group: %s", err.Error())
		return err
	}
	fmt.Printf("Register Target Group CREATION SUCCEEDED: %s", rTargetResp)
	return nil

}
