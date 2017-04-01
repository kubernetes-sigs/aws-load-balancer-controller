package ec2

import (
	"fmt"
	"time"

	"github.com/coreos-inc/alb-ingress-controller/pkg/metrics"
	"github.com/coreos-inc/alb-ingress-controller/pkg/config"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/karlseguin/ccache"
)

type EC2 struct {
	svc ec2iface.EC2API
	cache *ccache.Cache
}

func newEC2(awsconfig *aws.Config, config *config.Config, cache *ccache.Cache) *EC2 {
	awsSession, err := session.NewSession(awsconfig)
	if err != nil {
		metrics.AWSErrorCount.With(prometheus.Labels{"service": "EC2", "request": "NewSession"}).Add(float64(1))
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	awsSession.Handlers.Send.PushFront(func(r *request.Request) {
		metrics.AWSRequest.With(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name}).Add(float64(1))
		if config.AWSDebug {
			glog.Infof("Request: %s/%s, Payload: %s", r.ClientInfo.ServiceName, r.Operation, r.Params)
		}
	})

	elbClient := EC2{
		ec2.New(awsSession),
		cache,
	}
	return &elbClient
}

func (e *EC2) getVPCID(subnets []*string) (*string, error) {
	var vpc *string

	if len(subnets) == 0 {
		return nil, fmt.Errorf("Empty subnet list provided to getVPCID")
	}

	key := fmt.Sprintf("%s-vpc", *subnets[0])
	item := e.cache.Get(key)

	if item != nil {
		vpc = item.Value().(*string)
		metrics.AWSCache.With(prometheus.Labels{"cache": "subnets", "action": "hit"}).Add(float64(1))
		return vpc, nil
	}

	subnetInfo, err := e.svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
		SubnetIds: subnets,
	})
	if err != nil {
		metrics.AWSErrorCount.With(prometheus.Labels{"service": "EC2", "request": "DescribeSubnets"}).Add(float64(1))
		return nil, err
	}

	if len(subnetInfo.Subnets) == 0 {
		return nil, fmt.Errorf("DescribeSubnets returned no subnets")
	}

	vpc = subnetInfo.Subnets[0].VpcId
	e.cache.Set(key, vpc, time.Minute*60)

	metrics.AWSCache.With(prometheus.Labels{"cache": "subnets", "action": "miss"}).Add(float64(1))

	return vpc, nil
}
