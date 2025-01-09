package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/wafregional"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
)

type WAFRegional interface {
	Available() bool

	AssociateWebACLWithContext(ctx context.Context, input *wafregional.AssociateWebACLInput) (*wafregional.AssociateWebACLOutput, error)
	DisassociateWebACLWithContext(ctx context.Context, input *wafregional.DisassociateWebACLInput) (*wafregional.DisassociateWebACLOutput, error)
	GetWebACLForResourceWithContext(ctx context.Context, input *wafregional.GetWebACLForResourceInput) (*wafregional.GetWebACLForResourceOutput, error)
}

// NewWAFRegional constructs new WAFRegional implementation.
func NewWAFRegional(awsClientsProvider provider.AWSClientsProvider, region string) WAFRegional {
	return &wafRegionalClient{
		awsClientsProvider: awsClientsProvider,
		region:             region,
	}
}

// default implementation for WAFRegional.
type wafRegionalClient struct {
	awsClientsProvider provider.AWSClientsProvider
	region             string
}

func (c *wafRegionalClient) Available() bool {
	resolver := wafregional.NewDefaultEndpointResolverV2()
	_, err := resolver.ResolveEndpoint(context.Background(), wafregional.EndpointParameters{
		Region: &c.region,
	})
	return err == nil
}

func (c *wafRegionalClient) AssociateWebACLWithContext(ctx context.Context, input *wafregional.AssociateWebACLInput) (*wafregional.AssociateWebACLOutput, error) {
	client, err := c.awsClientsProvider.GetWAFRegionClient(ctx, "AssociateWebACL")
	if err != nil {
		return nil, err
	}
	return client.AssociateWebACL(ctx, input)
}

func (c *wafRegionalClient) DisassociateWebACLWithContext(ctx context.Context, input *wafregional.DisassociateWebACLInput) (*wafregional.DisassociateWebACLOutput, error) {
	client, err := c.awsClientsProvider.GetWAFRegionClient(ctx, "DisassociateWebACL")
	if err != nil {
		return nil, err
	}
	return client.DisassociateWebACL(ctx, input)
}

func (c *wafRegionalClient) GetWebACLForResourceWithContext(ctx context.Context, input *wafregional.GetWebACLForResourceInput) (*wafregional.GetWebACLForResourceOutput, error) {
	client, err := c.awsClientsProvider.GetWAFRegionClient(ctx, "GetWebACLForResource")
	if err != nil {
		return nil, err
	}
	return client.GetWebACLForResource(ctx, input)
}
