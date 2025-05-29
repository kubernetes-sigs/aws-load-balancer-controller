package elbv2

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/stretchr/testify/assert"
)

func TestTargetGroupCache(t *testing.T) {
	targetGroups := []TargetGroupWithTags{
		{
			TargetGroup: &elbv2types.TargetGroup{
				TargetGroupArn: awssdk.String("arn-1"),
			},
		},
		{
			TargetGroup: &elbv2types.TargetGroup{
				TargetGroupArn: awssdk.String("arn-2"),
			},
		},
	}

	t.Run("empty cache returns no data", func(t *testing.T) {
		cache := NewTargetGroupCache()
		groups, exists := cache.GetSDKTargetGroups()
		assert.False(t, exists)
		assert.Nil(t, groups)
	})

	t.Run("can store and retrieve target groups", func(t *testing.T) {
		cache := NewTargetGroupCache()
		cache.SetSDKTargetGroups(targetGroups)

		groups, exists := cache.GetSDKTargetGroups()
		assert.True(t, exists)
		assert.Equal(t, targetGroups, groups)
	})

	t.Run("can update stored target groups", func(t *testing.T) {
		cache := NewTargetGroupCache()

		cache.SetSDKTargetGroups(targetGroups)

		updatedTargetGroups := []TargetGroupWithTags{
			{
				TargetGroup: &elbv2types.TargetGroup{
					TargetGroupArn: awssdk.String("arn-3"),
				},
			},
		}
		cache.SetSDKTargetGroups(updatedTargetGroups)

		groups, exists := cache.GetSDKTargetGroups()
		assert.True(t, exists)
		assert.Equal(t, updatedTargetGroups, groups)
		assert.NotEqual(t, targetGroups, groups)
	})
}
