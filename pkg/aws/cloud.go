package aws

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/aws/ratelimit"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	smithymiddleware "github.com/aws/smithy-go/middleware"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/throttle"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/version"

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

const userAgent = "elbv2.k8s.aws"

// NewCloud constructs new Cloud implementation.
func NewCloud(cfg CloudConfig, metricsCollector *aws_metrics.Collector, logger logr.Logger, awsClientsProvider provider.AWSClientsProvider) (services.Cloud, error) {
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
	awsConfig, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(cfg.Region),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.RateLimiter = ratelimit.None
				o.MaxAttempts = cfg.MaxRetries
			})
		}),
		config.WithEC2IMDSEndpointMode(ec2IMDSEndpointMode),
		config.WithAPIOptions([]func(stack *smithymiddleware.Stack) error{
			awsmiddleware.AddUserAgentKeyValue(userAgent, version.GitVersion),
		}),
	)

	if cfg.ThrottleConfig != nil {
		throttler := throttle.NewThrottler(cfg.ThrottleConfig)
		awsConfig.APIOptions = append(awsConfig.APIOptions, func(stack *smithymiddleware.Stack) error {
			return throttle.WithSDKRequestThrottleMiddleware(throttler)(stack)
		})
	}

	if metricsCollector != nil {
		awsConfig.APIOptions = aws_metrics.WithSDKMetricCollector(metricsCollector, awsConfig.APIOptions)
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
		cfg:         cfg,
		ec2:         ec2Service,
		acm:         services.NewACM(awsClientsProvider),
		wafv2:       services.NewWAFv2(awsClientsProvider),
		wafRegional: services.NewWAFRegional(awsClientsProvider, cfg.Region),
		shield:      services.NewShield(awsClientsProvider),
		rgt:         services.NewRGT(awsClientsProvider),

		assumeRoleElbV2:    make(map[string]services.ELBV2),
		awsClientsProvider: awsClientsProvider,
		logger:             logger,
	}

	thisObj.elbv2 = services.NewELBV2(awsClientsProvider, thisObj)

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

	ec2         services.EC2
	elbv2       services.ELBV2
	acm         services.ACM
	wafv2       services.WAFv2
	wafRegional services.WAFRegional
	shield      services.Shield
	rgt         services.RGT

	assumeRoleElbV2    map[string]services.ELBV2
	awsClientsProvider provider.AWSClientsProvider
	logger             logr.Logger
}

// returns ELBV2 client for the given assumeRoleArn, or the default ELBV2 client if assumeRoleArn is empty
func (c *defaultCloud) GetAssumedRoleELBV2(ctx context.Context, assumeRoleArn string, externalId string) services.ELBV2 {

	if assumeRoleArn == "" {
		return c.elbv2
	}

	assumedRoleELBV2, exists := c.assumeRoleElbV2[assumeRoleArn]
	if exists {
		return assumedRoleELBV2
	}
	c.logger.Info("awsCloud", "method", "GetAssumedRoleELBV2", "AssumeRoleArn", assumeRoleArn, "externalId", externalId)

	////////////////
	existingAwsConfig, _ := c.awsClientsProvider.GetAWSConfig(ctx, "GetAWSConfigForIAMRoleImpersonation")

	sourceAccount := sts.NewFromConfig(*existingAwsConfig)
	response, err := sourceAccount.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(assumeRoleArn),
		RoleSessionName: aws.String("aws-load-balancer-controller"),
		ExternalId:      aws.String(externalId),
	})
	if err != nil {
		log.Fatalf("Unable to assume target role, %v. Attempting to use default client", err)
		return c.elbv2
	}
	assumedRoleCreds := response.Credentials
	newCreds := credentials.NewStaticCredentialsProvider(*assumedRoleCreds.AccessKeyId, *assumedRoleCreds.SecretAccessKey, *assumedRoleCreds.SessionToken)
	newAwsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(c.cfg.Region), config.WithCredentialsProvider(newCreds))
	if err != nil {
		log.Fatalf("Unable to load static credentials for service client config, %v. Attempting to use default client", err)
		return c.elbv2
	}

	existingAwsConfig.Credentials = newAwsConfig.Credentials //  response.Credentials

	// // var assumedRoleCreds *stsTypes.Credentials = response.Credentials

	// // Create config with target service client, using assumed role
	// cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(*assumedRoleCreds.AccessKeyId, *assumedRoleCreds.SecretAccessKey, *assumedRoleCreds.SessionToken)))
	// if err != nil {
	// 	log.Fatalf("unable to load static credentials for service client config, %v", err)
	// }

	// ////////////////
	// appCreds := stscreds.NewAssumeRoleProvider(client, assumeRoleArn)
	// value, err := appCreds.Retrieve(context.TODO())
	// if err != nil {
	// 	// handle error
	// }
	// /////////

	// ///////////// OLD
	// creds := stscreds.NewCredentials(c.session, assumeRoleArn, func(p *stscreds.AssumeRoleProvider) {
	// 	p.ExternalID = &externalId
	// })
	// //////////////

	// c.awsConfig.Credentials = creds
	// // newObj := services.NewELBV2(c.session, c, c.awsCFG)
	// newObj := services.NewELBV2(*c.awsConfig, c.endpointsResolver, c)

	newObj := services.NewELBV2(c.awsClientsProvider, c)
	c.assumeRoleElbV2[assumeRoleArn] = newObj

	return newObj
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

func (c *defaultCloud) Region() string {
	return c.cfg.Region
}

func (c *defaultCloud) VpcID() string {
	return c.cfg.VpcID
}
