package controller

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
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

	cfg *ELBV2Configuration
}

type ELBV2Configuration struct {
	subnets        []*string
	scheme         *string
	securityGroups []*string
	tags           []*elbv2.Tag
}

const (
	securityGroupsKey = "ingress.ticketmaster.com/security-groups"
	subnetsKey        = "ingress.ticketmaster.com/subnets"
	schemeKey         = "ingress.ticketmaster.com/scheme"
	tagsKey           = "ingress.ticketmaster.com/tags"
)

func newELBV2(awsconfig *aws.Config) *ELBV2 {
	session, err := session.NewSession(awsconfig)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2"}).Add(float64(1))
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	// Temporary for tests
	// TODO: Auto-resolve
	region := "us-east-1"
	session.Config.Region = &region

	elbv2 := ELBV2{
		elbv2.New(session),
		ec2.New(session),
		nil,
		nil,
	}
	return &elbv2
}

// Handles ALB change events to determine whether the ALB must be created, deleted, or altered.
// TODO: Implement alter and deletion logic
func (elb *ELBV2) alterALB(a *albIngress) error {

	err := elb.configureFromAnnotations(a.annotations)
	if err != nil {
		return err
	}

	err = elb.createALB(a)
	if err != nil {
		return err
	}
	return nil
}

// Starts the process of creating a new ALB. If successful, this will create a TargetGroup (TG), register targets in
// the TG, create a ALB, and create a Listener that maps the ALB to the TG in AWS.
func (elb *ELBV2) createALB(a *albIngress) error {
	albName := fmt.Sprintf("%s-%s", a.clusterName, a.serviceName)

	tGroupResp, err := elb.createTargetGroup(a, &albName)
	if err != nil {
		return err
	}
	err = elb.registerTargets(a, tGroupResp)
	if err != nil {
		return err
	}

	albParams := &elbv2.CreateLoadBalancerInput{
		Name:    &albName,
		Subnets: elb.cfg.subnets,
		Scheme:  elb.cfg.scheme,
		// Tags:           elb.cfg.tags,
		SecurityGroups: elb.cfg.securityGroups,
	}

	// Debug logger to introspect CreateLoadBalancer request
	glog.Infof("Create LB request sent:\n%s", albParams)
	resp, err := elb.CreateLoadBalancer(albParams)

	if err != nil {
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
func (elb *ELBV2) createTargetGroup(a *albIngress, albName *string) (*elbv2.CreateTargetGroupOutput, error) {
	descRequest := &ec2.DescribeSubnetsInput{SubnetIds: elb.cfg.subnets}
	subnetInfo, err := elb.EC2.DescribeSubnets(descRequest)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2"}).Add(float64(1))
		return nil, err
	}

	vpcID := subnetInfo.Subnets[0].VpcId
	// Target group in VPC for which ALB will route to
	targetParams := &elbv2.CreateTargetGroupInput{
		Name:     albName,
		Port:     aws.Int64(int64(a.nodePort)),
		Protocol: aws.String("HTTP"),
		VpcId:    vpcID,
	}

	// Debug logger to introspect CreateTargetGroup request
	glog.Infof("Create LB request sent:\n%s", targetParams)
	tGroupResp, err := elb.CreateTargetGroup(targetParams)
	if err != nil {
		return nil, err
	}

	return tGroupResp, err
}

// Registers Targets (ec2 instances) to a pre-existing TargetGroup in AWS
func (elb *ELBV2) registerTargets(a *albIngress, tGroupResp *elbv2.CreateTargetGroupOutput) error {
	descRequest := &ec2.DescribeSubnetsInput{SubnetIds: elb.cfg.subnets}
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
	glog.Infof("Create LB request sent:\n%s", registerParams)
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
	glog.Infof("Create LB request sent:\n%s", listenerParams)
	listenerResponse, err := elb.CreateListener(listenerParams)
	if err != nil {
		return nil, err
	}
	return listenerResponse, nil
}

func (elb *ELBV2) configureFromAnnotations(annotations map[string]string) error {
	// Verify subnet and ingress scheme keys are present before starting ALB creation.
	switch {
	case annotations[subnetsKey] == "":
		return fmt.Errorf(`Necessary annotations missing. Must include %s`, subnetsKey)
	case annotations[schemeKey] == "":
		return fmt.Errorf(`Necessary annotations missing. Must include %s`, schemeKey)
	}

	cfg := &ELBV2Configuration{
		subnets:        stringToAwsSlice(annotations[subnetsKey]),
		scheme:         aws.String(annotations[schemeKey]),
		securityGroups: stringToAwsSlice(annotations[securityGroupsKey]),
		// tags:           stringToAwsSlice(annotations[tagsKey]),
	}

	elb.cfg = cfg
	return nil
}

func stringToAwsSlice(s string) (out []*string) {
	parts := strings.Split(s, ",")
	for _, part := range parts {
		out = append(out, aws.String(strings.TrimSpace(part)))
	}
	return out
}
