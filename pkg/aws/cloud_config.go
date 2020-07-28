package aws

import (
	"github.com/spf13/pflag"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/throttle"
)

const (
	flagAWSRegion      = "aws-region"
	flagAWSAPIThrottle = "aws-api-throttle"
)

type CloudConfig struct {
	// AWS Region for the kubernetes cluster
	Region string

	// Throttle settings for aws APIs
	ThrottleConfig *throttle.ServiceOperationsThrottleConfig
}

func (cfg *CloudConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.Region, flagAWSRegion, "", "AWS Region for the kubernetes cluster")
	fs.Var(cfg.ThrottleConfig, flagAWSAPIThrottle, "throttle settings for AWS APIs, format: serviceID1:operationRegex1=rate:burst,serviceID2:operationRegex2=rate:burst")
}
