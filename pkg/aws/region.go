package aws

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	epresolver "sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	aws_metrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/aws"
)

// NewCloudForRegion creates a Cloud for the given region and vpcID without using EC2 metadata.
// The base cfg is copied; only Region and VpcID are set. Use this when deploying to a non-default region.
func NewCloudForRegion(cfg CloudConfig, region, vpcID string, clusterName string, metricsCollector *aws_metrics.Collector, logger logr.Logger, lbStabilizationTime time.Duration) (services.Cloud, error) {
	cfgForRegion := cfg
	cfgForRegion.Region = region
	cfgForRegion.VpcID = vpcID
	return NewCloud(cfgForRegion, clusterName, metricsCollector, logger, nil, lbStabilizationTime)
}

// NewEC2ClientForRegion returns an EC2 client configured for the given region.
// Used for VPC resolution (e.g. DescribeVpcs, DescribeSubnets) in that region before creating a full Cloud.
func NewEC2ClientForRegion(cfg CloudConfig, region string, metricsCollector *aws_metrics.Collector, logger logr.Logger) (services.EC2, error) {
	awsClientsProvider, err := newClientsProviderForRegion(cfg, region, metricsCollector)
	if err != nil {
		return nil, err
	}
	return services.NewEC2(awsClientsProvider), nil
}

// NewELBV2ForRegion returns an ELBV2 client configured for the given region.
// Used by webhooks and model builders that need to describe/validate resources in a non-default region.
// This does not require a VPC ID or full Cloud — only the region is needed for API calls like DescribeTargetGroups.
func NewELBV2ForRegion(cfg CloudConfig, region string, metricsCollector *aws_metrics.Collector, logger logr.Logger, lbStabilizationTime time.Duration) (services.ELBV2, error) {
	awsClientsProvider, err := newClientsProviderForRegion(cfg, region, metricsCollector)
	if err != nil {
		return nil, err
	}
	cloud := &regionStubCloud{region: region}
	elbv2 := services.NewELBV2(awsClientsProvider, cloud, lbStabilizationTime)
	cloud.elbv2 = elbv2
	return elbv2, nil
}

func newClientsProviderForRegion(cfg CloudConfig, region string, metricsCollector *aws_metrics.Collector) (provider.AWSClientsProvider, error) {
	cfgForRegion := cfg
	cfgForRegion.Region = region
	endpointsResolver := epresolver.NewResolver(cfgForRegion.AWSEndpoints)
	configGenerator := NewAWSConfigGenerator(cfgForRegion, imds.EndpointModeStateIPv4, metricsCollector)
	awsConfig, err := configGenerator.GenerateAWSConfig()
	if err != nil {
		return nil, err
	}
	return provider.NewDefaultAWSClientsProvider(awsConfig, endpointsResolver)
}

// regionStubCloud is a minimal Cloud implementation used only by NewELBV2ForRegion.
// Only Region() and ELBV2() are meaningful; other methods are unused by the ELBV2 client
// for basic operations like DescribeTargetGroups.
type regionStubCloud struct {
	region string
	elbv2  services.ELBV2
}

func (c *regionStubCloud) Region() string                                { return c.region }
func (c *regionStubCloud) VpcID() string                                 { return "" }
func (c *regionStubCloud) ELBV2() services.ELBV2                         { return c.elbv2 }
func (c *regionStubCloud) EC2() services.EC2                             { return nil }
func (c *regionStubCloud) ACM() services.ACM                             { return nil }
func (c *regionStubCloud) WAFv2() services.WAFv2                         { return nil }
func (c *regionStubCloud) WAFRegional() services.WAFRegional             { return nil }
func (c *regionStubCloud) Shield() services.Shield                       { return nil }
func (c *regionStubCloud) RGT() services.RGT                             { return nil }
func (c *regionStubCloud) GlobalAccelerator() services.GlobalAccelerator { return nil }
func (c *regionStubCloud) GetAssumedRoleELBV2(_ context.Context, _ string, _ string) (services.ELBV2, error) {
	return nil, errors.New("AssumeRole is not supported for cross-region stub cloud; use a full Cloud instead")
}

var _ services.Cloud = &regionStubCloud{}
