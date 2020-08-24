package framework

import (
	"flag"
	"github.com/pkg/errors"
)

var GlobalOptions Options

func init() {
	GlobalOptions.BindFlags()
}

type Options struct {
	KubeConfig  string
	ClusterName string
	AwsRegion   string
}

func (options *Options) BindFlags() {
	flag.StringVar(&options.KubeConfig, "cluster-kubeconfig", "", "Path to kubeconfig containing embedded authinfo (required)")
	flag.StringVar(&options.ClusterName, "cluster-name", "", `Kubernetes cluster name (required)`)
	flag.StringVar(&options.AwsRegion, "aws-region", "", `AWS Region for the kubernetes cluster`)
}

func (options *Options) Validate() error {
	if options.KubeConfig == "" {
		return errors.Errorf("kubeconfig must be set!")
	}
	if options.ClusterName == "" {
		return errors.Errorf("cluster-name must be set")
	}
	if options.AwsRegion == "" {
		return errors.Errorf("aws-region must be set")
	}
	return nil
}
