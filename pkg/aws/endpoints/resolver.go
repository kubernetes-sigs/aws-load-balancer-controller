package endpoints

import (
	awsendpoints "github.com/aws/aws-sdk-go/aws/endpoints"
)

func NewResolver(configuration map[string]string) *resolver {
	return &resolver{
		configuration: configuration,
	}
}

var _ awsendpoints.Resolver = &resolver{}

// resolver is an AWS endpoints.Resolver that allows to customize AWS API endpoints.
// It can be configured using the following format "${AWSServiceID}=${URL}"
// e.g. "ec2=https://ec2.domain.com,elasticloadbalancing=https://elbv2.domain.com"
type resolver struct {
	configuration map[string]string
}

func (c *resolver) EndpointFor(service, region string, opts ...func(*awsendpoints.Options)) (awsendpoints.ResolvedEndpoint, error) {
	customEndpoint := c.configuration[service]
	if len(customEndpoint) != 0 {
		return awsendpoints.ResolvedEndpoint{
			URL: customEndpoint,
		}, nil
	}
	return awsendpoints.DefaultResolver().EndpointFor(service, region, opts...)
}
