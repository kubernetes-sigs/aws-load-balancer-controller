package endpoints

import "github.com/aws/aws-sdk-go-v2/aws"

func NewResolver(configuration map[string]string) *Resolver {
	return &Resolver{
		configuration: configuration,
	}
}

// Resolver is an AWS endpoints.Resolver that allows to customize AWS API endpoints.
// It can be configured using the following format "${AWSServiceID}=${URL}"
// e.g. "EC2=https://ec2.domain.com,Elastic Load Balancing v2=https://elbv2.domain.com"
type Resolver struct {
	configuration map[string]string
}

func (c *Resolver) EndpointFor(serviceId string) *string {
	customEndpoint := c.configuration[serviceId]
	if len(customEndpoint) != 0 {
		return aws.String(customEndpoint)
	}
	return nil
}
