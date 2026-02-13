package framework

import (
	"flag"
	"time"

	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/controller"
)

const (
	IPv4 = "IPv4"
	IPv6 = "IPv6"
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

	// The timeout to consider failed DNS propagation.
	DNSTimeout time.Duration

	// AWS Load Balancer Controller image. leave empty to use default one from helm chart.
	ControllerImage string

	// Additional parameters for e2e tests
	S3BucketName        string
	CertificateARNs     string
	IPFamily            string
	TestImageRegistry   string
	EnableGatewayTests  bool
	EnableAGATests      bool
	EnableCertMgmtTests bool

	// ACM Certificate Management configuration for e2e test
	Route53ValidationDomain string
	PCAARN                  string

	// Cognito configuration for e2e tests
	CognitoUserPoolArn      string
	CognitoUserPoolClientId string
	CognitoUserPoolDomain   string

	// mTLS configuration for e2e tests
	TrustStoreARN  string
	ClientCertPath string
	ClientKeyPath  string
}

func (options *Options) BindFlags() {
	flag.StringVar(&options.ClusterName, "cluster-name", "", `Kubernetes cluster name (required)`)
	flag.StringVar(&options.AWSRegion, "aws-region", "", `AWS Region for the kubernetes cluster`)
	flag.StringVar(&options.AWSVPCID, "aws-vpc-id", "", `ID of VPC to create load balancers in`)
	flag.StringVar(&options.HelmChart, "helm-chart", controller.AWSLoadBalancerControllerHelmChart, `Helm chart`)
	flag.DurationVar(&options.DNSTimeout, "dns-propagation", 10*time.Minute, `The timeout we expect the DNS records of ALB to be propagated.`)

	flag.StringVar(&options.ControllerImage, "controller-image", "", `AWS Load Balancer Controller image`)

	flag.StringVar(&options.S3BucketName, "s3-bucket-name", "", `S3 bucket to use for testing load balancer access logging feature`)
	flag.StringVar(&options.CertificateARNs, "certificate-arns", "", `Certificate ARNs to use for TLS listeners`)
	flag.StringVar(&options.IPFamily, "ip-family", IPv4, "the ip family used for the cluster, can be IPv4 or IPv6")
	flag.StringVar(&options.TestImageRegistry, "test-image-registry", "617930562442.dkr.ecr.us-west-2.amazonaws.com", "the aws registry in test-infra-* accounts where e2e test images are stored")

	flag.BoolVar(&options.EnableGatewayTests, "enable-gateway-tests", false, "enables gateway tests")
	flag.BoolVar(&options.EnableAGATests, "enable-aga-tests", false, "enables AWS Global Accelerator tests")
	flag.BoolVar(&options.EnableCertMgmtTests, "enable-cert-tests", false, "enables AWS ACM Certificate Management tests")

	flag.StringVar(&options.Route53ValidationDomain, "route53-validation-domain", "", `Route53 domain that can be used for requesting amazon_issued certificates`)
	flag.StringVar(&options.PCAARN, "pca-arn", "", `PCA ARN of CA that can be used to request private certificates`)

	flag.StringVar(&options.CognitoUserPoolArn, "cognito-user-pool-arn", "", `Cognito User Pool ARN for authenticate-cognito tests`)
	flag.StringVar(&options.CognitoUserPoolClientId, "cognito-user-pool-client-id", "", `Cognito User Pool Client ID for authenticate-cognito tests`)
	flag.StringVar(&options.CognitoUserPoolDomain, "cognito-user-pool-domain", "", `Cognito User Pool Domain for authenticate-cognito tests`)

	flag.StringVar(&options.TrustStoreARN, "trust-store-arn", "", `Trust Store ARN for mTLS tests`)
	flag.StringVar(&options.ClientCertPath, "client-cert-path", "", `Path to client certificate for mTLS tests`)
	flag.StringVar(&options.ClientKeyPath, "client-key-path", "", `Path to client key for mTLS tests`)
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
	if len(options.TestImageRegistry) == 0 {
		return errors.Errorf("%s must be set!", "test-image-registry")
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
