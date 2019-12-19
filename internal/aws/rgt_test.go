package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
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
