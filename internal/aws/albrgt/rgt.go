package albrgt

import (
	"strings"
	"time"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"

	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi/resourcegroupstaggingapiiface"

	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

const (
	GetResourcesCacheTTL = time.Minute * 60
)

// RGTsvc is a pointer to the aws ResourceGroupsTaggingAPI service
var RGTsvc RGTiface

type RGTiface interface {
	resourcegroupstaggingapiiface.ResourceGroupsTaggingAPIAPI
	GetClusterResources() (*Resources, error)
	SetResponse(interface{}, error)
}

// RGT is our extension to AWS's resourcegroupstaggingapi.ResourceGroupsTaggingAPI
type RGT struct {
	resourcegroupstaggingapiiface.ResourceGroupsTaggingAPIAPI
	clusterName string
}

// NewRGT sets RGTsvc based off of the provided AWS session
func NewRGT(awsSession *session.Session, clusterName string) {
	RGTsvc = &RGT{
		resourcegroupstaggingapi.New(awsSession),
		clusterName,
	}
}

type Resources struct {
	LoadBalancers map[string]util.ELBv2Tags
	Listeners     map[string]util.ELBv2Tags
	ListenerRules map[string]util.ELBv2Tags
	TargetGroups  map[string]util.ELBv2Tags
	Subnets       map[string]util.EC2Tags
}

// GetClusterResources looks up all ELBV2 (ALB) resources in AWS that are part of the cluster.
func (r *RGT) GetClusterResources() (*Resources, error) {
	cacheName := "ResourceGroupsTagging.GetClusterResources"
	item := albcache.Get(cacheName, "")

	if item != nil {
		r := item.Value().(*Resources)
		return r, nil
	}

	resources := &Resources{
		LoadBalancers: make(map[string]util.ELBv2Tags),
		Listeners:     make(map[string]util.ELBv2Tags),
		ListenerRules: make(map[string]util.ELBv2Tags),
		TargetGroups:  make(map[string]util.ELBv2Tags),
		Subnets:       make(map[string]util.EC2Tags),
	}

	paramSet := []*resourcegroupstaggingapi.GetResourcesInput{
		{
			ResourceTypeFilters: []*string{
				aws.String("ec2"),
			},
			TagFilters: []*resourcegroupstaggingapi.TagFilter{
				{
					Key:    aws.String("kubernetes.io/role/internal-elb"),
					Values: []*string{aws.String(""), aws.String("1")},
				},
				{
					Key:    aws.String("kubernetes.io/cluster/" + r.clusterName),
					Values: []*string{aws.String("owned"), aws.String("shared")},
				},
			},
		},
		{
			ResourceTypeFilters: []*string{
				aws.String("ec2"),
			},
			TagFilters: []*resourcegroupstaggingapi.TagFilter{
				{
					Key:    aws.String("kubernetes.io/role/elb"),
					Values: []*string{aws.String(""), aws.String("1")},
				},
				{
					Key:    aws.String("kubernetes.io/cluster/" + r.clusterName),
					Values: []*string{aws.String("owned"), aws.String("shared")},
				},
			},
		},
		{
			ResourceTypeFilters: []*string{
				aws.String("elasticloadbalancing"),
			},
			TagFilters: []*resourcegroupstaggingapi.TagFilter{
				{
					Key:    aws.String("kubernetes.io/cluster/" + r.clusterName),
					Values: []*string{aws.String("owned"), aws.String("shared")},
				},
			},
		},
	}

	for _, param := range paramSet {
		err := r.GetResourcesPages(param, func(page *resourcegroupstaggingapi.GetResourcesOutput, lastPage bool) bool {
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

	// Legacy deployments may not have the proper tags, and RGT doesn't allow you to use wildcards on names
	p := request.Pagination{
		EndPageOnSameToken: true,
		NewRequest: func() (*request.Request, error) {
			req, _ := r.GetResourcesRequest(&resourcegroupstaggingapi.GetResourcesInput{
				ResourceTypeFilters: []*string{
					aws.String("elasticloadbalancing"),
				},
			})
			return req, nil
		},
	}
	for p.Next() {
		page := p.Page().(*resourcegroupstaggingapi.GetResourcesOutput)
		for _, rtm := range page.ResourceTagMappingList {
			// arn:aws:elasticloadbalancing:us-east-1:234212695392:loadbalancer/app/2a6df4b6-prd112-distribute-1704
			s := strings.Split(*rtm.ResourceARN, ":")
			if strings.HasPrefix(s[5], "targetgroup/"+r.clusterName) {
				resources.TargetGroups[*rtm.ResourceARN] = rgtTagAsELBV2Tag(rtm.Tags)
			}
			if strings.HasPrefix(s[5], "loadbalancer/app/"+r.clusterName) {
				resources.LoadBalancers[*rtm.ResourceARN] = rgtTagAsELBV2Tag(rtm.Tags)
			}
		}
	}
	if p.Err() != nil {
		return nil, p.Err()
	}

	albcache.Set(cacheName, "", resources, GetResourcesCacheTTL)
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

func (r *RGT) SetResponse(i interface{}, e error) {
}
