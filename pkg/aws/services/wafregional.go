package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/wafregional"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
)

type WAFRegional interface {
	Available() bool

	AssociateWebACLWithContext(ctx context.Context, input *wafregional.AssociateWebACLInput) (*wafregional.AssociateWebACLOutput, error)
	DisassociateWebACLWithContext(ctx context.Context, input *wafregional.DisassociateWebACLInput) (*wafregional.DisassociateWebACLOutput, error)
	GetWebACLForResourceWithContext(ctx context.Context, input *wafregional.GetWebACLForResourceInput) (*wafregional.GetWebACLForResourceOutput, error)
}

// NewWAFRegional constructs new WAFRegional implementation.
func NewWAFRegional(cfg aws.Config, endpointsResolver *endpoints.Resolver, region string) WAFRegional {
	customEndpoint := endpointsResolver.EndpointFor(wafregional.ServiceID)
	return &wafRegionalClient{
		wafRegionalClient: wafregional.NewFromConfig(cfg, func(o *wafregional.Options) {
			o.Region = region
			o.BaseEndpoint = customEndpoint
		}),
		region: region,
	}
}

// default implementation for WAFRegional.
type wafRegionalClient struct {
	wafRegionalClient *wafregional.Client
	region            string
}

func (c *wafRegionalClient) Available() bool {
	resolver := wafregional.NewDefaultEndpointResolverV2()
	_, err := resolver.ResolveEndpoint(context.Background(), wafregional.EndpointParameters{
		Region: &c.region,
	})
	return err == nil
}

func (c *wafRegionalClient) AssociateWebACLWithContext(ctx context.Context, input *wafregional.AssociateWebACLInput) (*wafregional.AssociateWebACLOutput, error) {
	return c.wafRegionalClient.AssociateWebACL(ctx, input)
}

func (c *wafRegionalClient) DisassociateWebACLWithContext(ctx context.Context, input *wafregional.DisassociateWebACLInput) (*wafregional.DisassociateWebACLOutput, error) {
	return c.wafRegionalClient.DisassociateWebACL(ctx, input)
}

func (c *wafRegionalClient) GetWebACLForResourceWithContext(ctx context.Context, input *wafregional.GetWebACLForResourceInput) (*wafregional.GetWebACLForResourceOutput, error) {
	return c.wafRegionalClient.GetWebACLForResource(ctx, input)
}
