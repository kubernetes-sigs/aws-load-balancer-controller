package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_buildSDKSetRulePrioritiesInput(t *testing.T) {
	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
	type args struct {
		matchedResAndSDKLRsBySettings []resAndSDKListenerRulePair
		unmatchedSDKLRs               []ListenerRuleWithTags
	}
	tests := []struct {
		name string
		args args
		want *elbv2sdk.SetRulePrioritiesInput
	}{
		{
			name: "Only re-prioritize matched rules by settings",
			args: args{
				matchedResAndSDKLRsBySettings: []resAndSDKListenerRulePair{
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
				unmatchedSDKLRs: []ListenerRuleWithTags{},
			},
			want: &elbv2sdk.SetRulePrioritiesInput{
				RulePriorities: []elbv2types.RulePriorityPair{
					{
						RuleArn:  awssdk.String("arn-1"),
						Priority: awssdk.Int32(3),
					},
					{
						RuleArn:  awssdk.String("arn-3"),
						Priority: awssdk.Int32(1),
					},
				},
			},
		},
		{
			name: "push down unmatched sdk rules in order",
			args: args{
				matchedResAndSDKLRsBySettings: []resAndSDKListenerRulePair{},
				unmatchedSDKLRs: []ListenerRuleWithTags{
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
			},
			want: &elbv2sdk.SetRulePrioritiesInput{
				RulePriorities: []elbv2types.RulePriorityPair{
					{
						RuleArn:  awssdk.String("arn-3"),
						Priority: awssdk.Int32(50000),
					},
					{
						RuleArn:  awssdk.String("arn-1"),
						Priority: awssdk.Int32(49999),
					},
				},
			},
		},
		{
			name: "Re-prioritize matched rules by settings and also push down unmatched sdk rules in order",
			args: args{
				matchedResAndSDKLRsBySettings: []resAndSDKListenerRulePair{
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
				unmatchedSDKLRs: []ListenerRuleWithTags{
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-35"),
							Priority: awssdk.String("35"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo35-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo35"},
									},
								},
							},
						},
					},
					{
						ListenerRule: &elbv2types.Rule{
							RuleArn:  awssdk.String("arn-16"),
							Priority: awssdk.String("16"),
							Actions: []elbv2types.Action{
								{
									Type: elbv2types.ActionTypeEnumForward,
									ForwardConfig: &elbv2types.ForwardActionConfig{
										TargetGroups: []elbv2types.TargetGroupTuple{
											{
												TargetGroupArn: awssdk.String("foo16-tg"),
											},
										},
									},
								},
							},
							Conditions: []elbv2types.RuleCondition{
								{
									Field: awssdk.String("path-pattern"),
									PathPatternConfig: &elbv2types.PathPatternConditionConfig{
										Values: []string{"/foo16"},
									},
								},
							},
						},
					},
				},
			},
			want: &elbv2sdk.SetRulePrioritiesInput{
				RulePriorities: []elbv2types.RulePriorityPair{
					{
						RuleArn:  awssdk.String("arn-35"),
						Priority: awssdk.Int32(50000),
					},
					{
						RuleArn:  awssdk.String("arn-16"),
						Priority: awssdk.Int32(49999),
					},
					{
						RuleArn:  awssdk.String("arn-1"),
						Priority: awssdk.Int32(3),
					},
					{
						RuleArn:  awssdk.String("arn-3"),
						Priority: awssdk.Int32(1),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			got := buildSDKSetRulePrioritiesInput(tt.args.matchedResAndSDKLRsBySettings, tt.args.unmatchedSDKLRs)
			assert.Equal(t, tt.want, got)
		})
	}
}
