package controller

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
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

func newELBV2(awsconfig *aws.Config) *ELBV2 {
	awsSession, err := session.NewSession(awsconfig)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2"}).Add(float64(1))
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	elbClient := ELBV2{
		elbv2.New(awsSession),
		ec2.New(awsSession),
		nil,
	}
	return &elbClient
}

// Handles ALB change events to determine whether the ALB must be created, or altered.
// TODO: Implement alter and deletion logic
func (elb *ELBV2) alterALB(a *albIngress) error {

	exists, err := elb.albExists(a)
	if err != nil {
		return err
	}

	if exists {
		elb.modifyALB(a)
	} else {
		err := elb.createALB(a)
		if err != nil {
			return err
		}
	}

	return nil
}

func (elb *ELBV2) deleteALB(a *albIngress) error {
	exists, err := elb.albExists(a)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	glog.Infof("Deleting ALB %v", a.Name())
	err = elb.deleteListeners()
	if err != nil {
		glog.Infof("Unable to delete listeners on %s: %s",
			*a.elbv2.LoadBalancer.LoadBalancerArn,
			err)
	}

	err = elb.deleteTargetGroups(a)
	if err != nil {
		glog.Infof("Unable to delete target groups on %s: %s",
			*a.elbv2.LoadBalancer.LoadBalancerArn,
			err)
	}

	deleteParams := &elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: a.elbv2.LoadBalancer.LoadBalancerArn,
	}
	glog.Infof("Delete LB request sent:\n%s", deleteParams)
	_, err = elb.ELBV2.DeleteLoadBalancer(deleteParams)
	if err != nil {
		return err
	}

	return nil
}

// Modifies the attributes of an existing ALB.
func (elb *ELBV2) modifyALB(a *albIngress) error {

	attr := []*elbv2.LoadBalancerAttribute{}
	if *elb.LoadBalancer.Scheme != *a.annotations.scheme {
		attr = append(attr, &elbv2.LoadBalancerAttribute{
			Key:   aws.String("scheme"),
			Value: a.annotations.scheme,
		})
	}

	params := &elbv2.ModifyLoadBalancerAttributesInput{
		LoadBalancerArn: elb.LoadBalancer.LoadBalancerArn,
		Attributes:      attr,
	}

	// Debug logger to introspect CreateLoadBalancer request
	glog.Infof("Modify LB request sent:\n%s", params)
	_, err := elb.ELBV2.ModifyLoadBalancerAttributes(params)
	if err != nil {
		return err
	}

	return nil
}

// Starts the process of creating a new ALB. If successful, this will create a TargetGroup (TG), register targets in
// the TG, create a ALB, and create a Listener that maps the ALB to the TG in AWS.
func (elb *ELBV2) createALB(a *albIngress) error {
	tGroupResp, err := elb.createTargetGroup(a)
	if err != nil {
		return err
	}
	err = elb.registerTargets(a, tGroupResp)
	if err != nil {
		return err
	}

	albParams := &elbv2.CreateLoadBalancerInput{
		Name:    aws.String(a.Name()),
		Subnets: a.annotations.subnets,
		Scheme:  a.annotations.scheme,
		// Tags:           a.annotations.tags,
		SecurityGroups: a.annotations.securityGroups,
	}

	// Debug logger to introspect CreateLoadBalancer request
	glog.Infof("Create LB request sent:\n%s", albParams)
	resp, err := elb.CreateLoadBalancer(albParams)

	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2"}).Add(float64(1))
		return err
	}
	elb.LoadBalancer = resp.LoadBalancers[0]
	_, err = elb.createListener(a, tGroupResp)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2"}).Add(float64(1))
		return err
	}

	glog.Infof("ALB %s finished creation", *elb.LoadBalancerName)
	return nil
}

// Creates a new TargetGroup in AWS.
func (elb *ELBV2) createTargetGroup(a *albIngress) (*elbv2.CreateTargetGroupOutput, error) {
	descRequest := &ec2.DescribeSubnetsInput{SubnetIds: a.annotations.subnets}
	subnetInfo, err := elb.EC2.DescribeSubnets(descRequest)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2"}).Add(float64(1))
		return nil, err
	}

	vpcID := subnetInfo.Subnets[0].VpcId
	// Target group in VPC for which ALB will route to
	targetParams := &elbv2.CreateTargetGroupInput{
		Name:     aws.String(a.Name()),
		Port:     aws.Int64(int64(a.nodePort)),
		Protocol: aws.String("HTTP"),
		VpcId:    vpcID,
	}

	// Debug logger to introspect CreateTargetGroup request
	glog.Infof("Create TargetGroup request sent:\n%s", targetParams)
	tGroupResp, err := elb.CreateTargetGroup(targetParams)
	if err != nil {
		return nil, err
	}

	return tGroupResp, err
}

// Deletes a TargetGroup in AWS.
func (elb *ELBV2) deleteTargetGroups(a *albIngress) error {
	describeParams := &elbv2.DescribeTargetGroupsInput{
		LoadBalancerArn: a.elbv2.LoadBalancer.LoadBalancerArn,
	}
	glog.Infof("Describe TargetGroup request sent:\n%s", describeParams)
	describeResp, err := elb.DescribeTargetGroups(describeParams)
	if err != nil {
		return err
	}

	if len(describeResp.TargetGroups) == 0 {
		return nil
	}

	for i := range describeResp.TargetGroups {
		deleteParams := &elbv2.DeleteTargetGroupInput{
			TargetGroupArn: describeResp.TargetGroups[i].TargetGroupArn,
		}
		glog.Infof("Delete TargetGroup request sent:\n%s", deleteParams)
		_, err := elb.DeleteTargetGroup(deleteParams)
		if err != nil {
			return err
		}
	}

	return nil
}

// Registers Targets (ec2 instances) to a pre-existing TargetGroup in AWS
func (elb *ELBV2) registerTargets(a *albIngress, tGroupResp *elbv2.CreateTargetGroupOutput) error {
	descRequest := &ec2.DescribeSubnetsInput{SubnetIds: a.annotations.subnets}
	subnetInfo, err := elb.EC2.DescribeSubnets(descRequest)
	// ugly hack to get all instanceIds for a VPC;
	// we'll eventually go through k8s api
	// TODO: Remove all this in favor of introspecting instanceIds from
	//       albIngress struct
	ec2InstanceFilter := &ec2.Filter{
		Name:   aws.String("vpc-id"),
		Values: []*string{subnetInfo.Subnets[0].VpcId},
	}
	descInstParams := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{ec2InstanceFilter},
	}
	instances, err := elb.EC2.DescribeInstances(descInstParams)

	// Instance registration for target group
	targets := []*elbv2.TargetDescription{}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			targets = append(targets, &elbv2.TargetDescription{Id: instance.InstanceId, Port: aws.Int64(int64(a.nodePort))})
		}
	}

	// for Kraig
	targets = []*elbv2.TargetDescription{}
	for _, target := range a.nodeIds {
		targets = append(targets, &elbv2.TargetDescription{Id: aws.String(target), Port: aws.Int64(int64(a.nodePort))})
	}

	registerParams := &elbv2.RegisterTargetsInput{
		TargetGroupArn: tGroupResp.TargetGroups[0].TargetGroupArn,
		Targets:        targets,
	}
	// Debug logger to introspect RegisterTargets request
	glog.Infof("RegisterTargets request sent:\n%s", registerParams)
	_, err = elb.RegisterTargets(registerParams)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2"}).Add(float64(1))
		return err
	}

	return nil
}

// Adds a Listener to an existing ALB in AWS. This Listener maps the ALB to an existing TargetGroup.
func (elb *ELBV2) createListener(a *albIngress, tGroupResp *elbv2.CreateTargetGroupOutput) (*elbv2.CreateListenerOutput, error) {
	listenerParams := &elbv2.CreateListenerInput{
		LoadBalancerArn: elb.LoadBalancer.LoadBalancerArn,
		Protocol:        aws.String("HTTP"),
		Port:            aws.Int64(80),
		DefaultActions: []*elbv2.Action{
			{
				Type:           aws.String("forward"),
				TargetGroupArn: tGroupResp.TargetGroups[0].TargetGroupArn,
			},
		},
	}

	// Debug logger to introspect CreateListener request
	glog.Infof("Create Listener request sent:\n%s", listenerParams)
	listenerResponse, err := elb.CreateListener(listenerParams)
	if err != nil {
		return nil, err
	}
	return listenerResponse, nil
}

// Deletes a Listener from an existing ALB in AWS.
func (elb *ELBV2) deleteListeners() error {
	listenerParams := &elbv2.DescribeListenersInput{
		LoadBalancerArn: elb.LoadBalancer.LoadBalancerArn,
	}

	// Debug logger to introspect DeleteListener request
	glog.Infof("Describe Listeners request sent:\n%s", listenerParams)
	listenerResponse, err := elb.DescribeListeners(listenerParams)
	if err != nil {
		return err
	}

	for _, listener := range listenerResponse.Listeners {
		_, err := elb.deleteListener(listener)
		if err != nil {
			glog.Info("Unable to delete %v: %v", listener.ListenerArn, err)
		}
	}
	return nil
}

// Deletes a Listener from an existing ALB in AWS.
func (elb *ELBV2) deleteListener(listener *elbv2.Listener) (*elbv2.DeleteListenerOutput, error) {
	listenerParams := &elbv2.DeleteListenerInput{
		ListenerArn: listener.ListenerArn,
	}

	// Debug logger to introspect DeleteListener request
	glog.Infof("Delete Listener request sent:\n%s", listenerParams)
	listenerResponse, err := elb.DeleteListener(listenerParams)
	if err != nil {
		return nil, err
	}
	return listenerResponse, nil
}

// Check if an ALB, based on its Name, pre-exists in AWS. Returns true is the ALB exists, returns false if it doesn't
func (elb *ELBV2) albExists(a *albIngress) (bool, error) {
	params := &elbv2.DescribeLoadBalancersInput{
		Names: []*string{aws.String(a.Name())},
	}
	resp, err := elb.ELBV2.DescribeLoadBalancers(params)
	if err != nil && err.(awserr.Error).Code() != "LoadBalancerNotFound" {
		return false, err
	}
	if len(resp.LoadBalancers) > 0 {
		// Store existing ALB for later reference
		elb.LoadBalancer = resp.LoadBalancers[0] // seems fishy
		// ALB *does* exist
		return true, nil
	}
	// ALB does *not* exist
	return false, nil
}

// Returns the ALBs name; maintains consistency amongst areas of code that much resolve this.
// TODO: Find a way to make these unique, easy to find, and under 32chars
func (a *albIngress) Name() string {
	return fmt.Sprintf("%s-%s-%s", a.clusterName, a.namespace, a.serviceName)
}
