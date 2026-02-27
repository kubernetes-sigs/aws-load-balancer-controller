package aws

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/cache"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	amerrors "k8s.io/apimachinery/pkg/util/errors"
	epresolver "sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	aws_metrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/aws"
)

const (
	cacheTTLBufferTime         = 30 * time.Second
	DefaultLbStabilizationTime = 5 * time.Minute
)

// NewCloud constructs new Cloud implementation.
func NewCloud(cfg CloudConfig, clusterName string, metricsCollector *aws_metrics.Collector, logger logr.Logger, awsClientsProvider provider.AWSClientsProvider, lbStabilizationTime time.Duration) (services.Cloud, error) {
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
	var ec2IMDSEndpointMode imds.EndpointModeState
	if !hasIPv4 {
		ec2IMDSEndpointMode = imds.EndpointModeStateIPv6
	} else {
		ec2IMDSEndpointMode = imds.EndpointModeStateIPv4
	}
	endpointsResolver := epresolver.NewResolver(cfg.AWSEndpoints)
	ec2MetadataCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRetryMaxAttempts(cfg.MaxRetries),
		config.WithEC2IMDSEndpointMode(ec2IMDSEndpointMode),
	)
	ec2Metadata := services.NewEC2Metadata(ec2MetadataCfg, endpointsResolver)

	if len(cfg.Region) == 0 {
		region := os.Getenv("AWS_DEFAULT_REGION")
		if region == "" {
			region = os.Getenv("AWS_REGION")
		}

		if region == "" {
			err := (error)(nil)
			region, err = ec2Metadata.Region()
			if err != nil {
				return nil, errors.Wrap(err, "failed to introspect region from EC2Metadata, specify --aws-region instead if EC2Metadata is unavailable")
			}
		}
		cfg.Region = region
	}

	awsConfigGenerator := NewAWSConfigGenerator(cfg, ec2IMDSEndpointMode, metricsCollector)
	awsConfig, err := awsConfigGenerator.GenerateAWSConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Unable to generate AWS config")
	}

	if awsClientsProvider == nil {
		var err error
		awsClientsProvider, err = provider.NewDefaultAWSClientsProvider(awsConfig, endpointsResolver)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create aws clients provider")
		}
	}
	ec2Service := services.NewEC2(awsClientsProvider)

	vpcID, err := getVpcID(cfg, ec2Service, ec2Metadata, logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get VPC ID")
	}

	cfg.VpcID = vpcID

	thisObj := &defaultCloud{
		cfg:               cfg,
		clusterName:       clusterName,
		ec2:               ec2Service,
		route53:           services.NewRoute53(awsClientsProvider),
		acm:               services.NewACM(awsClientsProvider),
		wafv2:             services.NewWAFv2(awsClientsProvider),
		wafRegional:       services.NewWAFRegional(awsClientsProvider, cfg.Region),
		shield:            services.NewShield(awsClientsProvider),
		rgt:               services.NewRGT(awsClientsProvider),
		globalAccelerator: services.NewGlobalAccelerator(awsClientsProvider),

		awsConfigGenerator: awsConfigGenerator,

		assumeRoleElbV2Cache: cache.NewExpiring(),

		awsClientsProvider: awsClientsProvider,
		logger:             logger,
	}

	thisObj.elbv2 = services.NewELBV2(awsClientsProvider, thisObj, lbStabilizationTime)

	return thisObj, nil
}

func getVpcID(cfg CloudConfig, ec2Service services.EC2, ec2Metadata services.EC2Metadata, logger logr.Logger) (string, error) {
	if cfg.VpcID != "" {
		logger.V(1).Info("vpcid is specified using flag --aws-vpc-id, controller will use the value", "vpc: ", cfg.VpcID)
		return cfg.VpcID, nil
	}

	if cfg.VpcTags != nil {
		return inferVPCIDFromTags(ec2Service, cfg.VpcNameTagKey, cfg.VpcTags[cfg.VpcNameTagKey])
	}

	return inferVPCID(ec2Metadata, ec2Service)
}

func inferVPCID(ec2Metadata services.EC2Metadata, ec2Service services.EC2) (string, error) {
	var errList []error
	vpcId, err := ec2Metadata.VpcID()
	if err == nil {
		return vpcId, nil
	} else {
		errList = append(errList, errors.Wrap(err, "failed to fetch VPC ID from instance metadata"))
	}

	nodeName := os.Getenv("NODENAME")
	if strings.HasPrefix(nodeName, "i-") {
		output, err := ec2Service.DescribeInstancesWithContext(context.Background(), &ec2.DescribeInstancesInput{
			InstanceIds: []string{nodeName},
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

func inferVPCIDFromTags(ec2Service services.EC2, VpcNameTagKey string, VpcNameTagValue string) (string, error) {
	vpcs, err := ec2Service.DescribeVPCsAsList(context.Background(), &ec2.DescribeVpcsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag:" + VpcNameTagKey),
				Values: []string{VpcNameTagValue},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch VPC ID with tag: %w", err)
	}
	if len(vpcs) == 0 {
		return "", fmt.Errorf("no VPC exists with tag: %w", err)
	}
	if len(vpcs) > 1 {
		return "", fmt.Errorf("multiple VPCs exists with tag: %w", err)
	}

	return *vpcs[0].VpcId, nil
}

var _ services.Cloud = &defaultCloud{}

type defaultCloud struct {
	cfg CloudConfig

	route53           services.Route53
	ec2               services.EC2
	elbv2             services.ELBV2
	acm               services.ACM
	wafv2             services.WAFv2
	wafRegional       services.WAFRegional
	shield            services.Shield
	rgt               services.RGT
	globalAccelerator services.GlobalAccelerator

	clusterName string

	awsConfigGenerator AWSConfigGenerator

	// A cache holding elbv2 clients that are assuming a role.
	assumeRoleElbV2Cache *cache.Expiring
	// assumeRoleElbV2CacheMutex protects assumeRoleElbV2Cache
	assumeRoleElbV2CacheMutex sync.RWMutex

	awsClientsProvider provider.AWSClientsProvider
	logger             logr.Logger
}

// GetAssumedRoleELBV2 returns ELBV2 client for the given assumeRoleArn, or the default ELBV2 client if assumeRoleArn is empty
func (c *defaultCloud) GetAssumedRoleELBV2(ctx context.Context, assumeRoleArn string, externalId string) (services.ELBV2, error) {
	if assumeRoleArn == "" {
		return c.elbv2, nil
	}

	c.assumeRoleElbV2CacheMutex.RLock()
	assumedRoleELBV2, exists := c.assumeRoleElbV2Cache.Get(assumeRoleArn)
	c.assumeRoleElbV2CacheMutex.RUnlock()

	if exists {
		return assumedRoleELBV2.(services.ELBV2), nil
	}
	c.logger.Info("Constructing new elbv2 client", "AssumeRoleArn", assumeRoleArn, "externalId", externalId)

	stsClient, err := c.awsClientsProvider.GetSTSClient(ctx, "AssumeRole")
	if err != nil {
		// This should never happen, but let's be forward-looking.
		return nil, err
	}

	response, err := stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(assumeRoleArn),
		RoleSessionName: aws.String(generateAssumeRoleSessionName(c.clusterName)),
		ExternalId:      aws.String(externalId),
	})
	if err != nil {
		c.logger.Error(err, "Unable to assume target role", "roleArn", assumeRoleArn)
		return nil, err
	}
	assumedRoleCreds := response.Credentials
	newCreds := credentials.NewStaticCredentialsProvider(*assumedRoleCreds.AccessKeyId, *assumedRoleCreds.SecretAccessKey, *assumedRoleCreds.SessionToken)
	newAwsConfig, err := c.awsConfigGenerator.GenerateAWSConfig(config.WithCredentialsProvider(newCreds))
	if err != nil {
		c.logger.Error(err, "Create new service client config service client config", "roleArn", assumeRoleArn)
		return nil, err
	}

	cacheTTL := assumedRoleCreds.Expiration.Sub(time.Now())
	elbv2WithAssumedRole := services.NewELBV2FromStaticClient(c.awsClientsProvider.GenerateNewELBv2Client(newAwsConfig), c, DefaultLbStabilizationTime)

	c.assumeRoleElbV2CacheMutex.Lock()
	defer c.assumeRoleElbV2CacheMutex.Unlock()
	c.assumeRoleElbV2Cache.Set(assumeRoleArn, elbv2WithAssumedRole, cacheTTL-cacheTTLBufferTime)
	return elbv2WithAssumedRole, nil
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

func (c *defaultCloud) Route53() services.Route53 {
	return c.route53
}

func (c *defaultCloud) GlobalAccelerator() services.GlobalAccelerator {
	return c.globalAccelerator
}

func (c *defaultCloud) Region() string {
	return c.cfg.Region
}

func (c *defaultCloud) VpcID() string {
	return c.cfg.VpcID
}
