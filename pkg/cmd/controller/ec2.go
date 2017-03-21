package controller

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
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

	awsSession.Handlers.Send.PushFront(func(r *request.Request) {
		AWSRequest.With(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name}).Add(float64(1))
		if AWSDebug {
			glog.Infof("Request: %s/%s, Payload: %s", r.ClientInfo.ServiceName, r.Operation, r.Params)
		}
	})

	elbClient := EC2{
		ec2.New(awsSession),
	}
	return &elbClient
}

func (e *EC2) getVPCID(subnets []*string) (*string, error) {
	var vpc *string

	if subnets == nil {
		return nil, fmt.Errorf("Empty subnet list provided to getVPCID")
	}

	key := fmt.Sprintf("%s-vpc", *subnets[0])
	item := cache.Get(key)

	if item == nil {
		subnetInfo, err := e.svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
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
		cache.Set(key, vpc, time.Minute*60)

		AWSCache.With(prometheus.Labels{"cache": "subnets", "action": "miss"}).Add(float64(1))
	} else {
		vpc = item.Value().(*string)
		AWSCache.With(prometheus.Labels{"cache": "subnets", "action": "hit"}).Add(float64(1))
	}

	return vpc, nil
}
