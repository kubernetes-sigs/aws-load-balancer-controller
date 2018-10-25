package aws

import (
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"

	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

const (
	ResourceTypeEnumELBTargetGroup = "elasticloadbalancing:targetgroup"
)

type ResourceGroupsTaggingAPIAPI interface {
	GetClusterResources() (*Resources, error)

	// GetResourcesByFilters fetches resources ARNs by tagFilters and 0 or more resourceTypesFilters
	GetResourcesByFilters(tagFilters map[string][]string, resourceTypeFilters ...string) ([]string, error)

	TagResources(*resourcegroupstaggingapi.TagResourcesInput) (*resourcegroupstaggingapi.TagResourcesOutput, error)
	UntagResources(*resourcegroupstaggingapi.UntagResourcesInput) (*resourcegroupstaggingapi.UntagResourcesOutput, error)
}

func (c *Cloud) TagResources(i *resourcegroupstaggingapi.TagResourcesInput) (*resourcegroupstaggingapi.TagResourcesOutput, error) {
	return c.rgt.TagResources(i)
}
func (c *Cloud) UntagResources(i *resourcegroupstaggingapi.UntagResourcesInput) (*resourcegroupstaggingapi.UntagResourcesOutput, error) {
	return c.rgt.UntagResources(i)
}

type Resources struct {
	LoadBalancers map[string]util.ELBv2Tags
	Listeners     map[string]util.ELBv2Tags
	ListenerRules map[string]util.ELBv2Tags
	TargetGroups  map[string]util.ELBv2Tags
	Subnets       map[string]util.EC2Tags
}

// GetClusterResources looks up all ELBV2 (ALB) resources in AWS that are part of the cluster.
func (c *Cloud) GetClusterResources() (*Resources, error) {
	resources := &Resources{
		LoadBalancers: make(map[string]util.ELBv2Tags),
		Listeners:     make(map[string]util.ELBv2Tags),
		ListenerRules: make(map[string]util.ELBv2Tags),
		TargetGroups:  make(map[string]util.ELBv2Tags),
		Subnets:       make(map[string]util.EC2Tags),
	}

	paramSet := []*resourcegroupstaggingapi.GetResourcesInput{
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
		{
			ResourcesPerPage: aws.Int64(50),
			ResourceTypeFilters: []*string{
				aws.String("elasticloadbalancing"),
			},
			TagFilters: []*resourcegroupstaggingapi.TagFilter{
				{
					Key:    aws.String("kubernetes.io/cluster/" + c.clusterName),
					Values: []*string{aws.String("owned"), aws.String("shared")},
				},
			},
		},
	}

	for _, param := range paramSet {
		err := c.rgt.GetResourcesPages(param, func(page *resourcegroupstaggingapi.GetResourcesOutput, lastPage bool) bool {
			for _, rtm := range page.ResourceTagMappingList {
				switch {
				case strings.Contains(*rtm.ResourceARN, ":loadbalancer/app/"):
					resources.LoadBalancers[*rtm.ResourceARN] = rgtTagAsELBV2Tag(rtm.Tags)
				case strings.Contains(*rtm.ResourceARN, ":listener/app/"):
					resources.Listeners[*rtm.ResourceARN] = rgtTagAsELBV2Tag(rtm.Tags)
				case strings.Contains(*rtm.ResourceARN, ":listener-rule/app/"):
					resources.ListenerRules[*rtm.ResourceARN] = rgtTagAsELBV2Tag(rtm.Tags)
				case strings.Contains(*rtm.ResourceARN, ":targetgroup/"):
					resources.TargetGroups[*rtm.ResourceARN] = rgtTagAsELBV2Tag(rtm.Tags)
				case strings.Contains(*rtm.ResourceARN, ":subnet/"):
					resources.Subnets[*rtm.ResourceARN] = rgtTagAsEC2Tag(rtm.Tags)
				}
			}
			return true
		})
		if err != nil {
			return nil, err
		}
	}

	if os.Getenv("ALB_SUPPORT_LEGACY_DEPLOYMENTS") != "" {
		// Legacy deployments may not have the proper tags, and RGT doesn't allow you to use wildcards on names
		err := c.rgt.GetResourcesPages(&resourcegroupstaggingapi.GetResourcesInput{
			ResourcesPerPage: aws.Int64(50),
			ResourceTypeFilters: []*string{
				aws.String("elasticloadbalancing"),
			},
		}, func(page *resourcegroupstaggingapi.GetResourcesOutput, lastPage bool) bool {
			for _, rtm := range page.ResourceTagMappingList {
				s := strings.Split(*rtm.ResourceARN, ":")
				if strings.HasPrefix(s[5], "targetgroup/"+c.clusterName) {
					resources.TargetGroups[*rtm.ResourceARN] = rgtTagAsELBV2Tag(rtm.Tags)
				}
				if strings.HasPrefix(s[5], "loadbalancer/app/"+c.clusterName) {
					resources.LoadBalancers[*rtm.ResourceARN] = rgtTagAsELBV2Tag(rtm.Tags)
				}
			}
			return true
		})
		if err != nil {
			return nil, err
		}
	}

	return resources, nil
}

func rgtTagAsELBV2Tag(in []*resourcegroupstaggingapi.Tag) (tags util.ELBv2Tags) {
	for _, t := range in {
		tags = append(tags, &elbv2.Tag{
			Key:   t.Key,
			Value: t.Value,
		})
	}
	return tags
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
		for _, i := range output.ResourceTagMappingList {
			result = append(result, aws.StringValue(i.ResourceARN))
		}
		return true
	})
	return result, err
}
