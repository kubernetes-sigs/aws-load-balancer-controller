package services

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi/resourcegroupstaggingapiiface"
)

const (
	ResourceTypeELBTargetGroup  = "elasticloadbalancing:targetgroup"
	ResourceTypeELBLoadBalancer = "elasticloadbalancing:loadbalancer"
)

type RGT interface {
	resourcegroupstaggingapiiface.ResourceGroupsTaggingAPIAPI

	GetResourcesAsList(ctx context.Context, input *resourcegroupstaggingapi.GetResourcesInput) ([]*resourcegroupstaggingapi.ResourceTagMapping, error)
}

// NewRGT constructs new RGT implementation.
func NewRGT(session *session.Session) RGT {
	return &defaultRGT{
		ResourceGroupsTaggingAPIAPI: resourcegroupstaggingapi.New(session),
	}
}

var _ RGT = (*defaultRGT)(nil)

type defaultRGT struct {
	resourcegroupstaggingapiiface.ResourceGroupsTaggingAPIAPI
}

func (c *defaultRGT) GetResourcesAsList(ctx context.Context, input *resourcegroupstaggingapi.GetResourcesInput) ([]*resourcegroupstaggingapi.ResourceTagMapping, error) {
	var result []*resourcegroupstaggingapi.ResourceTagMapping
	if err := c.GetResourcesPagesWithContext(ctx, input, func(output *resourcegroupstaggingapi.GetResourcesOutput, _ bool) bool {
		for _, i := range output.ResourceTagMappingList {
			result = append(result, i)
		}
		return true
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func ParseRGTTags(tags []*resourcegroupstaggingapi.Tag) map[string]string {
	result := make(map[string]string, len(tags))
	for _, tag := range tags {
		result[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
	}
	return result
}
