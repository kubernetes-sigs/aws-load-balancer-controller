package provider

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/shield"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/wafregional"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
)

type defaultAWSClientsProvider struct {
	ec2Client               *ec2.Client
	elbv2Client             *elasticloadbalancingv2.Client
	acmClient               *acm.Client
	wafv2Client             *wafv2.Client
	wafRegionClient         *wafregional.Client
	shieldClient            *shield.Client
	rgtClient               *resourcegroupstaggingapi.Client
	stsClient               *sts.Client
	route53Client           *route53.Client
	globalAcceleratorClient *globalaccelerator.Client

	// used for dynamic creation of ELBv2 client
	elbv2CustomEndpoint *string
}

func NewDefaultAWSClientsProvider(cfg aws.Config, endpointsResolver *endpoints.Resolver) (AWSClientsProvider, error) {
	ec2CustomEndpoint := endpointsResolver.EndpointFor(ec2.ServiceID)
	elbv2CustomEndpoint := endpointsResolver.EndpointFor(elasticloadbalancingv2.ServiceID)
	acmCustomEndpoint := endpointsResolver.EndpointFor(acm.ServiceID)
	wafv2CustomEndpoint := endpointsResolver.EndpointFor(wafv2.ServiceID)
	wafregionalCustomEndpoint := endpointsResolver.EndpointFor(wafregional.ServiceID)
	shieldCustomEndpoint := endpointsResolver.EndpointFor(shield.ServiceID)
	rgtCustomEndpoint := endpointsResolver.EndpointFor(resourcegroupstaggingapi.ServiceID)
	stsCustomEndpoint := endpointsResolver.EndpointFor(sts.ServiceID)
	globalAcceleratorCustomEndpoint := endpointsResolver.EndpointFor(globalaccelerator.ServiceID)
	route53CustomEndpoint := endpointsResolver.EndpointFor(route53.ServiceID)

	ec2Client := ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		if ec2CustomEndpoint != nil {
			o.BaseEndpoint = ec2CustomEndpoint
		}
	})

	elbv2Client := generateNewELBv2ClientHelper(cfg, elbv2CustomEndpoint)

	acmClient := acm.NewFromConfig(cfg, func(o *acm.Options) {
		if acmCustomEndpoint != nil {
			o.BaseEndpoint = acmCustomEndpoint
		}
	})
	wafv2Client := wafv2.NewFromConfig(cfg, func(o *wafv2.Options) {
		if wafv2CustomEndpoint != nil {
			o.BaseEndpoint = wafv2CustomEndpoint
		}
	})
	wafregionalClient := wafregional.NewFromConfig(cfg, func(o *wafregional.Options) {
		o.Region = cfg.Region
		o.BaseEndpoint = wafregionalCustomEndpoint
	})
	shieldClient := shield.NewFromConfig(cfg, func(o *shield.Options) {
		o.Region = cfg.Region
		o.BaseEndpoint = shieldCustomEndpoint
	})
	rgtClient := resourcegroupstaggingapi.NewFromConfig(cfg, func(o *resourcegroupstaggingapi.Options) {
		if rgtCustomEndpoint != nil {
			o.BaseEndpoint = rgtCustomEndpoint
		}
	})

	stsClient := sts.NewFromConfig(cfg, func(o *sts.Options) {
		if stsCustomEndpoint != nil {
			o.BaseEndpoint = stsCustomEndpoint
		}
	})

	globalAcceleratorClient := globalaccelerator.NewFromConfig(cfg, func(o *globalaccelerator.Options) {
		o.Region = "us-west-2" // Global Accelerator is a global service that requires us-west-2
		if globalAcceleratorCustomEndpoint != nil {
			o.BaseEndpoint = globalAcceleratorCustomEndpoint
		}
	})

	route53Client := route53.NewFromConfig(cfg, func(o *route53.Options) {
		if route53CustomEndpoint != nil {
			o.BaseEndpoint = route53CustomEndpoint
		}
	})

	return &defaultAWSClientsProvider{
		ec2Client:               ec2Client,
		elbv2Client:             elbv2Client,
		acmClient:               acmClient,
		wafv2Client:             wafv2Client,
		wafRegionClient:         wafregionalClient,
		shieldClient:            shieldClient,
		rgtClient:               rgtClient,
		stsClient:               stsClient,
		route53Client:           route53Client,
		globalAcceleratorClient: globalAcceleratorClient,

		elbv2CustomEndpoint: elbv2CustomEndpoint,
	}, nil
}

// DO NOT REMOVE operationName as parameter, this is on purpose
// to retain the default behavior for OSS controller to use the default client for each aws service
// for our internal controller, we will choose different client based on operationName
func (p *defaultAWSClientsProvider) GetEC2Client(ctx context.Context, operationName string) (*ec2.Client, error) {
	return p.ec2Client, nil
}

func (p *defaultAWSClientsProvider) GetELBv2Client(ctx context.Context, operationName string) (*elasticloadbalancingv2.Client, error) {
	return p.elbv2Client, nil
}

func (p *defaultAWSClientsProvider) GetACMClient(ctx context.Context, operationName string) (*acm.Client, error) {
	return p.acmClient, nil
}

func (p *defaultAWSClientsProvider) GetWAFv2Client(ctx context.Context, operationName string) (*wafv2.Client, error) {
	return p.wafv2Client, nil
}

func (p *defaultAWSClientsProvider) GetWAFRegionClient(ctx context.Context, operationName string) (*wafregional.Client, error) {
	return p.wafRegionClient, nil
}

func (p *defaultAWSClientsProvider) GetShieldClient(ctx context.Context, operationName string) (*shield.Client, error) {
	return p.shieldClient, nil
}

func (p *defaultAWSClientsProvider) GetRGTClient(ctx context.Context, operationName string) (*resourcegroupstaggingapi.Client, error) {
	return p.rgtClient, nil
}

func (p *defaultAWSClientsProvider) GetSTSClient(ctx context.Context, operationName string) (*sts.Client, error) {
	return p.stsClient, nil
}

func (p *defaultAWSClientsProvider) GetRoute53Client(ctx context.Context, operationName string) (*route53.Client, error) {
	return p.route53Client, nil
}

func (p *defaultAWSClientsProvider) GetGlobalAcceleratorClient(ctx context.Context, operationName string) (*globalaccelerator.Client, error) {
	return p.globalAcceleratorClient, nil
}

func (p *defaultAWSClientsProvider) GenerateNewELBv2Client(cfg aws.Config) *elasticloadbalancingv2.Client {
	return generateNewELBv2ClientHelper(cfg, p.elbv2CustomEndpoint)
}

func generateNewELBv2ClientHelper(cfg aws.Config, elbv2CustomEndpoint *string) *elasticloadbalancingv2.Client {
	return elasticloadbalancingv2.NewFromConfig(cfg, func(o *elasticloadbalancingv2.Options) {
		if elbv2CustomEndpoint != nil {
			o.BaseEndpoint = elbv2CustomEndpoint
		}
	})
}
