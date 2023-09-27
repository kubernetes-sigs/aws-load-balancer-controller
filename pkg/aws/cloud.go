package aws

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	amerrors "k8s.io/apimachinery/pkg/util/errors"
	epresolver "sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/metrics"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/throttle"
)

type Cloud interface {
	// EC2 provides API to AWS EC2
	EC2() services.EC2

	// ELBV2 provides API to AWS ELBV2
	ELBV2() services.ELBV2

	// ACM provides API to AWS ACM
	ACM() services.ACM

	// WAFv2 provides API to AWS WAFv2
	WAFv2() services.WAFv2

	// WAFRegional provides API to AWS WAFRegional
	WAFRegional() services.WAFRegional

	// Shield provides API to AWS Shield
	Shield() services.Shield

	// RGT provides API to AWS RGT
	RGT() services.RGT

	// GA provides API to AWS Global Accelerator
	GA() services.GlobalAccelerator

	// Region for the kubernetes cluster
	Region() string

	// VpcID for the LoadBalancer resources.
	VpcID() string
}

// NewCloud constructs new Cloud implementation.
func NewCloud(cfg CloudConfig, metricsRegisterer prometheus.Registerer) (Cloud, error) {
	hasIPv4 := true
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		hasIPv4 = false
		for _, addr := range addrs {
			str := addr.String()
			if !strings.HasPrefix(str, "127.") && !strings.Contains(str, ":") {
				hasIPv4 = true
				break
			}
		}
	}

	endpointsResolver := epresolver.NewResolver(cfg.AWSEndpoints)
	metadataCFG := aws.NewConfig().WithEndpointResolver(endpointsResolver)
	opts := session.Options{}
	opts.Config.MergeIn(metadataCFG)
	if !hasIPv4 {
		opts.EC2IMDSEndpointMode = endpoints.EC2IMDSEndpointModeStateIPv6
	}

	metadataSess := session.Must(session.NewSessionWithOptions(opts))
	metadata := services.NewEC2Metadata(metadataSess)

	if len(cfg.Region) == 0 {
		region := os.Getenv("AWS_DEFAULT_REGION")
		if region == "" {
			region = os.Getenv("AWS_REGION")
		}

		if region == "" {
			err := (error)(nil)
			region, err = metadata.Region()
			if err != nil {
				return nil, errors.Wrap(err, "failed to introspect region from EC2Metadata, specify --aws-region instead if EC2Metadata is unavailable")
			}
		}
		cfg.Region = region
	}
	awsCFG := aws.NewConfig().WithRegion(cfg.Region).WithSTSRegionalEndpoint(endpoints.RegionalSTSEndpoint).WithMaxRetries(cfg.MaxRetries).WithEndpointResolver(endpointsResolver)
	opts = session.Options{}
	opts.Config.MergeIn(awsCFG)
	if !hasIPv4 {
		opts.EC2IMDSEndpointMode = endpoints.EC2IMDSEndpointModeStateIPv6
	}
	sess := session.Must(session.NewSessionWithOptions(opts))
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

	ec2Service := services.NewEC2(sess)

	if len(cfg.VpcID) == 0 {
		vpcID, err := inferVPCID(metadata, ec2Service)
		if err != nil {
			return nil, errors.Wrap(err, "failed to introspect vpcID from EC2Metadata or Node name, specify --aws-vpc-id instead if EC2Metadata is unavailable")
		}
		cfg.VpcID = vpcID
	}

	return &defaultCloud{
		cfg:         cfg,
		ec2:         ec2Service,
		elbv2:       services.NewELBV2(sess),
		acm:         services.NewACM(sess),
		ga:          services.NewGlobalAccelerator(sess),
		wafv2:       services.NewWAFv2(sess),
		wafRegional: services.NewWAFRegional(sess, cfg.Region),
		shield:      services.NewShield(sess),
		rgt:         services.NewRGT(sess),
	}, nil
}

func inferVPCID(metadata services.EC2Metadata, ec2Service services.EC2) (string, error) {
	var errList []error
	vpcId, err := metadata.VpcID()
	if err == nil {
		return vpcId, nil
	} else {
		errList = append(errList, errors.Wrap(err, "failed to fetch VPC ID from instance metadata"))
	}

	nodeName := os.Getenv("NODENAME")
	if strings.HasPrefix(nodeName, "i-") {
		output, err := ec2Service.DescribeInstances(&ec2.DescribeInstancesInput{
			InstanceIds: []*string{&nodeName},
		})
		if err != nil {
			errList = append(errList, errors.Wrapf(err, "failed to describe instance %q", nodeName))
			return "", amerrors.NewAggregate(errList)
		}
		if len(output.Reservations) != 1 {
			errList = append(errList, fmt.Errorf("found more than one reservation for instance %q", nodeName))
			return "", amerrors.NewAggregate(errList)
		}
		if len(output.Reservations[0].Instances) != 1 {
			errList = append(errList, fmt.Errorf("found more than one instance with instance ID %q", nodeName))
			return "", amerrors.NewAggregate(errList)
		}

		vpcID := output.Reservations[0].Instances[0].VpcId
		if vpcID != nil {
			return *vpcID, nil
		}

	}
	return "", amerrors.NewAggregate(errList)
}

var _ Cloud = &defaultCloud{}

type defaultCloud struct {
	cfg CloudConfig

	ec2   services.EC2
	elbv2 services.ELBV2

	acm         services.ACM
	ga          services.GlobalAccelerator
	wafv2       services.WAFv2
	wafRegional services.WAFRegional
	shield      services.Shield
	rgt         services.RGT
}

func (c *defaultCloud) EC2() services.EC2 {
	return c.ec2
}

func (c *defaultCloud) ELBV2() services.ELBV2 {
	return c.elbv2
}

func (c *defaultCloud) ACM() services.ACM {
	return c.acm
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

func (c *defaultCloud) RGT() services.RGT {
	return c.rgt
}

func (c *defaultCloud) GA() services.GlobalAccelerator {
	return c.ga
}

func (c *defaultCloud) Region() string {
	return c.cfg.Region
}

func (c *defaultCloud) VpcID() string {
	return c.cfg.VpcID
}
