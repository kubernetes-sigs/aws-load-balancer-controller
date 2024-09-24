package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgttypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
)

const (
	ResourceTypeELBTargetGroup  = "elasticloadbalancing:targetgroup"
	ResourceTypeELBLoadBalancer = "elasticloadbalancing:loadbalancer"
)

type RGT interface {
	GetResourcesAsList(ctx context.Context, input *resourcegroupstaggingapi.GetResourcesInput) ([]rgttypes.ResourceTagMapping, error)
}

// NewRGT constructs new RGT implementation.
func NewRGT(cfg aws.Config, endpointsResolver *endpoints.Resolver) RGT {
	customEndpoint := endpointsResolver.EndpointFor(resourcegroupstaggingapi.ServiceID)
	client := resourcegroupstaggingapi.NewFromConfig(cfg, func(o *resourcegroupstaggingapi.Options) {
		if customEndpoint != nil {
			o.BaseEndpoint = customEndpoint
		}
	})
	return &rgtClient{rgtClient: client}
}

type rgtClient struct {
	rgtClient *resourcegroupstaggingapi.Client
}

func (c *rgtClient) GetResourcesAsList(ctx context.Context, input *resourcegroupstaggingapi.GetResourcesInput) ([]rgttypes.ResourceTagMapping, error) {
	var result []rgttypes.ResourceTagMapping
	paginator := resourcegroupstaggingapi.NewGetResourcesPaginator(c.rgtClient, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.ResourceTagMappingList...)
	}
	return result, nil
}

func ParseRGTTags(tags []rgttypes.Tag) map[string]string {
	result := make(map[string]string, len(tags))
	for _, tag := range tags {
		result[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return result
}
