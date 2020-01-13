package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
)

const (
	ResourceTypeEnumELBLoadBalancer  = "elasticloadbalancing:loadbalancer"
	ResourceTypeEnumELBTargetGroup   = "elasticloadbalancing:targetgroup"
	ResourceTypeEnumEC2SecurityGroup = "ec2:security-group"
)

type ResourceGroupsTaggingAPIAPI interface {
	// GetResourcesByFilters fetches resources ARNs by tagFilters and 0 or more resourceTypesFilters
	GetResourcesByFilters(tagFilters map[string][]string, resourceTypeFilters ...string) ([]string, error)

	TagResourcesWithContext(context.Context, *resourcegroupstaggingapi.TagResourcesInput) (*resourcegroupstaggingapi.TagResourcesOutput, error)
	UntagResourcesWithContext(context.Context, *resourcegroupstaggingapi.UntagResourcesInput) (*resourcegroupstaggingapi.UntagResourcesOutput, error)
}

func (c *Cloud) TagResourcesWithContext(ctx context.Context, i *resourcegroupstaggingapi.TagResourcesInput) (*resourcegroupstaggingapi.TagResourcesOutput, error) {
	return c.rgt.TagResourcesWithContext(ctx, i)
}
func (c *Cloud) UntagResourcesWithContext(ctx context.Context, i *resourcegroupstaggingapi.UntagResourcesInput) (*resourcegroupstaggingapi.UntagResourcesOutput, error) {
	return c.rgt.UntagResourcesWithContext(ctx, i)
}

func (c *Cloud) GetResourcesByFilters(tagFilters map[string][]string, resourceTypeFilters ...string) ([]string, error) {
	var awsTagFilters []*resourcegroupstaggingapi.TagFilter
	for k, v := range tagFilters {
		awsTagFilters = append(awsTagFilters, &resourcegroupstaggingapi.TagFilter{
			Key:    aws.String(k),
			Values: aws.StringSlice(v),
		})
	}
	req := &resourcegroupstaggingapi.GetResourcesInput{
		ResourceTypeFilters: aws.StringSlice(resourceTypeFilters),
		TagFilters:          awsTagFilters,
	}

	var result []string
	err := c.rgt.GetResourcesPages(req, func(output *resourcegroupstaggingapi.GetResourcesOutput, b bool) bool {
		if output == nil {
			return false
		}
		for _, i := range output.ResourceTagMappingList {
			result = append(result, aws.StringValue(i.ResourceARN))
		}
		return true
	})
	return result, err
}
