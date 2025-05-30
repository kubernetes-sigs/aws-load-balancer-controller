package elbv2

import (
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func CompareOptionForTransform() cmp.Option {
	return cmp.Options{
		cmpopts.IgnoreUnexported(elbv2types.RuleTransform{}),
		cmpopts.IgnoreUnexported(elbv2types.HostHeaderRewriteConfig{}),
		cmpopts.IgnoreUnexported(elbv2types.UrlRewriteConfig{}),
		cmpopts.IgnoreUnexported(elbv2types.RewriteConfig{}),
	}
}

// CompareOptionForTransforms returns the compare option for transforms slice.
func CompareOptionForTransforms(_, _ []elbv2types.RuleTransform) cmp.Option {
	return cmp.Options{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(lhs elbv2types.RuleTransform, rhs elbv2types.RuleTransform) bool {
			return string(lhs.Type) < string(rhs.Type)
		}),
		CompareOptionForTransform(),
	}
}
