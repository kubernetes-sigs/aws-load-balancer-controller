package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/smithy-go"
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
			name: "all listener rules settings including transforms has match but not priority",
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
							Transforms: []elbv2model.Transform{
								{
									Type: elbv2model.TransformTypeUrlRewrite,
									UrlRewriteConfig: &elbv2model.RewriteConfigObject{
										Rewrites: []elbv2model.RewriteConfig{
											{
												Regex:   "/path1/(.*)",
												Replace: "/newpath1/$1",
											},
										},
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
							Transforms: []elbv2model.Transform{
								{
									Type: elbv2model.TransformTypeUrlRewrite,
									UrlRewriteConfig: &elbv2model.RewriteConfigObject{
										Rewrites: []elbv2model.RewriteConfig{
											{
												Regex:   "/path2/(.*)",
												Replace: "/newpath2/$1",
											},
										},
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
							Transforms: []elbv2model.Transform{
								{
									Type: elbv2model.TransformTypeUrlRewrite,
									UrlRewriteConfig: &elbv2model.RewriteConfigObject{
										Rewrites: []elbv2model.RewriteConfig{
											{
												Regex:   "/path3/(.*)",
												Replace: "/newpath3/$1",
											},
										},
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
							Transforms: []elbv2types.RuleTransform{
								{
									Type: elbv2types.TransformTypeEnumUrlRewrite,
									UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
										Rewrites: []elbv2types.RewriteConfig{
											{
												Regex:   awssdk.String("/path1/(.*)"),
												Replace: awssdk.String("/newpath1/$1"),
											},
										},
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
							Transforms: []elbv2types.RuleTransform{
								{
									Type: elbv2types.TransformTypeEnumUrlRewrite,
									UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
										Rewrites: []elbv2types.RewriteConfig{
											{
												Regex:   awssdk.String("/path2/(.*)"),
												Replace: awssdk.String("/newpath2/$1"),
											},
										},
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
							Transforms: []elbv2types.RuleTransform{
								{
									Type: elbv2types.TransformTypeEnumUrlRewrite,
									UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
										Rewrites: []elbv2types.RewriteConfig{
											{
												Regex:   awssdk.String("/path3/(.*)"),
												Replace: awssdk.String("/newpath3/$1"),
											},
										},
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
							Transforms: []elbv2model.Transform{
								{
									Type: elbv2model.TransformTypeUrlRewrite,
									UrlRewriteConfig: &elbv2model.RewriteConfigObject{
										Rewrites: []elbv2model.RewriteConfig{
											{
												Regex:   "/path1/(.*)",
												Replace: "/newpath1/$1",
											},
										},
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
							Transforms: []elbv2types.RuleTransform{
								{
									Type: elbv2types.TransformTypeEnumUrlRewrite,
									UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
										Rewrites: []elbv2types.RewriteConfig{
											{
												Regex:   awssdk.String("/path1/(.*)"),
												Replace: awssdk.String("/newpath1/$1"),
											},
										},
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
							Transforms: []elbv2model.Transform{
								{
									Type: elbv2model.TransformTypeUrlRewrite,
									UrlRewriteConfig: &elbv2model.RewriteConfigObject{
										Rewrites: []elbv2model.RewriteConfig{
											{
												Regex:   "/path3/(.*)",
												Replace: "/newpath3/$1",
											},
										},
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
							Transforms: []elbv2types.RuleTransform{
								{
									Type: elbv2types.TransformTypeEnumUrlRewrite,
									UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
										Rewrites: []elbv2types.RewriteConfig{
											{
												Regex:   awssdk.String("/path3/(.*)"),
												Replace: awssdk.String("/newpath3/$1"),
											},
										},
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
			resLRDesiredRuleConfigs := make(map[*elbv2model.ListenerRule]*resLRDesiredRuleConfig, len(tt.args.resLRs))
			for _, resLR := range tt.args.resLRs {
				resLRDesiredRuleConfig, _ := buildResLRDesiredRuleConfig(resLR, featureGates)
				resLRDesiredRuleConfigs[resLR] = resLRDesiredRuleConfig
			}
			got, got1, got2, got3, err := s.matchResAndSDKListenerRules(tt.args.resLRs, tt.args.sdkLRs, resLRDesiredRuleConfigs)
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

type mockLROperation int

const (
	createLR mockLROperation = iota
	deleteLR
)

type mockLRCall struct {
	arn string
	op  mockLROperation
}

func Test_CreateAndDeleteRules(t *testing.T) {
	testCases := []struct {
		name             string
		initialRuleCount int
		maxRuleCount     int
		unmatchedResLRs  []*elbv2model.ListenerRule
		unmatchedSDKLRs  []ListenerRuleWithTags
		expectErr        bool
		expectedCalls    []mockLRCall
	}{
		{
			name: "no rules",
		},
		{
			name:             "just creation",
			initialRuleCount: 0,
			maxRuleCount:     100,
			unmatchedResLRs: []*elbv2model.ListenerRule{
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-1"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-2"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-3"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-4"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-5"),
					},
				},
			},
			expectedCalls: []mockLRCall{{arn: "arn-1", op: createLR}, {arn: "arn-2", op: createLR}, {arn: "arn-3", op: createLR}, {arn: "arn-4", op: createLR}, {arn: "arn-5", op: createLR}},
		},
		{
			name:             "just deletes",
			initialRuleCount: 0,
			maxRuleCount:     100,
			unmatchedSDKLRs: []ListenerRuleWithTags{
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-1"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-2"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-3"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-4"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-5"),
					},
				},
			},
			expectedCalls: []mockLRCall{{arn: "arn-1", op: deleteLR}, {arn: "arn-2", op: deleteLR}, {arn: "arn-3", op: deleteLR}, {arn: "arn-4", op: deleteLR}, {arn: "arn-5", op: deleteLR}},
		},
		{
			name:             "just creation -- at limit",
			initialRuleCount: 2,
			maxRuleCount:     2,
			unmatchedResLRs: []*elbv2model.ListenerRule{
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-1"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-2"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-3"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-4"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-5"),
					},
				},
			},
			expectErr: true,
		},
		{
			name:             "just deletes -- at max limit",
			initialRuleCount: 2,
			maxRuleCount:     2,
			unmatchedSDKLRs: []ListenerRuleWithTags{
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-1"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-2"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-3"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-4"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-5"),
					},
				},
			},
			expectedCalls: []mockLRCall{{arn: "arn-1", op: deleteLR}, {arn: "arn-2", op: deleteLR}, {arn: "arn-3", op: deleteLR}, {arn: "arn-4", op: deleteLR}, {arn: "arn-5", op: deleteLR}},
		},
		{
			name:             "mix of deletes and creates",
			initialRuleCount: 0,
			maxRuleCount:     100,
			unmatchedSDKLRs: []ListenerRuleWithTags{
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-1"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-2"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-3"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-4"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-5"),
					},
				},
			},
			unmatchedResLRs: []*elbv2model.ListenerRule{
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-6"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-7"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-8"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-9"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-10"),
					},
				},
			},
			expectedCalls: []mockLRCall{{arn: "arn-6", op: createLR}, {arn: "arn-7", op: createLR}, {arn: "arn-8", op: createLR}, {arn: "arn-9", op: createLR}, {arn: "arn-10", op: createLR}, {arn: "arn-1", op: deleteLR}, {arn: "arn-2", op: deleteLR}, {arn: "arn-3", op: deleteLR}, {arn: "arn-4", op: deleteLR}, {arn: "arn-5", op: deleteLR}},
		},
		{
			name:             "mix of deletes and creates -- already at limit",
			initialRuleCount: 100,
			maxRuleCount:     100,
			unmatchedSDKLRs: []ListenerRuleWithTags{
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-1"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-2"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-3"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-4"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-5"),
					},
				},
			},
			unmatchedResLRs: []*elbv2model.ListenerRule{
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-6"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-7"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-8"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-9"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-10"),
					},
				},
			},
			expectedCalls: []mockLRCall{{arn: "arn-1", op: deleteLR}, {arn: "arn-6", op: createLR}, {arn: "arn-2", op: deleteLR}, {arn: "arn-7", op: createLR}, {arn: "arn-3", op: deleteLR}, {arn: "arn-8", op: createLR}, {arn: "arn-4", op: deleteLR}, {arn: "arn-9", op: createLR}, {arn: "arn-5", op: deleteLR}, {arn: "arn-10", op: createLR}},
		},
		{
			name:             "mix of deletes and creates -- limit reached part way between creations",
			initialRuleCount: 97,
			maxRuleCount:     100,
			unmatchedSDKLRs: []ListenerRuleWithTags{
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-1"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-2"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-3"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-4"),
					},
				},
				{
					ListenerRule: &elbv2types.Rule{
						RuleArn: awssdk.String("arn-5"),
					},
				},
			},
			unmatchedResLRs: []*elbv2model.ListenerRule{
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-6"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-7"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-8"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-9"),
					},
				},
				{
					Spec: elbv2model.ListenerRuleSpec{
						ListenerARN: core.LiteralStringToken("arn-10"),
					},
				},
			},
			expectedCalls: []mockLRCall{{arn: "arn-6", op: createLR}, {arn: "arn-7", op: createLR}, {arn: "arn-8", op: createLR}, {arn: "arn-1", op: deleteLR}, {arn: "arn-9", op: createLR}, {arn: "arn-2", op: deleteLR}, {arn: "arn-10", op: createLR}, {arn: "arn-3", op: deleteLR}, {arn: "arn-4", op: deleteLR}, {arn: "arn-5", op: deleteLR}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mLRManager := &mockListenerRuleManager{
				ruleCount:    tc.initialRuleCount,
				maxRuleCount: tc.maxRuleCount,
			}
			s := &listenerRuleSynthesizer{
				lrManager: mLRManager,
			}

			err := s.createAndDeleteRules(context.Background(), tc.initialRuleCount, map[*elbv2model.ListenerRule]*resLRDesiredRuleConfig{}, tc.unmatchedResLRs, tc.unmatchedSDKLRs)

			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedCalls, mLRManager.calls)
			}
		})
	}
}

type mockListenerRuleManager struct {
	ruleCount    int
	maxRuleCount int
	createErr    error
	deleteErr    error
	calls        []mockLRCall
}

func (m *mockListenerRuleManager) Create(ctx context.Context, resLR *elbv2model.ListenerRule, desiredActionsAndConditions *resLRDesiredRuleConfig) (elbv2model.ListenerRuleStatus, error) {
	if m.ruleCount == m.maxRuleCount {
		return elbv2model.ListenerRuleStatus{}, &smithy.GenericAPIError{Code: "TooManyRules", Message: "some message"}
	}
	arn, _ := resLR.Spec.ListenerARN.Resolve(ctx)
	m.calls = append(m.calls, mockLRCall{
		arn: arn,
		op:  createLR,
	})
	m.ruleCount++
	return elbv2model.ListenerRuleStatus{}, m.createErr
}

func (m *mockListenerRuleManager) Delete(ctx context.Context, sdkLR ListenerRuleWithTags) error {
	m.calls = append(m.calls, mockLRCall{
		arn: *sdkLR.ListenerRule.RuleArn,
		op:  deleteLR,
	})
	m.ruleCount--
	return m.deleteErr
}

func (m *mockListenerRuleManager) UpdateRules(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR ListenerRuleWithTags, desiredActionsAndConditions *resLRDesiredRuleConfig) (elbv2model.ListenerRuleStatus, error) {
	//TODO implement me
	panic("implement me")
}

func (m *mockListenerRuleManager) UpdateRulesTags(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR ListenerRuleWithTags) (elbv2model.ListenerRuleStatus, error) {
	//TODO implement me
	panic("implement me")
}

func (m *mockListenerRuleManager) SetRulePriorities(ctx context.Context, matchedResAndSDKLRsBySettings []resAndSDKListenerRulePair, unmatchedSDKLRs []ListenerRuleWithTags) error {
	//TODO implement me
	panic("implement me")
}
