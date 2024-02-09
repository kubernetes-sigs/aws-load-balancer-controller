package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
)

func CompareOptionForTargetGroupTuples() cmp.Option {
	return cmpopts.AcyclicTransformer("normalizeWeights", func(tgt []*elbv2sdk.TargetGroupTuple) []*elbv2sdk.TargetGroupTuple {
		if len(tgt) != 1 {
			return tgt
		}
		singleTG := tgt[0]
		return []*elbv2sdk.TargetGroupTuple{
			{
				TargetGroupArn: singleTG.TargetGroupArn,
				Weight:         nil,
			},
		}
	})
}

func CompareOptionForForwardActionConfig() cmp.Option {
	return cmp.Options{
		equality.IgnoreLeftHandUnset(elbv2sdk.ForwardActionConfig{}, "TargetGroupStickinessConfig"),
		CompareOptionForTargetGroupTuples(),
	}
}

func CompareOptionForRedirectActionConfig() cmp.Option {
	return cmpopts.AcyclicTransformer("normalizeRedirectActionConfig", func(config *elbv2sdk.RedirectActionConfig) *elbv2sdk.RedirectActionConfig {
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
	})
}

// CompareOptionForAction returns the compare option for action.
func CompareOptionForAction() cmp.Option {
	return cmp.Options{
		cmpopts.IgnoreFields(elbv2sdk.Action{}, "Order"),
		cmpopts.IgnoreFields(elbv2sdk.Action{}, "TargetGroupArn"),
		CompareOptionForForwardActionConfig(),
		CompareOptionForRedirectActionConfig(),
	}
}

// CompareOptionForActions returns the compare option for action slice.
func CompareOptionForActions() cmp.Option {
	return cmp.Options{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(lhs *elbv2sdk.Action, rhs *elbv2sdk.Action) bool {
			if lhs.Order == nil || rhs.Order == nil {
				return false
			}
			return awssdk.Int64Value(lhs.Order) < awssdk.Int64Value(rhs.Order)
		}),
		CompareOptionForAction(),
	}
}
