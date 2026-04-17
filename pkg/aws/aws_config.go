package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/ratelimit"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	smithymiddleware "github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/throttle"
	awsmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/version"
)

const (
	userAgent = "elbv2.k8s.aws"
)

func NewAWSConfigGenerator(cfg CloudConfig, ec2IMDSEndpointMode imds.EndpointModeState, metricsCollector *awsmetrics.Collector) AWSConfigGenerator {
	return &awsConfigGeneratorImpl{
		cfg:                 cfg,
		ec2IMDSEndpointMode: ec2IMDSEndpointMode,
		metricsCollector:    metricsCollector,
	}

}

// AWSConfigGenerator is responsible for generating an aws config based on the running environment
type AWSConfigGenerator interface {
	GenerateAWSConfig(optFns ...func(*config.LoadOptions) error) (aws.Config, error)
}

type awsConfigGeneratorImpl struct {
	cfg                 CloudConfig
	ec2IMDSEndpointMode imds.EndpointModeState
	metricsCollector    *awsmetrics.Collector
}

func (gen *awsConfigGeneratorImpl) GenerateAWSConfig(optFns ...func(*config.LoadOptions) error) (aws.Config, error) {

	defaultOpts := []func(*config.LoadOptions) error{
		config.WithRegion(gen.cfg.Region),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.RateLimiter = ratelimit.None
				o.MaxAttempts = gen.cfg.MaxRetries
			})
		}),
		config.WithEC2IMDSEndpointMode(gen.ec2IMDSEndpointMode),
		config.WithAPIOptions([]func(stack *smithymiddleware.Stack) error{
			overrideUserAgentMiddleware(userAgent + "/" + version.GitVersion),
		}),
	}

	defaultOpts = append(defaultOpts, optFns...)

	awsConfig, err := config.LoadDefaultConfig(context.TODO(),
		defaultOpts...,
	)

	if err != nil {
		return aws.Config{}, err
	}

	if gen.cfg.ThrottleConfig != nil {
		throttler := throttle.NewThrottler(gen.cfg.ThrottleConfig)
		awsConfig.APIOptions = append(awsConfig.APIOptions, func(stack *smithymiddleware.Stack) error {
			return throttle.WithSDKRequestThrottleMiddleware(throttler)(stack)
		})
	}

	if gen.metricsCollector != nil {
		awsConfig.APIOptions = awsmetrics.WithSDKMetricCollector(gen.metricsCollector, awsConfig.APIOptions)
	}

	return awsConfig, nil
}

var _ AWSConfigGenerator = &awsConfigGeneratorImpl{}

// overrideUserAgentMiddleware returns a middleware that replaces the User-Agent
// header with the given value, stripping all SDK-generated metadata.
func overrideUserAgentMiddleware(ua string) func(stack *smithymiddleware.Stack) error {
	return func(stack *smithymiddleware.Stack) error {
		return stack.Build.Add(smithymiddleware.BuildMiddlewareFunc("OverrideUserAgent",
			func(ctx context.Context, in smithymiddleware.BuildInput, next smithymiddleware.BuildHandler) (smithymiddleware.BuildOutput, smithymiddleware.Metadata, error) {
				if req, ok := in.Request.(*smithyhttp.Request); ok {
					req.Header.Set("User-Agent", ua)
				}
				return next.HandleBuild(ctx, in)
			}), smithymiddleware.After)
	}
}
