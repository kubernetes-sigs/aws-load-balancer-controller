package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_CompareOptionForRuleConditions(t *testing.T) {
	testCase := []struct {
		name                 string
		desiredRuleCondition []types.RuleCondition
		actualRuleCondition  []types.RuleCondition
		expected             bool
	}{
		{
			name: "equal",
			desiredRuleCondition: []types.RuleCondition{
				{
					Field: awssdk.String("host-header"),
					HostHeaderConfig: &types.HostHeaderConditionConfig{
						Values: []string{"h1"},
					},
				},
				{
					Field: awssdk.String("path-pattern"),
					PathPatternConfig: &types.PathPatternConditionConfig{
						Values: []string{"path-pattern"},
					},
				},
			},
			actualRuleCondition: []types.RuleCondition{
				{
					Field: awssdk.String("host-header"),
					HostHeaderConfig: &types.HostHeaderConditionConfig{
						Values: []string{"h1"},
					},
				},
				{
					Field: awssdk.String("path-pattern"),
					PathPatternConfig: &types.PathPatternConditionConfig{
						Values: []string{"path-pattern"},
					},
				},
			},
			expected: true,
		},
		{
			name: "equal - different order",
			desiredRuleCondition: []types.RuleCondition{
				{
					Field: awssdk.String("host-header"),
					HostHeaderConfig: &types.HostHeaderConditionConfig{
						Values: []string{"h1"},
					},
				},
				{
					Field: awssdk.String("path-pattern"),
					PathPatternConfig: &types.PathPatternConditionConfig{
						Values: []string{"path-pattern"},
					},
				},
			},
			actualRuleCondition: []types.RuleCondition{
				{
					Field: awssdk.String("path-pattern"),
					PathPatternConfig: &types.PathPatternConditionConfig{
						Values: []string{"path-pattern"},
					},
				},
				{
					Field: awssdk.String("host-header"),
					HostHeaderConfig: &types.HostHeaderConditionConfig{
						Values: []string{"h1"},
					},
				},
			},
			expected: true,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			conditionsEqual := cmp.Equal(tc.desiredRuleCondition, tc.actualRuleCondition, CompareOptionForRuleConditions(nil, nil))
			assert.Equal(t, tc.expected, conditionsEqual)
		})
	}
}
