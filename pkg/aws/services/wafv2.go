package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
)

type WAFv2 interface {
	AssociateWebACLWithContext(context.Context, *wafv2.AssociateWebACLInput) (*wafv2.AssociateWebACLOutput, error)
	DisassociateWebACLWithContext(ctx context.Context, req *wafv2.DisassociateWebACLInput) (*wafv2.DisassociateWebACLOutput, error)
	GetWebACLForResourceWithContext(ctx context.Context, req *wafv2.GetWebACLForResourceInput) (*wafv2.GetWebACLForResourceOutput, error)
}

// NewWAFv2 constructs new WAFv2 implementation.
func NewWAFv2(cfg aws.Config) WAFv2 {
	client := wafv2.NewFromConfig(cfg)
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
