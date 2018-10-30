package types

import (
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

type AvailabilityZones []*elbv2.AvailabilityZone

var logger *log.Logger

func init() {
	logger = log.New("util")
}

func DeepEqual(x, y interface{}) bool {
	b := awsutil.DeepEqual(x, y)
	if !b {
		logger.DebugLevelf(3, "DeepEqual(%v, %v) found inequality", log.Prettify(x), log.Prettify(y))
	}
	return b
}

func (az AvailabilityZones) AsSubnets() []*string {
	var out []*string
	for _, a := range az {
		out = append(out, a.SubnetId)
	}
	return out
}
