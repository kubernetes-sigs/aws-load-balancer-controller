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
	Svc ec2iface.EC2API
}

var ec2Cache = ccache.New(ccache.Configure())

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
	}
	return &elbClient
}

func (e *EC2) GetVPCID(subnets []*string) (*string, error) {
	var vpc *string

	if len(subnets) == 0 {
		return nil, fmt.Errorf("Empty subnet list provided to getVPCID")
	}

	key := fmt.Sprintf("%s-vpc", *subnets[0])
	item := ec2Cache.Get(key)

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
		ec2Cache.Set(key, vpc, time.Minute*60)

		AWSCache.With(prometheus.Labels{"cache": "vpc", "action": "miss"}).Add(float64(1))
	} else {
		vpc = item.Value().(*string)
		AWSCache.With(prometheus.Labels{"cache": "vpc", "action": "hit"}).Add(float64(1))
	}

	return vpc, nil
}
