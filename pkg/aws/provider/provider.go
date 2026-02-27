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
)

type AWSClientsProvider interface {
	GetRoute53Client(ctx context.Context, operationName string) (*route53.Client, error)
	GetEC2Client(ctx context.Context, operationName string) (*ec2.Client, error)
	GetELBv2Client(ctx context.Context, operationName string) (*elasticloadbalancingv2.Client, error)
	GetACMClient(ctx context.Context, operationName string) (*acm.Client, error)
	GetWAFv2Client(ctx context.Context, operationName string) (*wafv2.Client, error)
	GetWAFRegionClient(ctx context.Context, operationName string) (*wafregional.Client, error)
	GetShieldClient(ctx context.Context, operationName string) (*shield.Client, error)
	GetRGTClient(ctx context.Context, operationName string) (*resourcegroupstaggingapi.Client, error)
	GetSTSClient(ctx context.Context, operationName string) (*sts.Client, error)
	GetGlobalAcceleratorClient(ctx context.Context, operationName string) (*globalaccelerator.Client, error)
	GenerateNewELBv2Client(cfg aws.Config) *elasticloadbalancingv2.Client
}
