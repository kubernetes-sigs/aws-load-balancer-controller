package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"

	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

const (
	ResourceTypeEnumELBLoadBalancer  = "elasticloadbalancing:loadbalancer"
	ResourceTypeEnumELBTargetGroup   = "elasticloadbalancing:targetgroup"
	ResourceTypeEnumEC2SecurityGroup = "ec2:security-group"
)

type ResourceGroupsTaggingAPIAPI interface {
	GetClusterSubnets() (map[string]util.EC2Tags, error)

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

// GetClusterSubnets looks up all subnets in AWS that are tagged for the cluster.
func (c *Cloud) GetClusterSubnets() (map[string]util.EC2Tags, error) {
	subnets := make(map[string]util.EC2Tags)

	paramSets := []*resourcegroupstaggingapi.GetResourcesInput{
		{
			ResourcesPerPage: aws.Int64(50),
			ResourceTypeFilters: []*string{
				aws.String("ec2"),
			},
			TagFilters: []*resourcegroupstaggingapi.TagFilter{
				{
					Key:    aws.String("kubernetes.io/role/internal-elb"),
					Values: []*string{aws.String(""), aws.String("1")},
				},
				{
					Key:    aws.String("kubernetes.io/cluster/" + c.clusterName),
					Values: []*string{aws.String("owned"), aws.String("shared")},
				},
			},
		},
		{
			ResourcesPerPage: aws.Int64(50),
			ResourceTypeFilters: []*string{
				aws.String("ec2"),
			},
			TagFilters: []*resourcegroupstaggingapi.TagFilter{
				{
					Key:    aws.String("kubernetes.io/role/elb"),
					Values: []*string{aws.String(""), aws.String("1")},
				},
				{
					Key:    aws.String("kubernetes.io/cluster/" + c.clusterName),
					Values: []*string{aws.String("owned"), aws.String("shared")},
				},
			},
		},
	}

	for _, paramSet := range paramSets {
		err := c.rgt.GetResourcesPages(paramSet, func(page *resourcegroupstaggingapi.GetResourcesOutput, lastPage bool) bool {
			if page == nil {
				return false
			}
			for _, rtm := range page.ResourceTagMappingList {
				switch {
				case strings.Contains(*rtm.ResourceARN, ":subnet/"):
					subnets[*rtm.ResourceARN] = rgtTagAsEC2Tag(rtm.Tags)
				}
			}
			return true
		})
		if err != nil {
			return nil, err
		}
	}

	return subnets, nil
}

func rgtTagAsEC2Tag(in []*resourcegroupstaggingapi.Tag) (tags util.EC2Tags) {
	for _, t := range in {
		tags = append(tags, &ec2.Tag{
			Key:   t.Key,
			Value: t.Value,
		})
	}
	return tags
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
