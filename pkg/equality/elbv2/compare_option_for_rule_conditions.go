package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func CompareOptionForRuleCondition() cmp.Option {
	return cmp.Options{
		cmpopts.IgnoreFields(elbv2sdk.RuleCondition{}, "Values"),
	}
}

// CompareOptionForRuleConditions returns the compare option for rule conditions slice.
func CompareOptionForRuleConditions() cmp.Option {
	return cmp.Options{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(lhs *elbv2sdk.RuleCondition, rhs *elbv2sdk.RuleCondition) bool {
			return awssdk.StringValue(lhs.Field) < awssdk.StringValue(rhs.Field)
		}),
		CompareOptionForRuleCondition(),
	}
}
