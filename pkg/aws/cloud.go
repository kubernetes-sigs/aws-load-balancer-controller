package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/metrics"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/throttle"
)

type Cloud interface {
	// EC2 provides API to AWS EC2
	EC2() services.EC2

	// ELBV2 provides API to AWS ELBV2
	ELBV2() services.ELBV2

	// WAFv2 provides API to AWS WAFv2
	WAFv2() services.WAFv2

	// WAFRegional provides API to AWS WAFRegional
	WAFRegional() services.WAFRegional

	// Shield provides API to AWS Shield
	Shield() services.Shield

	// Region for the kubernetes cluster
	Region() string

	// VPC ID for the the kubernetes cluster
	VpcID() string
}

// NewCloud constructs new Cloud implementation.
func NewCloud(cfg CloudConfig, metricsRegisterer prometheus.Registerer) (Cloud, error) {
	sess := session.Must(session.NewSession(aws.NewConfig()))
	injectUserAgent(&sess.Handlers)
	metadata := services.NewEC2Metadata(sess)
	if cfg.ThrottleConfig != nil {
		throttler := throttle.NewThrottler(cfg.ThrottleConfig)
		throttler.InjectHandlers(&sess.Handlers)
	}
	if metricsRegisterer != nil {
		metricsCollector, err := metrics.NewCollector(metricsRegisterer)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to initialize sdk metrics collector")
		}
		metricsCollector.InjectHandlers(&sess.Handlers)
	}

	if len(cfg.Region) == 0 {
		region, err := metadata.Region()
		if err != nil {
			return nil, errors.Wrap(err, "failed to introspect region from EC2Metadata, specify --aws-region instead if EC2Metadata is unavailable")
		}
		cfg.Region = region
	}

	if len(cfg.VpcID) == 0 {
		vpcId, err := metadata.VpcID()
		if err != nil {
			return nil, errors.Wrap(err, "failed to introspect vpcID from EC2Metadata, specify --aws-vpc-id instead if EC2Metadata is unavailable")
		}
		cfg.VpcID = vpcId
	}

	awsCfg := aws.NewConfig().WithRegion(cfg.Region).WithSTSRegionalEndpoint(endpoints.RegionalSTSEndpoint)
	sess = sess.Copy(awsCfg)
	return &defaultCloud{
		cfg:         cfg,
		ec2:         services.NewEC2(sess),
		elbv2:       services.NewELBV2(sess),
		wafv2:       services.NewWAFv2(sess),
		wafRegional: services.NewWAFRegional(sess, cfg.Region),
		shield:      services.NewShield(sess),
	}, nil
}

var _ Cloud = &defaultCloud{}

type defaultCloud struct {
	cfg CloudConfig

	ec2   services.EC2
	elbv2 services.ELBV2

	wafv2       services.WAFv2
	wafRegional services.WAFRegional
	shield      services.Shield
}

func (c *defaultCloud) EC2() services.EC2 {
	return c.ec2
}

func (c *defaultCloud) ELBV2() services.ELBV2 {
	return c.elbv2
}

func (c *defaultCloud) WAFv2() services.WAFv2 {
	return c.wafv2
}

func (c *defaultCloud) WAFRegional() services.WAFRegional {
	return c.wafRegional
}

func (c *defaultCloud) Shield() services.Shield {
	return c.shield
}

func (c *defaultCloud) Region() string {
	return c.cfg.Region
}

func (c *defaultCloud) VpcID() string {
	return c.cfg.VpcID
}
