package elbv2

import (
	"github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/equality"
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

// CompareOptionForAction returns the compare option for action.
func CompareOptionForAction() cmp.Option {
	return cmp.Options{
		cmpopts.IgnoreFields(elbv2sdk.Action{}, "Order"),
		cmpopts.IgnoreFields(elbv2sdk.Action{}, "TargetGroupArn"),
		CompareOptionForForwardActionConfig(),
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
			return aws.Int64Value(lhs.Order) < aws.Int64Value(rhs.Order)
		}),
		CompareOptionForAction(),
	}
}
