package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
)

func CompareOptionForTargetGroupTuples() cmp.Option {
	return cmp.Options{
		cmpopts.IgnoreUnexported(elbv2types.TargetGroupTuple{}),
		cmpopts.AcyclicTransformer("normalizeWeights", func(tgt []elbv2types.TargetGroupTuple) []elbv2types.TargetGroupTuple {
			if len(tgt) != 1 {
				return tgt
			}
			singleTG := tgt[0]
			return []elbv2types.TargetGroupTuple{
				{
					TargetGroupArn: singleTG.TargetGroupArn,
					Weight:         nil,
				},
			}
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
func CompareOptionForActions() cmp.Option {
	return cmp.Options{
		cmpopts.EquateEmpty(),
		cmpopts.IgnoreUnexported(elbv2types.AuthenticateCognitoActionConfig{}),
		cmpopts.IgnoreUnexported(elbv2types.AuthenticateOidcActionConfig{}),
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
