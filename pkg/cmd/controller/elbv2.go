package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"fmt"
)

// ELBV2 is our extension to AWS's elbv2.ELBV2
type ELBV2 struct {
	*elbv2.ELBV2
	// LB created; otherwise nil
	*elbv2.LoadBalancer
}

func newELBV2(awsconfig *aws.Config) *ELBV2 {
	session, err := session.NewSession(awsconfig)
	if err != nil {
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	// Temporary for tests
	// TODO: Auto-resovle
	region :="us-west-1"
	session.Config.Region = &region

	elbv2 := ELBV2{
		elbv2.New(session),
		nil,
	}
	return &elbv2
}

// initial function to test creation of ALB
// WIP
func (elb *ELBV2) createALB(a *albIngress) error {
	// this should automatically be resolved up stack
	// TODO: Remove once resolving correctly
	a.clusterName = "TEMPCLUSTERNAME"
	albName := fmt.Sprintf("%s-%s", a.clusterName, a.serviceName)
	alb := &elbv2.CreateLoadBalancerInput{
		Name: &albName,
		Subnets: []*string{aws.String("subnet-dc76ecb8"), aws.String("subnet-1510bb4d")},
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