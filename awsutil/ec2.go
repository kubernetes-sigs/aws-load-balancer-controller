package awsutil

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/golang/glog"
	"github.com/karlseguin/ccache"
	"github.com/prometheus/client_golang/prometheus"
)

// EC2 is our extension to AWS's ec2.EC2
type EC2 struct {
	Svc   ec2iface.EC2API
	cache APICache
}

// NewEC2 returns an awsutil EC2 service
func NewEC2(awsconfig *aws.Config) *EC2 {
	awsSession, err := session.NewSession(awsconfig)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "EC2", "request": "NewSession"}).Add(float64(1))
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	awsSession.Handlers.Send.PushFront(func(r *request.Request) {
		AWSRequest.With(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name}).Add(float64(1))
		if AWSDebug {
			glog.Infof("Request: %s/%s, Payload: %s", r.ClientInfo.ServiceName, r.Operation, r.Params)
		}
	})

	elbClient := EC2{
		ec2.New(awsSession),
		APICache{ccache.New(ccache.Configure()), },
	}
	return &elbClient
}

// DescribeSubnets looks up Subnets based on input and returns a list of Subnets.
func (e *EC2) DescribeSubnets(in ec2.DescribeSubnetsInput) ([]*ec2.Subnet, error) {
	o, err := e.Svc.DescribeSubnets(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "EC2", "request": "DescribeSubnets"}).Add(float64(1))
		return nil, err
	}

	return o.Subnets, nil
}

// DescribeSecurityGroups looks up Security Groups based on input and returns a list of Security
// Groups.
func (e *EC2) DescribeSecurityGroups(in ec2.DescribeSecurityGroupsInput) ([]*ec2.SecurityGroup, error) {
	o, err := e.Svc.DescribeSecurityGroups(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "EC2", "request": "DescribeSecurityGroups"}).Add(float64(1))
		return nil, err
	}

	return o.SecurityGroups, nil
}

// GetVPCID retrieves the VPC that the subents passed are contained in.
func (e *EC2) GetVPCID(subnets []*string) (*string, error) {
	var vpc *string

	if len(subnets) == 0 {
		return nil, fmt.Errorf("Empty subnet list provided to getVPCID")
	}

	key := fmt.Sprintf("%s-vpc", *subnets[0])
	item := e.cache.Get(key)

	if item == nil {
		subnetInfo, err := e.Svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
			SubnetIds: subnets,
		})
		if err != nil {
			AWSErrorCount.With(prometheus.Labels{"service": "EC2", "request": "DescribeSubnets"}).Add(float64(1))
			return nil, err
		}

		if len(subnetInfo.Subnets) == 0 {
			return nil, fmt.Errorf("DescribeSubnets returned no subnets")
		}

		vpc = subnetInfo.Subnets[0].VpcId
		e.cache.Set(key, vpc, time.Minute*60)

		AWSCache.With(prometheus.Labels{"cache": "vpc", "action": "miss"}).Add(float64(1))
	} else {
		vpc = item.Value().(*string)
		AWSCache.With(prometheus.Labels{"cache": "vpc", "action": "hit"}).Add(float64(1))
	}

	return vpc, nil
}
