package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func CompareOptionForRuleCondition() cmp.Option {
	return cmp.Options{
		cmpopts.IgnoreUnexported(elbv2types.RuleCondition{}),
		cmpopts.IgnoreUnexported(elbv2types.HostHeaderConditionConfig{}),
		cmpopts.IgnoreUnexported(elbv2types.HttpHeaderConditionConfig{}),
		cmpopts.IgnoreUnexported(elbv2types.HttpRequestMethodConditionConfig{}),
		cmpopts.IgnoreUnexported(elbv2types.PathPatternConditionConfig{}),
		cmpopts.IgnoreUnexported(elbv2types.QueryStringConditionConfig{}),
		cmpopts.IgnoreUnexported(elbv2types.QueryStringKeyValuePair{}),
		cmpopts.IgnoreUnexported(elbv2types.SourceIpConditionConfig{}),
		cmpopts.IgnoreFields(elbv2types.RuleCondition{}, "Values"),
	}
}

// CompareOptionForRuleConditions returns the compare option for rule conditions slice.
func CompareOptionForRuleConditions() cmp.Option {
	return cmp.Options{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(lhs *elbv2types.RuleCondition, rhs *elbv2types.RuleCondition) bool {
			return awssdk.ToString(lhs.Field) < awssdk.ToString(rhs.Field)
		}),
		CompareOptionForRuleCondition(),
	}
}
