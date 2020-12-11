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
	KubeConfig  string

	// AWS Load Balancer Controller image. leave empty to use default one from helm chart.
	ControllerImage string
}

func (options *Options) BindFlags() {
	flag.StringVar(&options.ClusterName, "cluster-name", "", `Kubernetes cluster name (required)`)
	flag.StringVar(&options.AWSRegion, "aws-region", "", `AWS Region for the kubernetes cluster`)
	flag.StringVar(&options.AWSVPCID, "aws-vpc-id", "", `AWS VPC ID for the kubernetes cluster`)

	flag.StringVar(&options.ControllerImage, "controller-image", "", `AWS Load Balancer Controller image`)
}

func (options *Options) Validate() error {
	if err := options.rebindFlags(); err != nil {
		return err
	}
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

func (options *Options) rebindFlags() error {
	// kubeconfig is already defined by controller-runtime. we rebind it to our KubeConfig variable.
	kubeConfigFlag := flag.Lookup("kubeconfig")
	if kubeConfigFlag == nil {
		return errors.Errorf("%s must be set!", "kubeconfig")
	}
	options.KubeConfig = kubeConfigFlag.Value.(flag.Getter).Get().(string)
	return nil
}
