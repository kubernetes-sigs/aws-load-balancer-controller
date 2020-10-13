package framework

import (
	"flag"
	"github.com/pkg/errors"
)

var globalOptions Options

func init() {
	globalOptions.BindFlags()
}

// configuration options
type Options struct {
	ClusterName string
	AWSRegion   string
	AWSVPCID    string
}

func (options *Options) BindFlags() {
	flag.StringVar(&options.ClusterName, "cluster-name", "", `Kubernetes cluster name (required)`)
	flag.StringVar(&options.AWSRegion, "aws-region", "", `AWS Region for the kubernetes cluster`)
	flag.StringVar(&options.AWSVPCID, "aws-vpc-id", "", `AWS VPC ID for the kubernetes cluster`)
}

func (options *Options) Validate() error {
	if len(options.ClusterName) == 0 {
		return errors.Errorf("%s must be set!", "cluster-name")
	}
	if len(options.AWSRegion) == 0 {
		return errors.Errorf("%s must be set!", "aws-region")
	}
	if len(options.AWSVPCID) == 0 {
		return errors.Errorf("%s must be set!", "aws-vpc-id")
	}
	return nil
}
