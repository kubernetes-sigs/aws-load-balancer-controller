package albrgt

import (
	"strings"

	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi/resourcegroupstaggingapiiface"

	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

// RGTsvc is a pointer to the aws ResourceGroupsTaggingAPI service
var RGTsvc *RGT

// RGT is our extension to AWS's resourcegroupstaggingapi.ResourceGroupsTaggingAPI
type RGT struct {
	resourcegroupstaggingapiiface.ResourceGroupsTaggingAPIAPI
}

// NewRGT sets RGTsvc based off of the provided AWS session
func NewRGT(awsSession *session.Session) {
	RGTsvc = &RGT{
		resourcegroupstaggingapi.New(awsSession),
	}
}

type Resources struct {
	LoadBalancers map[string]util.ELBv2Tags
	Listeners     map[string]util.ELBv2Tags
	ListenerRules map[string]util.ELBv2Tags
	TargetGroups  map[string]util.ELBv2Tags
	Subnets       map[string]util.EC2Tags
}

// GetResources looks up all ELBV2 (ALB) resources in AWS that are part of the cluster.
func (r *RGT) GetResources(clusterName *string) (*Resources, error) {
	resources := &Resources{
		LoadBalancers: make(map[string]util.ELBv2Tags),
		Listeners:     make(map[string]util.ELBv2Tags),
		ListenerRules: make(map[string]util.ELBv2Tags),
		TargetGroups:  make(map[string]util.ELBv2Tags),
		Subnets:       make(map[string]util.EC2Tags),
	}

	paramSet := []*resourcegroupstaggingapi.GetResourcesInput{
		&resourcegroupstaggingapi.GetResourcesInput{
			ResourceTypeFilters: []*string{
				aws.String("ec2"),
			},
			TagFilters: []*resourcegroupstaggingapi.TagFilter{
				&resourcegroupstaggingapi.TagFilter{
					Key: aws.String("kubernetes.io/role/internal-elb"),
				},
				&resourcegroupstaggingapi.TagFilter{
					Key:    aws.String("kubernetes.io/cluster/" + *clusterName),
					Values: []*string{aws.String("owned"), aws.String("shared")},
				},
			},
		},
		&resourcegroupstaggingapi.GetResourcesInput{
			ResourceTypeFilters: []*string{
				aws.String("ec2"),
			},
			TagFilters: []*resourcegroupstaggingapi.TagFilter{
				&resourcegroupstaggingapi.TagFilter{
					Key: aws.String("kubernetes.io/role/elb"),
				},
				&resourcegroupstaggingapi.TagFilter{
					Key:    aws.String("kubernetes.io/cluster/" + *clusterName),
					Values: []*string{aws.String("owned"), aws.String("shared")},
				},
			},
		},
		&resourcegroupstaggingapi.GetResourcesInput{
			ResourceTypeFilters: []*string{
				aws.String("elasticloadbalancing"),
			},
			TagFilters: []*resourcegroupstaggingapi.TagFilter{
				&resourcegroupstaggingapi.TagFilter{
					Key:    aws.String("kubernetes.io/cluster/" + *clusterName),
					Values: []*string{aws.String("owned"), aws.String("shared")},
				},
			},
		},
	}

	for _, param := range paramSet {
		p := request.Pagination{
			EndPageOnSameToken: true,
			NewRequest: func() (*request.Request, error) {
				req, _ := r.GetResourcesRequest(param)
				return req, nil
			},
		}

		for p.Next() {
			page := p.Page().(*resourcegroupstaggingapi.GetResourcesOutput)
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
		}
		if p.Err() != nil {
			return nil, p.Err()
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
