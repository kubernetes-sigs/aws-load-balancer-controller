package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCloud_TagResourcesWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ResourceGroupsTaggingAPIAPI{}

		i := &resourcegroupstaggingapi.TagResourcesInput{}
		o := &resourcegroupstaggingapi.TagResourcesOutput{}
		var e error

		svc.On("TagResourcesWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			rgt: svc,
		}

		a, b := cloud.TagResourcesWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func TestCloud_UntagResourcesWithContext(t *testing.T) {
	t.Run("apiwrapper", func(t *testing.T) {
		ctx := context.Background()
		svc := &mocks.ResourceGroupsTaggingAPIAPI{}

		i := &resourcegroupstaggingapi.UntagResourcesInput{}
		o := &resourcegroupstaggingapi.UntagResourcesOutput{}
		var e error

		svc.On("UntagResourcesWithContext", ctx, i).Return(o, e)
		cloud := &Cloud{
			rgt: svc,
		}

		a, b := cloud.UntagResourcesWithContext(ctx, i)
		assert.Equal(t, o, a)
		assert.Equal(t, b, e)
		svc.AssertExpectations(t)
	})
}

func tag(k, v string) *resourcegroupstaggingapi.Tag {
	return &resourcegroupstaggingapi.Tag{Key: String(k), Value: String(v)}
}

func tags(t ...*resourcegroupstaggingapi.Tag) []*resourcegroupstaggingapi.Tag {
	return t
}

func ec2Tag(k, v string) *ec2.Tag {
	return &ec2.Tag{Key: aws.String(k), Value: aws.String(v)}
}

func TestCloud_GetClusterSubnets(t *testing.T) {
	clusterName := "clusterName"
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
					Key:    aws.String("kubernetes.io/cluster/" + clusterName),
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
					Key:    aws.String("kubernetes.io/cluster/" + clusterName),
					Values: []*string{aws.String("owned"), aws.String("shared")},
				},
			},
		},
	}
	for _, tc := range []struct {
		Name               string
		GetResourcesOutput *resourcegroupstaggingapi.GetResourcesOutput
		GetResourcesError  awserr.Error
		Params             int
		ExpectedResult     map[string]util.EC2Tags
		ExpectedError      error
	}{
		{
			Name:   "No subnets returned",
			Params: 2,
			GetResourcesOutput: &resourcegroupstaggingapi.GetResourcesOutput{
				ResourceTagMappingList: []*resourcegroupstaggingapi.ResourceTagMapping{
					{
						ResourceARN: aws.String("arn1"),
						Tags:        tags(tag("tag1", "val1")),
					},
				},
			},
			ExpectedResult: map[string]util.EC2Tags{},
		},
		{
			Name:   "Three subnets returned",
			Params: 2,
			GetResourcesOutput: &resourcegroupstaggingapi.GetResourcesOutput{
				ResourceTagMappingList: []*resourcegroupstaggingapi.ResourceTagMapping{
					{
						ResourceARN: aws.String("arn:aws:ec2:region:account-id:subnet/subnet-id1"),
						Tags:        tags(tag("tag1", "val1")),
					},
					{
						ResourceARN: aws.String("arn:aws:ec2:region:account-id:subnet/subnet-id2"),
						Tags:        tags(tag("tag2", "val2")),
					},
					{
						ResourceARN: aws.String("arn:aws:ec2:region:account-id:subnet/subnet-id3"),
						Tags:        tags(tag("tag3", "val3")),
					},
				},
			},
			ExpectedResult: map[string]util.EC2Tags{
				"arn:aws:ec2:region:account-id:subnet/subnet-id1": {ec2Tag("tag1", "val1")},
				"arn:aws:ec2:region:account-id:subnet/subnet-id2": {ec2Tag("tag2", "val2")},
				"arn:aws:ec2:region:account-id:subnet/subnet-id3": {ec2Tag("tag3", "val3")},
			},
		},
		{
			Name:              "API throws an error",
			Params:            1,
			GetResourcesError: awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
			ExpectedError:     awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			rgtsvc := &mocks.ResourceGroupsTaggingAPIAPI{}

			for paramSet := 0; paramSet < tc.Params; paramSet++ {
				rgtsvc.On("GetResourcesPages",
					paramSets[paramSet],
					mock.AnythingOfType("func(*resourcegroupstaggingapi.GetResourcesOutput, bool) bool"),
				).Return(tc.GetResourcesError).Run(func(args mock.Arguments) {
					arg := args.Get(1).(func(*resourcegroupstaggingapi.GetResourcesOutput, bool) bool)
					arg(tc.GetResourcesOutput, false)
				})
			}

			cloud := &Cloud{
				clusterName: clusterName,
				rgt:         rgtsvc,
			}
			subnets, err := cloud.GetClusterSubnets()
			assert.Equal(t, tc.ExpectedResult, subnets)
			assert.Equal(t, tc.ExpectedError, err)
			rgtsvc.AssertExpectations(t)
		})
	}
}

func TestCloud_GetResourcesByFilters(t *testing.T) {
	for _, tc := range []struct {
		Name                string
		ResourceTypeFilters []string
		TagFilters          map[string][]string
		GetResourcesInput   *resourcegroupstaggingapi.GetResourcesInput
		GetResourcesOutput  *resourcegroupstaggingapi.GetResourcesOutput
		GetResourcesError   awserr.Error
		ExpectedResult      []string
		ExpectedError       error
	}{
		{
			Name:                "No results returned",
			ResourceTypeFilters: []string{ResourceTypeEnumELBTargetGroup},
			GetResourcesInput: &resourcegroupstaggingapi.GetResourcesInput{
				ResourceTypeFilters: []*string{aws.String(ResourceTypeEnumELBTargetGroup)},
			},
			GetResourcesOutput: &resourcegroupstaggingapi.GetResourcesOutput{
				ResourceTagMappingList: []*resourcegroupstaggingapi.ResourceTagMapping{},
			},
		},
		{
			Name:                "Two results returned",
			ResourceTypeFilters: []string{ResourceTypeEnumELBTargetGroup},
			GetResourcesInput: &resourcegroupstaggingapi.GetResourcesInput{
				ResourceTypeFilters: []*string{aws.String(ResourceTypeEnumELBTargetGroup)},
			},
			GetResourcesOutput: &resourcegroupstaggingapi.GetResourcesOutput{
				ResourceTagMappingList: []*resourcegroupstaggingapi.ResourceTagMapping{
					{ResourceARN: aws.String("arn1")},
					{ResourceARN: aws.String("arn2")},
				},
			},
			ExpectedResult: []string{"arn1", "arn2"},
		},
		{
			Name:                "Tag filter construction",
			ResourceTypeFilters: []string{ResourceTypeEnumELBTargetGroup},
			TagFilters:          map[string][]string{"key": {"val1", "val2"}},
			GetResourcesInput: &resourcegroupstaggingapi.GetResourcesInput{
				ResourceTypeFilters: []*string{aws.String(ResourceTypeEnumELBTargetGroup)},
				TagFilters: []*resourcegroupstaggingapi.TagFilter{
					{
						Key: aws.String("key"),
						Values: []*string{
							aws.String("val1"),
							aws.String("val2"),
						},
					},
				},
			},
			GetResourcesOutput: &resourcegroupstaggingapi.GetResourcesOutput{
				ResourceTagMappingList: []*resourcegroupstaggingapi.ResourceTagMapping{},
			},
		},
		{
			Name:                "API throws an error",
			ResourceTypeFilters: []string{ResourceTypeEnumELBTargetGroup},
			GetResourcesInput: &resourcegroupstaggingapi.GetResourcesInput{
				ResourceTypeFilters: []*string{aws.String(ResourceTypeEnumELBTargetGroup)},
			},
			GetResourcesError: awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
			ExpectedError:     awserr.New(request.ErrCodeResponseTimeout, "timeout", nil),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			rgtsvc := &mocks.ResourceGroupsTaggingAPIAPI{}

			rgtsvc.On("GetResourcesPages",
				tc.GetResourcesInput,
				mock.AnythingOfType("func(*resourcegroupstaggingapi.GetResourcesOutput, bool) bool"),
			).Return(tc.GetResourcesError).Run(func(args mock.Arguments) {
				arg := args.Get(1).(func(*resourcegroupstaggingapi.GetResourcesOutput, bool) bool)
				arg(tc.GetResourcesOutput, false)
			})

			cloud := &Cloud{
				rgt: rgtsvc,
			}
			arns, err := cloud.GetResourcesByFilters(tc.TagFilters, tc.ResourceTypeFilters...)
			assert.Equal(t, tc.ExpectedResult, arns)
			assert.Equal(t, tc.ExpectedError, err)
			rgtsvc.AssertExpectations(t)
		})
	}
}
