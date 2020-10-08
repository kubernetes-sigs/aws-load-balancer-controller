package aws

import (
	"github.com/spf13/pflag"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/throttle"
)

const (
	flagAWSRegion        = "aws-region"
	flagAWSAPIThrottle   = "aws-api-throttle"
	flagAWSVpcID         = "aws-vpc-id"
	flagAWSMaxRetries    = "aws-max-retries"
	defaultVpcID         = ""
	defaultRegion        = ""
	defaultAPIMaxRetries = 10
)

type CloudConfig struct {
	// AWS Region for the kubernetes cluster
	Region string

	// Throttle settings for AWS APIs
	ThrottleConfig *throttle.ServiceOperationsThrottleConfig

	// VPC ID of the Kubernetes cluster
	VpcID string

	// Max retries configuration for AWS APIs
	MaxRetries int
}

func (cfg *CloudConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.Region, flagAWSRegion, defaultRegion, "AWS Region for the kubernetes cluster")
	fs.Var(cfg.ThrottleConfig, flagAWSAPIThrottle, "throttle settings for AWS APIs, format: serviceID1:operationRegex1=rate:burst,serviceID2:operationRegex2=rate:burst")
	fs.StringVar(&cfg.VpcID, flagAWSVpcID, defaultVpcID, "AWS VPC ID for the Kubernetes cluster")
	fs.IntVar(&cfg.MaxRetries, flagAWSMaxRetries, defaultAPIMaxRetries, "Maximum retries for AWS APIs")
}
