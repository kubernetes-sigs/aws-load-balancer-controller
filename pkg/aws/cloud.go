package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/metrics"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/services"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/throttle"
)

type Cloud interface {
	// EC2 provides API to AWS EC2
	EC2() services.EC2

	// ELBV2 provides API to AWS ELBV2
	ELBV2() services.ELBV2

	// Region for the kubernetes cluster
	Region() string
}

// NewCloud constructs new Cloud implementation.
func NewCloud(cfg CloudConfig, metricsRegisterer prometheus.Registerer) (Cloud, error) {
	sess := session.Must(session.NewSession(aws.NewConfig()))
	injectUserAgent(&sess.Handlers)
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
		metadata := services.NewEC2Metadata(sess)
		region, err := metadata.Region()
		if err != nil {
			return nil, errors.Wrap(err, "failed to introspect region from EC2Metadata, specify --aws-region instead if EC2Metadata is unavailable")
		}
		cfg.Region = region
	}

	awsCfg := aws.NewConfig().WithRegion(cfg.Region).WithSTSRegionalEndpoint(endpoints.RegionalSTSEndpoint)
	sess = sess.Copy(awsCfg)
	return &defaultCloud{
		cfg:   cfg,
		ec2:   services.NewEC2(sess),
		elbv2: services.NewELBV2(sess),
	}, nil
}

var _ Cloud = &defaultCloud{}

type defaultCloud struct {
	cfg CloudConfig

	ec2   services.EC2
	elbv2 services.ELBV2
}

func (c *defaultCloud) EC2() services.EC2 {
	return c.ec2
}

func (c *defaultCloud) ELBV2() services.ELBV2 {
	return c.elbv2
}

func (c *defaultCloud) Region() string {
	return c.cfg.Region
}
