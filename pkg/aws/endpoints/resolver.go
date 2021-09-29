package endpoints

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	awsendpoints "github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
)

var _ pflag.Value = &AWSEndpointResolver{}

// AWSEndpointResolver is an AWS endpoints.Resolver that allows to customize AWS API endpoints.
// It can be configured using the following format "${AWSServiceID}=${URL}"
// e.g. "ec2=https://ec2.domain.com,elasticloadbalancing=https://elbv2.domain.com"
type AWSEndpointResolver struct {
	configuration map[string]string
}

func (c *AWSEndpointResolver) String() string {
	if c == nil {
		return ""
	}

	var configs []string
	var serviceIDs []string
	for serviceID := range c.configuration {
		serviceIDs = append(serviceIDs, serviceID)
	}
	sort.Strings(serviceIDs)
	for _, serviceID := range serviceIDs {
		configs = append(configs, fmt.Sprintf("%s=%s", serviceID, c.configuration[serviceID]))
	}
	return strings.Join(configs, ",")
}

func (c *AWSEndpointResolver) Set(val string) error {
	configurationOverride := make(map[string]string)

	if val != "" {
		configPairs := strings.Split(val, ",")
		for _, pair := range configPairs {
			kv := strings.Split(pair, "=")
			if len(kv) != 2 {
				return errors.Errorf("%s must be formatted as serviceID=URL", pair)
			}
			serviceID := kv[0]
			urlStr := kv[1]
			url, err := url.Parse(urlStr)
			if err != nil {
				return errors.Errorf("%s must be a valid url", urlStr)
			}
			if !url.IsAbs() {
				return errors.Errorf("%s must be an absolute url", urlStr)
			}
			configurationOverride[serviceID] = url.String()
		}
	}

	if c.configuration == nil {
		c.configuration = make(map[string]string)
	}
	for k, v := range configurationOverride {
		c.configuration[k] = v
	}
	return nil
}

func (c *AWSEndpointResolver) Type() string {
	return "awsEndpointResolver"
}

func (c *AWSEndpointResolver) EndpointFor(service, region string, opts ...func(*awsendpoints.Options)) (awsendpoints.ResolvedEndpoint, error) {
	customEndpoint := c.configuration[service]
	if len(customEndpoint) != 0 {
		return awsendpoints.ResolvedEndpoint{
			URL: customEndpoint,
		}, nil
	}
	return awsendpoints.DefaultResolver().EndpointFor(service, region, opts...)
}