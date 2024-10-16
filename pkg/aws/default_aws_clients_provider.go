package aws

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
)

type defaultAWSClientsProvider struct {
	ec2Client   *ec2.Client
	elbv2Client *elasticloadbalancingv2.Client
}

func NewDefaultAWSClientsProvider(cfg aws.Config, endpointsResolver *endpoints.Resolver) (*defaultAWSClientsProvider, error) {
	customEndpoint := endpointsResolver.EndpointFor(ec2.ServiceID)
	ec2Client := ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		if customEndpoint != nil {
			o.BaseEndpoint = customEndpoint
		}
	})
	return &defaultAWSClientsProvider{
		ec2Client:   ec2Client,
		elbv2Client: nil,
	}, nil
}

func (p *defaultAWSClientsProvider) GetEC2Client(ctx context.Context, operationName string) (*ec2.Client, error) {
	return p.ec2Client, nil
}
