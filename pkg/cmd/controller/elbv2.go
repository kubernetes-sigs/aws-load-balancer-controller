package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

// ELBV2 is our extension to AWS's elbv2.ELBV2
type ELBV2 struct {
	*elbv2.ELBV2
}

func newELBV2(awsconfig *aws.Config) *ELBV2 {
	awsSession, err := session.NewSession(awsconfig)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "NewSession"}).Add(float64(1))
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	elbClient := ELBV2{
		elbv2.New(awsSession),
	}
	return &elbClient
}

// Handles ALB change events to determine whether the ALB must be created, or altered.
func (elb *ELBV2) alterALB(a *albIngress) error {

	exists, err := elb.albExists(a)
	if err != nil {
		return err
	}

	if exists {
		glog.Infof("Modifying existing ALB %s", a.id)
		elb.modifyALB(a)
	} else {
		glog.Infof("Creating new ALB %s", a.id)
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

	glog.Infof("Deleting ALB %v", a.id)
	err = elb.deleteListeners(a)
	if err != nil {
		glog.Infof("Unable to delete listeners on %s: %s",
			a.loadBalancerArn,
			err)
	}

	err = elb.deleteTargetGroups(a)
	if err != nil {
		glog.Infof("Unable to delete target groups on %s: %s",
			a.loadBalancerArn,
			err)
	}

	deleteParams := &elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: aws.String(a.loadBalancerArn),
	}
	glog.Infof("Delete LB request sent:\n%s", deleteParams)
	_, err = elb.DeleteLoadBalancer(deleteParams)
	if err != nil {
		return err
	}

	return nil
}

// Modifies the attributes of an existing ALB.
func (elb *ELBV2) modifyALB(a *albIngress) error {

	attr := []*elbv2.LoadBalancerAttribute{}

	if a.loadBalancerScheme != *a.annotations.scheme {
		attr = append(attr, &elbv2.LoadBalancerAttribute{
			Key:   aws.String("scheme"),
			Value: a.annotations.scheme,
		})
	}

	params := &elbv2.ModifyLoadBalancerAttributesInput{
		LoadBalancerArn: aws.String(a.loadBalancerArn),
		Attributes:      attr,
	}

	// Debug logger to introspect CreateLoadBalancer request
	glog.Infof("Modify LB request sent:\n%s", params)
	if !noop {
		_, err := elb.ModifyLoadBalancerAttributes(params)
		if err != nil {
			return err
		}
	}

	return nil
}

// Starts the process of creating a new ALB. If successful, this will create a TargetGroup (TG), register targets in
// the TG, create a ALB, and create a Listener that maps the ALB to the TG in AWS.
func (elb *ELBV2) createALB(a *albIngress) error {
	err := elb.createOrModifyTargetGroup(a)
	if err != nil {
		return err
	}
	err = elb.registerTargets(a)
	if err != nil {
		return err
	}

	a.annotations.tags = append(a.annotations.tags, &elbv2.Tag{
		Key:   aws.String("Namespace"),
		Value: aws.String(a.namespace),
	})
	a.annotations.tags = append(a.annotations.tags, &elbv2.Tag{
		Key:   aws.String("Service"),
		Value: aws.String(a.serviceName),
	})

	createLoadBalancerInput := &elbv2.CreateLoadBalancerInput{
		Name:           aws.String(a.id),
		Subnets:        a.annotations.subnets,
		Scheme:         a.annotations.scheme,
		Tags:           a.annotations.tags,
		SecurityGroups: a.annotations.securityGroups,
	}

	// Debug logger to introspect CreateLoadBalancer request
	glog.Infof("Create LB request sent:\n%s", createLoadBalancerInput)
	createLoadBalancerOutput, err := elb.createLoadBalancer(createLoadBalancerInput)
	if err != nil {
		return err
	}

	a.setLoadBalancer(createLoadBalancerOutput.LoadBalancers[0])

	err = elb.createListener(a)
	if err != nil {
		return err
	}

	glog.Infof("ALB %s finished creation", a.loadBalancerArn)
	return nil
}

func (elb *ELBV2) createLoadBalancer(createLoadBalancerInput *elbv2.CreateLoadBalancerInput) (*elbv2.CreateLoadBalancerOutput, error) {
	if !noop {
		createLoadBalancerOutput, err := elb.CreateLoadBalancer(createLoadBalancerInput)

		if err != nil {
			AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateLoadBalancer"}).Add(float64(1))
			return nil, err
		}
		return createLoadBalancerOutput, nil
	}

	return &elbv2.CreateLoadBalancerOutput{
		LoadBalancers: []*elbv2.LoadBalancer{
			&elbv2.LoadBalancer{
				LoadBalancerArn:       aws.String("mock/arn"),
				DNSName:               aws.String("loadbalancerdnsname"),
				Scheme:                aws.String("loadbalancerscheme"),
				CanonicalHostedZoneId: aws.String("loadbalancerzoneid"),
			},
		},
	}, nil
}

// Check if an ALB, based on its Name, pre-exists in AWS. Returns true is the ALB exists, returns false if it doesn't
func (elb *ELBV2) albExists(a *albIngress) (bool, error) {
	params := &elbv2.DescribeLoadBalancersInput{
		Names: []*string{aws.String(a.id)},
	}
	resp, err := elb.DescribeLoadBalancers(params)
	if err != nil && err.(awserr.Error).Code() != "LoadBalancerNotFound" {
		return false, err
	}
	if len(resp.LoadBalancers) > 0 {
		// Store existing ALB for later reference
		a.setLoadBalancer(resp.LoadBalancers[0])
		// ALB *does* exist
		return true, nil
	}
	// ALB does *not* exist
	return false, nil
}
