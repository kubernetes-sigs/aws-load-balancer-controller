package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
)

type WAFv2 interface {
	AssociateWebACLWithContext(context.Context, *wafv2.AssociateWebACLInput) (*wafv2.AssociateWebACLOutput, error)
	DisassociateWebACLWithContext(ctx context.Context, req *wafv2.DisassociateWebACLInput) (*wafv2.DisassociateWebACLOutput, error)
	GetWebACLForResourceWithContext(ctx context.Context, req *wafv2.GetWebACLForResourceInput) (*wafv2.GetWebACLForResourceOutput, error)
}

// NewWAFv2 constructs new WAFv2 implementation.
func NewWAFv2(awsClientsProvider provider.AWSClientsProvider) WAFv2 {
	return &wafv2Client{
		awsClientsProvider: awsClientsProvider,
	}
}

type wafv2Client struct {
	awsClientsProvider provider.AWSClientsProvider
}

func (c *wafv2Client) AssociateWebACLWithContext(ctx context.Context, req *wafv2.AssociateWebACLInput) (*wafv2.AssociateWebACLOutput, error) {
	client, err := c.awsClientsProvider.GetWAFv2Client(ctx, "AssociateWebACL")
	if err != nil {
		return nil, err
	}
	return client.AssociateWebACL(ctx, req)
}

func (c *wafv2Client) DisassociateWebACLWithContext(ctx context.Context, req *wafv2.DisassociateWebACLInput) (*wafv2.DisassociateWebACLOutput, error) {
	client, err := c.awsClientsProvider.GetWAFv2Client(ctx, "DisassociateWebACL")
	if err != nil {
		return nil, err
	}
	return client.DisassociateWebACL(ctx, req)
}

func (c *wafv2Client) GetWebACLForResourceWithContext(ctx context.Context, req *wafv2.GetWebACLForResourceInput) (*wafv2.GetWebACLForResourceOutput, error) {
	client, err := c.awsClientsProvider.GetWAFv2Client(ctx, "GetWebACLForResource")
	if err != nil {
		return nil, err
	}
	return client.GetWebACLForResource(ctx, req)
}
