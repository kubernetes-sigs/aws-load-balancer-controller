package aws

import (
	"fmt"
	"os"
	"strconv"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
)

const (
	defaultVpcID         = ""
	defaultRegion        = ""
	defaultAPIMaxRetries = 10
	defaultAPIDebug      = false
)

// configuration for cloud
type CloudConfig struct {
	VpcID  string
	Region string

	APIMaxRetries int
	APIDebug      bool
}

func (cfg *CloudConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.VpcID, "aws-vpc-id", defaultVpcID,
		`AWS VPC ID for the kubernetes cluster`)
	fs.StringVar(&cfg.Region, "aws-region", defaultRegion,
		`AWS Region for the kubernetes cluster`)
	fs.IntVar(&cfg.APIMaxRetries, "aws-max-retries", defaultAPIMaxRetries,
		`Maximum number of times to retry the AWS API.`)
	fs.BoolVar(&cfg.APIDebug, "aws-api-debug", defaultAPIDebug,
		`Enable debug logging of AWS API`)
}

func (cfg *CloudConfig) BindEnv() error {
	if s, ok := os.LookupEnv("AWS_VPC_ID"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --aws-vpc-id flag.")
		cfg.VpcID = s
	}

	if s, ok := os.LookupEnv("AWS_REGION"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --aws-region flag.")
		cfg.Region = s
	}

	if s, ok := os.LookupEnv("AWS_MAX_RETRIES"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --aws-max-retries flag.")
		v, err := strconv.ParseInt(s, 0, 32)
		if err != nil {
			return fmt.Errorf("AWS_MAX_RETRIES environment variable must be an integer. Value was: %s", s)
		}
		cfg.APIMaxRetries = int(v)
	}
	return nil
}
