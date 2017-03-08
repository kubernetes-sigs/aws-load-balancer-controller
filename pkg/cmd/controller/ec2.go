package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

// EC2 is our extension to AWS's ec2.EC2
type EC2 struct {
	svc ec2iface.EC2API
}

func newEC2(awsconfig *aws.Config) *EC2 {
	awsSession, err := session.NewSession(awsconfig)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "EC2", "request": "NewSession"}).Add(float64(1))
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	elbClient := EC2{
		ec2.New(awsSession),
	}
	return &elbClient
}

func (e *EC2) setVPC(a *albIngress) error {
	subnetInfo, err := e.svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
		SubnetIds: a.annotations.subnets,
	})
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "EC2", "request": "DescribeSubnets"}).Add(float64(1))
		return err
	}

	a.vpcID = *subnetInfo.Subnets[0].VpcId
	return nil
}
