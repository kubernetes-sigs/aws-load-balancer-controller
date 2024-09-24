package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
)

type WAFv2 interface {
	AssociateWebACLWithContext(context.Context, *wafv2.AssociateWebACLInput) (*wafv2.AssociateWebACLOutput, error)
	DisassociateWebACLWithContext(ctx context.Context, req *wafv2.DisassociateWebACLInput) (*wafv2.DisassociateWebACLOutput, error)
	GetWebACLForResourceWithContext(ctx context.Context, req *wafv2.GetWebACLForResourceInput) (*wafv2.GetWebACLForResourceOutput, error)
}

// NewWAFv2 constructs new WAFv2 implementation.
func NewWAFv2(cfg aws.Config, endpointsResolver *endpoints.Resolver) WAFv2 {
	customEndpoint := endpointsResolver.EndpointFor(wafv2.ServiceID)
	client := wafv2.NewFromConfig(cfg, func(o *wafv2.Options) {
		if customEndpoint != nil {
			o.BaseEndpoint = customEndpoint
		}
	})
	return &wafv2Client{wafv2Client: client}
}

type wafv2Client struct {
	wafv2Client *wafv2.Client
}

func (c *wafv2Client) AssociateWebACLWithContext(ctx context.Context, req *wafv2.AssociateWebACLInput) (*wafv2.AssociateWebACLOutput, error) {
	return c.wafv2Client.AssociateWebACL(ctx, req)
}

func (c *wafv2Client) DisassociateWebACLWithContext(ctx context.Context, req *wafv2.DisassociateWebACLInput) (*wafv2.DisassociateWebACLOutput, error) {
	return c.wafv2Client.DisassociateWebACL(ctx, req)
}

func (c *wafv2Client) GetWebACLForResourceWithContext(ctx context.Context, req *wafv2.GetWebACLForResourceInput) (*wafv2.GetWebACLForResourceOutput, error) {
	return c.wafv2Client.GetWebACLForResource(ctx, req)
}
