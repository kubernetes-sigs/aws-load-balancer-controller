package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
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
