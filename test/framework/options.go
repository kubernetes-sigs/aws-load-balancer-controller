package framework

import (
	"flag"

	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/controller"
)

var globalOptions Options

func init() {
	globalOptions.BindFlags()
}

// Options are the configuration options.
type Options struct {
	ClusterName string
	AWSRegion   string
	AWSVPCID    string
	HelmChart   string
	KubeConfig  string

	// AWS Load Balancer Controller image. leave empty to use default one from helm chart.
	ControllerImage string

	// Additional parameters for e2e tests
	S3BucketName    string
	CertificateARNs string
	IPFamily        string
}

func (options *Options) BindFlags() {
	flag.StringVar(&options.ClusterName, "cluster-name", "", `Kubernetes cluster name (required)`)
	flag.StringVar(&options.AWSRegion, "aws-region", "", `AWS Region for the kubernetes cluster`)
	flag.StringVar(&options.AWSVPCID, "aws-vpc-id", "", `ID of VPC to create load balancers in`)
	flag.StringVar(&options.HelmChart, "helm-chart", controller.AWSLoadBalancerControllerHelmChart, `Helm chart`)

	flag.StringVar(&options.ControllerImage, "controller-image", "", `AWS Load Balancer Controller image`)

	flag.StringVar(&options.S3BucketName, "s3-bucket-name", "", `S3 bucket to use for testing load balancer access logging feature`)
	flag.StringVar(&options.CertificateARNs, "certificate-arns", "", `Certificate ARNs to use for TLS listeners`)
	flag.StringVar(&options.IPFamily, "ip-family", "IPv4", "the ip family used for the cluster, can be IPv4 or IPv6")
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
