package types

import (
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

const (
	IdleTimeoutKey           = "idle_timeout.timeout_seconds"
	restrictIngressConfigMap = "alb-ingress-controller-internet-facing-ingresses"
)

type AvailabilityZones []*elbv2.AvailabilityZone

var logger *log.Logger

func init() {
	logger = log.New("util")
}

func DeepEqual(x, y interface{}) bool {
	b := awsutil.DeepEqual(x, y)
	if b == false {
		logger.DebugLevelf(3, "DeepEqual(%v, %v) found inequality", log.Prettify(x), log.Prettify(y))
	}
	return b
}

func SortedMap(m map[string]string) []string {
	var t []string
	for k, v := range m {
		t = append(t, fmt.Sprintf("%v:%v", k, v))
	}
	sort.Strings(t)
	return t
}

func (az AvailabilityZones) AsSubnets() AWSStringSlice {
	var out []*string
	for _, a := range az {
		out = append(out, a.SubnetId)
	}
	return out
}

func (subnets Subnets) AsAvailabilityZones() AvailabilityZones {
	var out []*elbv2.AvailabilityZone
	for _, s := range subnets {
		out = append(out, &elbv2.AvailabilityZone{SubnetId: s, ZoneName: aws.String("")})
	}
	return out
}

func (s Subnets) String() string {
	var out string
	for _, sub := range s {
		out += *sub
	}
	return out
}
