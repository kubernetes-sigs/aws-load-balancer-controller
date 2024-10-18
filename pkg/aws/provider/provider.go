package provider

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/shield"
	"github.com/aws/aws-sdk-go-v2/service/wafregional"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
)

type AWSClientsProvider interface {
	GetEC2Client(ctx context.Context, operationName string) (*ec2.Client, error)
	GetELBv2Client(ctx context.Context, operationName string) (*elasticloadbalancingv2.Client, error)
	GetACMClient(ctx context.Context, operationName string) (*acm.Client, error)
	GetWAFv2Client(ctx context.Context, operationName string) (*wafv2.Client, error)
	GetWAFRegionClient(ctx context.Context, operationName string) (*wafregional.Client, error)
	GetShieldClient(ctx context.Context, operationName string) (*shield.Client, error)
	GetRGTClient(ctx context.Context, operationName string) (*resourcegroupstaggingapi.Client, error)
}
