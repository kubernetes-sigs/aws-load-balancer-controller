package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	"sort"
)

func CompareOptionForTargetGroupTuples() cmp.Option {
	return cmp.Options{
		cmpopts.IgnoreUnexported(elbv2types.TargetGroupTuple{}),
		cmpopts.AcyclicTransformer("sortAndNormalize", func(tgt []elbv2types.TargetGroupTuple) []elbv2types.TargetGroupTuple {
			// Handle empty slice
			if len(tgt) == 0 {
				return tgt
			}

			// Sort by ARN for consistent ordering (for all cases)
			result := make([]elbv2types.TargetGroupTuple, len(tgt))
			copy(result, tgt)

			sort.Slice(result, func(i, j int) bool {
				arnI := awssdk.ToString(result[i].TargetGroupArn)
				arnJ := awssdk.ToString(result[j].TargetGroupArn)
				return arnI < arnJ
			})

			// Normalize weights (only for single target groups)
			if len(result) == 1 {
				result[0].Weight = nil
			}

			return result
		}),
	}
}

func CompareOptionForForwardActionConfig() cmp.Option {
	return cmp.Options{
		equality.IgnoreLeftHandUnset(elbv2types.ForwardActionConfig{}, "TargetGroupStickinessConfig"),
		cmpopts.IgnoreUnexported(elbv2types.ForwardActionConfig{}),
		cmpopts.IgnoreUnexported(elbv2types.TargetGroupStickinessConfig{}),
		CompareOptionForTargetGroupTuples(),
	}
}

func CompareOptionForRedirectActionConfig() cmp.Option {
	return cmp.Options{
		cmpopts.IgnoreUnexported(elbv2types.RedirectActionConfig{}),
		cmpopts.AcyclicTransformer("normalizeRedirectActionConfig", func(config *elbv2types.RedirectActionConfig) *elbv2types.RedirectActionConfig {
			if config == nil {
				return nil
			}
			normalizedCFG := *config
			if normalizedCFG.Host == nil {
				normalizedCFG.Host = awssdk.String("#{host}")
			}
			if normalizedCFG.Path == nil {
				normalizedCFG.Path = awssdk.String("/#{path}")
			}
			if normalizedCFG.Port == nil {
				normalizedCFG.Port = awssdk.String("#{port}")
			}
			if normalizedCFG.Protocol == nil {
				normalizedCFG.Protocol = awssdk.String("#{protocol}")
			}
			if normalizedCFG.Query == nil {
				normalizedCFG.Query = awssdk.String("#{query}")
			}
			return &normalizedCFG
		}),
	}
}

// CompareOptionForAction returns the compare option for action.
func CompareOptionForAction() cmp.Option {
	return cmp.Options{
		cmpopts.IgnoreFields(elbv2types.Action{}, "Order"),
		cmpopts.IgnoreFields(elbv2types.Action{}, "TargetGroupArn"),
		cmpopts.IgnoreUnexported(elbv2types.Action{}),
		CompareOptionForForwardActionConfig(),
		CompareOptionForRedirectActionConfig(),
	}
}

// CompareOptionForActions returns the compare option for action slice.
// IMPORTANT:
// When changing the types compared (e.g. the input to the function)
// ensure to update cmpopts.SortSlices to reflect the new type, otherwise sorting silently doesn't work.
func CompareOptionForActions(_, _ []elbv2types.Action) cmp.Option {
	return cmp.Options{
		cmpopts.EquateEmpty(),
		cmpopts.IgnoreUnexported(elbv2types.AuthenticateCognitoActionConfig{}),
		cmpopts.IgnoreUnexported(elbv2types.AuthenticateOidcActionConfig{}),
		cmpopts.IgnoreUnexported(elbv2types.JwtValidationActionConfig{}),
		cmpopts.IgnoreUnexported(elbv2types.JwtValidationActionAdditionalClaim{}),
		cmpopts.IgnoreUnexported(elbv2types.FixedResponseActionConfig{}),
		cmpopts.SortSlices(func(lhs elbv2types.Action, rhs elbv2types.Action) bool {
			if lhs.Order == nil || rhs.Order == nil {
				return false
			}
			return awssdk.ToInt32(lhs.Order) < awssdk.ToInt32(rhs.Order)
		}),
		CompareOptionForAction(),
	}
}
