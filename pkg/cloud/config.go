package cloud

import (
	"fmt"
	"github.com/spf13/pflag"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"strconv"
)

const (
	defaultVpcID         = ""
	defaultRegion        = ""
	defaultAPIMaxRetries = 10
	defaultAPIDebug      = false
)

var logger = log.Log.WithName("cloud/config")

type Config struct {
	ClusterName   string
	VpcID         string
	Region        string
	APIMaxRetries int
	APIDebug      bool
}

func (cfg *Config) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.ClusterName, "cluster-name", "", `Kubernetes cluster name (required)`)
	fs.StringVar(&cfg.VpcID, "aws-vpc-id", defaultVpcID,
		`AWS VPC ID for the kubernetes cluster`)
	fs.StringVar(&cfg.Region, "aws-region", defaultRegion,
		`AWS Region for the kubernetes cluster`)
	fs.IntVar(&cfg.APIMaxRetries, "aws-max-retries", defaultAPIMaxRetries,
		`Maximum number of times to retry the AWS API.`)
	fs.BoolVar(&cfg.APIDebug, "aws-api-debug", defaultAPIDebug,
		`Enable debug logging of AWS API`)
}

func (cfg *Config) BindEnv() error {
	if len(cfg.ClusterName) == 0 {
		if s, ok := os.LookupEnv("CLUSTER_NAME"); ok {
			logger.Info("Environment variable configuration is deprecated, switch to the --cluster-name flag.")
			cfg.ClusterName = s
		}
	}

	if len(cfg.VpcID) == 0 {
		if s, ok := os.LookupEnv("AWS_VPC_ID"); ok {
			logger.Info("Environment variable configuration is deprecated, switch to the --aws-vpc-id flag.")
			cfg.VpcID = s
		}
	}

	if len(cfg.Region) == 0 {
		if s, ok := os.LookupEnv("AWS_REGION"); ok {
			logger.Info("Environment variable configuration is deprecated, switch to the --aws-region flag.")
			cfg.Region = s
		}
	}

	if s, ok := os.LookupEnv("AWS_MAX_RETRIES"); ok {
		logger.Info("Environment variable configuration is deprecated, switch to the --aws-max-retries flag.")
		v, err := strconv.ParseInt(s, 0, 32)
		if err != nil {
			return fmt.Errorf("AWS_MAX_RETRIES environment variable must be an integer. Value was: %s", s)
		}
		cfg.APIMaxRetries = int(v)
	}
	return nil
}

func (cfg *Config) Validate() error {
	if len(cfg.ClusterName) == 0 {
		return fmt.Errorf("clusterName must be specified")
	}
	return nil
}
