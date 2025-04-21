package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_matchResAndSDKListenerRules(t *testing.T) {
	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
	type args struct {
		resLRs []*elbv2model.ListenerRule
		sdkLRs []ListenerRuleWithTags
	}
	tests := []struct {
		name    string
		args    args
		want    []resAndSDKListenerRulePair
		want1   []resAndSDKListenerRulePair
		want2   []*elbv2model.ListenerRule
		want3   []ListenerRuleWithTags
		wantErr error
	}{
		{
			name: "all listener rules has match",
			args: args{
				resLRs: []*elbv2model.ListenerRule{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 1,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo1-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo1"},
									},
								},
							},
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 2,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo2-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo2"},
									},
								},
							},
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 3,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo3-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo3"},
									},
								},
							},
						},
					},
				},
				sdkLRs: []ListenerRuleWithTags{
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-1"),
							Priority: awssdk.String("1"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo1-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo1"},
									},
								},
							},
						},
					},
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-2"),
							Priority: awssdk.String("2"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo2-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo2"},
									},
								},
							},
						},
					},
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-3"),
							Priority: awssdk.String("3"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo3-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo3"},
									},
								},
							},
						},
					},
				},
			},
			want:    nil,
			want1:   nil,
			want2:   nil,
			want3:   nil,
			wantErr: nil,
		},
		{
			name: "all listener rules settings has match but not priority",
			args: args{
				resLRs: []*elbv2model.ListenerRule{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 3,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo1-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo1"},
									},
								},
							},
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 2,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo2-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo2"},
									},
								},
							},
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 1,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo3-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo3"},
									},
								},
							},
						},
					},
				},
				sdkLRs: []ListenerRuleWithTags{
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-1"),
							Priority: awssdk.String("1"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo1-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo1"},
									},
								},
							},
						},
					},
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-2"),
							Priority: awssdk.String("2"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo2-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo2"},
									},
								},
							},
						},
					},
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-3"),
							Priority: awssdk.String("3"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo3-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo3"},
									},
								},
							},
						},
					},
				},
			},
			want: []resAndSDKListenerRulePair{
				{
					resLR: &elbv2model.ListenerRule{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 3,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo1-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo1"},
									},
								},
							},
						},
					},
					sdkLR: ListenerRuleWithTags{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-1"),
							Priority: awssdk.String("1"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo1-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo1"},
									},
								},
							},
						},
					},
				},
				{
					resLR: &elbv2model.ListenerRule{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 1,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo3-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo3"},
									},
								},
							},
						},
					},
					sdkLR: ListenerRuleWithTags{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-3"),
							Priority: awssdk.String("3"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo3-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo3"},
									},
								},
							},
						},
					},
				},
			},
			want1:   nil,
			want2:   nil,
			want3:   nil,
			wantErr: nil,
		},
		{
			name: "some listener rules settings has match but not priority, some listener rules priority match but not settings",
			args: args{
				resLRs: []*elbv2model.ListenerRule{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 3,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo1-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo1"},
									},
								},
							},
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 2,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo4-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo4"},
									},
								},
							},
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 1,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo3-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo3"},
									},
								},
							},
						},
					},
				},
				sdkLRs: []ListenerRuleWithTags{
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-1"),
							Priority: awssdk.String("1"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo1-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo1"},
									},
								},
							},
						},
					},
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-2"),
							Priority: awssdk.String("2"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo2-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo2"},
									},
								},
							},
						},
					},
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-3"),
							Priority: awssdk.String("3"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo3-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo3"},
									},
								},
							},
						},
					},
				},
			},
			want: []resAndSDKListenerRulePair{
				{
					resLR: &elbv2model.ListenerRule{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 3,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo1-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo1"},
									},
								},
							},
						},
					},
					sdkLR: ListenerRuleWithTags{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-1"),
							Priority: awssdk.String("1"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo1-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo1"},
									},
								},
							},
						},
					},
				},
				{
					resLR: &elbv2model.ListenerRule{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 1,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo3-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo3"},
									},
								},
							},
						},
					},
					sdkLR: ListenerRuleWithTags{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-3"),
							Priority: awssdk.String("3"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo3-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo3"},
									},
								},
							},
						},
					},
				},
			},
			want1: []resAndSDKListenerRulePair{
				{
					resLR: &elbv2model.ListenerRule{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 2,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo4-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo4"},
									},
								},
							},
						},
					},
					sdkLR: ListenerRuleWithTags{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-2"),
							Priority: awssdk.String("2"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo2-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo2"},
									},
								},
							},
						},
					},
				},
			},
			want2:   nil,
			want3:   nil,
			wantErr: nil,
		},
		{
			name: "some listener rules settings has match but not priority, some listener rules priority match but not settings(requires modification), some needs to be created and some sdk needs to be deleted",
			args: args{
				resLRs: []*elbv2model.ListenerRule{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 1,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo1-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo1"},
									},
								},
							},
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 2,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo2-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo2"},
									},
								},
							},
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 3,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo5-updated-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo5"},
									},
								},
							},
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 4,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo6-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo6"},
									},
								},
							},
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 5,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo4-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo4"},
									},
								},
							},
						},
					},
				},
				sdkLRs: []ListenerRuleWithTags{
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-1"),
							Priority: awssdk.String("1"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo1-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo1"},
									},
								},
							},
						},
					},
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-2"),
							Priority: awssdk.String("2"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo2-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo2"},
									},
								},
							},
						},
					},
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-3"),
							Priority: awssdk.String("3"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo3-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo3"},
									},
								},
							},
						},
					},
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-4"),
							Priority: awssdk.String("4"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo4-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo4"},
									},
								},
							},
						},
					},
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-5"),
							Priority: awssdk.String("5"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo5-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo5"},
									},
								},
							},
						},
					},
				},
			},
			want: []resAndSDKListenerRulePair{
				{
					resLR: &elbv2model.ListenerRule{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 5,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo4-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo4"},
									},
								},
							},
						},
					},
					sdkLR: ListenerRuleWithTags{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-4"),
							Priority: awssdk.String("4"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo4-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo4"},
									},
								},
							},
						},
					},
				},
			},
			want1: []resAndSDKListenerRulePair{
				{
					resLR: &elbv2model.ListenerRule{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
						Spec: elbv2model.ListenerRuleSpec{
							Priority: 3,
							Actions: []elbv2model.Action{
								{
									Type: "forward",
									ForwardConfig: &elbv2model.ForwardActionConfig{
										TargetGroups: []elbv2model.TargetGroupTuple{
											{
												TargetGroupARN: core.LiteralStringToken("foo5-updated-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2model.RuleCondition{
								{
									Field: "path-pattern",
									PathPatternConfig: &elbv2model.PathPatternConditionConfig{
										Values: []string{"/foo5"},
									},
								},
							},
						},
					},
					sdkLR: ListenerRuleWithTags{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-3"),
							Priority: awssdk.String("3"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo3-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo3"},
									},
								},
							},
						},
					},
				},
			},
			want2: []*elbv2model.ListenerRule{
				{
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", "id-1"),
					Spec: elbv2model.ListenerRuleSpec{
						Priority: 4,
						Actions: []elbv2model.Action{
							{
								Type: "forward",
								ForwardConfig: &elbv2model.ForwardActionConfig{
									TargetGroups: []elbv2model.TargetGroupTuple{
										{
											TargetGroupARN: core.LiteralStringToken("foo6-tg"),
										},
									},
								},
							},
						},
						Conditions: []elbv2model.RuleCondition{
							{
								Field: "path-pattern",
								PathPatternConfig: &elbv2model.PathPatternConditionConfig{
									Values: []string{"/foo6"},
								},
							},
						},
					},
				},
			},
			want3: []ListenerRuleWithTags{
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn:  awssdk.String("arn-5"),
						Priority: awssdk.String("5"),
						Actions: []elbv2types.Action{
							{
								Type: elbv2types.ActionTypeEnumForward,
								ForwardConfig: &elbv2types.ForwardActionConfig{
									TargetGroups: []elbv2types.TargetGroupTuple{
										{
											TargetGroupArn: awssdk.String("foo5-tg"),
										},
									},
								},
							},
						},
						Conditions: []elbv2types.RuleCondition{
							{
								Field: awssdk.String("path-pattern"),
								PathPatternConfig: &elbv2types.PathPatternConditionConfig{
									Values: []string{"/foo5"},
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			featureGates := config.NewFeatureGates()
			s := &listenerRuleSynthesizer{
				elbv2Client:  elbv2Client,
				featureGates: featureGates,
			}
			resLRDesiredActionsAndConditionsPairs := make(map[*elbv2model.ListenerRule]*resLRDesiredActionsAndConditionsPair, len(tt.args.resLRs))
			for _, resLR := range tt.args.resLRs {
				resLRDesiredActionsAndConditionsPair, _ := buildResLRDesiredActionsAndConditionsPair(resLR, featureGates)
				resLRDesiredActionsAndConditionsPairs[resLR] = resLRDesiredActionsAndConditionsPair
			}
			got, got1, got2, got3, err := s.matchResAndSDKListenerRules(tt.args.resLRs, tt.args.sdkLRs, resLRDesiredActionsAndConditionsPairs)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
				assert.Equal(t, tt.want1, got1)
				assert.Equal(t, tt.want2, got2)
				assert.Equal(t, tt.want3, got3)
			}
		})
	}
}
