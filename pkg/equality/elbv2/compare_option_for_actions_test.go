package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCompareOptionForRedirectActionConfig(t *testing.T) {
	type args struct {
		lhs *elbv2types.RedirectActionConfig
		rhs *elbv2types.RedirectActionConfig
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "equals for all fields",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: true,
		},
		{
			name: "host not equals",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("app.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "host not equals with #{host}",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("#{host}"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "nil host equals with #{host}",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("#{host}"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       nil,
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: true,
		},
		{
			name: "path not equals",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/app"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "path not equals with /#{path}",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/#{path}"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "nil path equals with /#{path}",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/#{path}"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       nil,
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: true,
		},
		{
			name: "port not equals",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("443"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "port not equals with #{port}",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("#{port}"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "nil port equals with #{port}",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("#{port}"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       nil,
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: true,
		},
		{
			name: "protocol not equals",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTP"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "protocol not equals with #{protocol}",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("#{protocol}"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "nil protocol equals with #{protocol}",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("#{protocol}"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   nil,
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: true,
		},
		{
			name: "query not equals",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=c"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "query not equals with #{query}",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("#{query}"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "nil query equals with #{query}",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("#{query}"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      nil,
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
			},
			want: true,
		},
		{
			name: "statusCode not equals",
			args: args{
				lhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_301"),
				},
				rhs: &elbv2types.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: elbv2types.RedirectActionStatusCodeEnum("HTTP_302"),
				},
			},
			want: false,
		},
		{
			name: "two nil RedirectActionConfig equals",
			args: args{
				lhs: nil,
				rhs: nil,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := CompareOptionForRedirectActionConfig()
			got := cmp.Equal(tt.args.lhs, tt.args.rhs, opts)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompareOptionForAction(t *testing.T) {
	type args struct {
		lhs elbv2types.Action
		rhs elbv2types.Action
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "two actions equals exactly",
			args: args{
				lhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
				},
				rhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "two actions are not equal if some fields un-equal",
			args: args{
				lhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
				},
				rhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-b"),
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "two actions are equal irrelevant of their order existence",
			args: args{
				lhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
				},
				rhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
					Order: awssdk.Int32(1),
				},
			},
			want: true,
		},
		{
			name: "two actions are equal irrelevant of their order value",
			args: args{
				lhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
					Order: awssdk.Int32(1),
				},
				rhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
					Order: awssdk.Int32(2),
				},
			},
			want: true,
		},
		{
			name: "two action are equal irrelevant of their targetGroupARN existence",
			args: args{
				lhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
				},
				rhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
					TargetGroupArn: awssdk.String("tg-a"),
				},
			},
			want: true,
		},
		{
			name: "actions with weighted target groups equal regardless of order",
			args: args{
				lhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
								Weight:         awssdk.Int32(60),
							},
							{
								TargetGroupArn: awssdk.String("tg-b"),
								Weight:         awssdk.Int32(40),
							},
						},
					},
				},
				rhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-b"),
								Weight:         awssdk.Int32(40),
							},
							{
								TargetGroupArn: awssdk.String("tg-a"),
								Weight:         awssdk.Int32(60),
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "actions with weighted target groups not equal when weights differ",
			args: args{
				lhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
								Weight:         awssdk.Int32(60),
							},
							{
								TargetGroupArn: awssdk.String("tg-b"),
								Weight:         awssdk.Int32(40),
							},
						},
					},
				},
				rhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
								Weight:         awssdk.Int32(30),
							},
							{
								TargetGroupArn: awssdk.String("tg-b"),
								Weight:         awssdk.Int32(70),
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "actions with three weighted target groups equal regardless of order",
			args: args{
				lhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
								Weight:         awssdk.Int32(50),
							},
							{
								TargetGroupArn: awssdk.String("tg-b"),
								Weight:         awssdk.Int32(30),
							},
							{
								TargetGroupArn: awssdk.String("tg-c"),
								Weight:         awssdk.Int32(20),
							},
						},
					},
				},
				rhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-c"),
								Weight:         awssdk.Int32(20),
							},
							{
								TargetGroupArn: awssdk.String("tg-a"),
								Weight:         awssdk.Int32(50),
							},
							{
								TargetGroupArn: awssdk.String("tg-b"),
								Weight:         awssdk.Int32(30),
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "actions with mixed zero and non-zero weights equal regardless of order",
			args: args{
				lhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
								Weight:         awssdk.Int32(80),
							},
							{
								TargetGroupArn: awssdk.String("tg-b"),
								Weight:         awssdk.Int32(20),
							},
							{
								TargetGroupArn: awssdk.String("tg-c"),
								Weight:         awssdk.Int32(0),
							},
						},
					},
					Order: awssdk.Int32(1),
				},
				rhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-c"),
								Weight:         awssdk.Int32(0),
							},
							{
								TargetGroupArn: awssdk.String("tg-b"),
								Weight:         awssdk.Int32(20),
							},
							{
								TargetGroupArn: awssdk.String("tg-a"),
								Weight:         awssdk.Int32(80),
							},
						},
					},
					Order:          awssdk.Int32(2),       // Different order should be ignored
					TargetGroupArn: awssdk.String("tg-a"), // Should be ignored
				},
			},
			want: true,
		},
		{
			name: "actions with three target groups on lhs not equal to two target groups on rhs",
			args: args{
				lhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
								Weight:         awssdk.Int32(50),
							},
							{
								TargetGroupArn: awssdk.String("tg-b"),
								Weight:         awssdk.Int32(30),
							},
							{
								TargetGroupArn: awssdk.String("tg-c"),
								Weight:         awssdk.Int32(20),
							},
						},
					},
				},
				rhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
								Weight:         awssdk.Int32(70),
							},
							{
								TargetGroupArn: awssdk.String("tg-b"),
								Weight:         awssdk.Int32(30),
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "actions with two target groups on lhs not equal to three target groups on rhs",
			args: args{
				lhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
								Weight:         awssdk.Int32(60),
							},
							{
								TargetGroupArn: awssdk.String("tg-b"),
								Weight:         awssdk.Int32(40),
							},
						},
					},
				},
				rhs: elbv2types.Action{
					Type: elbv2types.ActionTypeEnum("forward"),
					ForwardConfig: &elbv2types.ForwardActionConfig{
						TargetGroups: []elbv2types.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
								Weight:         awssdk.Int32(40),
							},
							{
								TargetGroupArn: awssdk.String("tg-b"),
								Weight:         awssdk.Int32(30),
							},
							{
								TargetGroupArn: awssdk.String("tg-c"),
								Weight:         awssdk.Int32(30),
							},
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cmp.Equal(tt.args.lhs, tt.args.rhs, CompareOptionForAction())
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompareOptionForForwardActionConfig(t *testing.T) {
	type args struct {
		lhs *elbv2types.ForwardActionConfig
		rhs *elbv2types.ForwardActionConfig
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "two nil ForwardActionConfig equals",
			args: args{
				lhs: nil,
				rhs: nil,
			},
			want: true,
		},
		{
			name: "ForwardActionConfig equals with same target groups",
			args: args{
				lhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
						},
					},
				},
				rhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "ForwardActionConfig with weighted target groups equal regardless of order",
			args: args{
				lhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
							Weight:         awssdk.Int32(70),
						},
						{
							TargetGroupArn: awssdk.String("tg-b"),
							Weight:         awssdk.Int32(30),
						},
					},
				},
				rhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-b"),
							Weight:         awssdk.Int32(30),
						},
						{
							TargetGroupArn: awssdk.String("tg-a"),
							Weight:         awssdk.Int32(70),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "ForwardActionConfig not equal when target groups differ",
			args: args{
				lhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
							Weight:         awssdk.Int32(100),
						},
					},
				},
				rhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-b"),
							Weight:         awssdk.Int32(100),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "ForwardActionConfig not equal when weights differ",
			args: args{
				lhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
							Weight:         awssdk.Int32(80),
						},
						{
							TargetGroupArn: awssdk.String("tg-b"),
							Weight:         awssdk.Int32(20),
						},
					},
				},
				rhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
							Weight:         awssdk.Int32(50),
						},
						{
							TargetGroupArn: awssdk.String("tg-b"),
							Weight:         awssdk.Int32(50),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "ForwardActionConfig with single target group weight normalization",
			args: args{
				lhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
						},
					},
				},
				rhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
						},
					},
				},
			},
			want: true, // Single target group weights should be normalized
		},
		{
			name: "ForwardActionConfig with TargetGroupStickinessConfig",
			args: args{
				lhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
						},
					},
					TargetGroupStickinessConfig: &elbv2types.TargetGroupStickinessConfig{
						Enabled:         awssdk.Bool(true),
						DurationSeconds: awssdk.Int32(3600),
					},
				},
				rhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
						},
					},
					TargetGroupStickinessConfig: &elbv2types.TargetGroupStickinessConfig{
						Enabled:         awssdk.Bool(true),
						DurationSeconds: awssdk.Int32(3600),
					},
				},
			},
			want: true,
		},
		{
			name: "ForwardActionConfig with different TargetGroupStickinessConfig",
			args: args{
				lhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
							Weight:         awssdk.Int32(100),
						},
					},
					TargetGroupStickinessConfig: &elbv2types.TargetGroupStickinessConfig{
						Enabled:         awssdk.Bool(true),
						DurationSeconds: awssdk.Int32(3600),
					},
				},
				rhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
							Weight:         awssdk.Int32(100),
						},
					},
					TargetGroupStickinessConfig: &elbv2types.TargetGroupStickinessConfig{
						Enabled:         awssdk.Bool(false),
						DurationSeconds: awssdk.Int32(1800),
					},
				},
			},
			want: false,
		},
		{
			name: "ForwardActionConfig with nil vs non-nil TargetGroupStickinessConfig should be equal (IgnoreLeftHandUnset)",
			args: args{
				lhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
							Weight:         awssdk.Int32(100),
						},
					},
					TargetGroupStickinessConfig: nil,
				},
				rhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
							Weight:         awssdk.Int32(100),
						},
					},
					TargetGroupStickinessConfig: &elbv2types.TargetGroupStickinessConfig{
						Enabled:         awssdk.Bool(true),
						DurationSeconds: awssdk.Int32(3600),
					},
				},
			},
			want: true, // IgnoreLeftHandUnset should make this equal
		},
		{
			name: "ForwardActionConfig with complex weighted target groups",
			args: args{
				lhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-a"),
							Weight:         awssdk.Int32(40),
						},
						{
							TargetGroupArn: awssdk.String("tg-b"),
							Weight:         awssdk.Int32(35),
						},
						{
							TargetGroupArn: awssdk.String("tg-c"),
							Weight:         awssdk.Int32(25),
						},
					},
					TargetGroupStickinessConfig: &elbv2types.TargetGroupStickinessConfig{
						Enabled:         awssdk.Bool(true),
						DurationSeconds: awssdk.Int32(7200),
					},
				},
				rhs: &elbv2types.ForwardActionConfig{
					TargetGroups: []elbv2types.TargetGroupTuple{
						{
							TargetGroupArn: awssdk.String("tg-c"),
							Weight:         awssdk.Int32(25),
						},
						{
							TargetGroupArn: awssdk.String("tg-a"),
							Weight:         awssdk.Int32(40),
						},
						{
							TargetGroupArn: awssdk.String("tg-b"),
							Weight:         awssdk.Int32(35),
						},
					},
					TargetGroupStickinessConfig: &elbv2types.TargetGroupStickinessConfig{
						Enabled:         awssdk.Bool(true),
						DurationSeconds: awssdk.Int32(7200),
					},
				},
			},
			want: true, // Should be equal after sorting by ARN
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cmp.Equal(tt.args.lhs, tt.args.rhs, CompareOptionForForwardActionConfig())
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompareOptionForTargetGroupTuples(t *testing.T) {
	type args struct {
		lhs []elbv2types.TargetGroupTuple
		rhs []elbv2types.TargetGroupTuple
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "empty slices are equal",
			args: args{
				lhs: []elbv2types.TargetGroupTuple{},
				rhs: []elbv2types.TargetGroupTuple{},
			},
			want: true,
		},
		{
			name: "single target group with weight normalization",
			args: args{
				lhs: []elbv2types.TargetGroupTuple{
					{
						TargetGroupArn: awssdk.String("tg-a"),
					},
				},
				rhs: []elbv2types.TargetGroupTuple{
					{
						TargetGroupArn: awssdk.String("tg-a"),
					},
				},
			},
			want: true, // Weights should be normalized to nil for single target groups
		},
		{
			name: "multiple target groups equal regardless of order",
			args: args{
				lhs: []elbv2types.TargetGroupTuple{
					{
						TargetGroupArn: awssdk.String("tg-a"),
						Weight:         awssdk.Int32(100),
					},
					{
						TargetGroupArn: awssdk.String("tg-b"),
						Weight:         awssdk.Int32(0),
					},
				},
				rhs: []elbv2types.TargetGroupTuple{
					{
						TargetGroupArn: awssdk.String("tg-b"),
						Weight:         awssdk.Int32(0),
					},
					{
						TargetGroupArn: awssdk.String("tg-a"),
						Weight:         awssdk.Int32(100),
					},
				},
			},
			want: true,
		},
		{
			name: "multiple target groups with different weights are not equal",
			args: args{
				lhs: []elbv2types.TargetGroupTuple{
					{
						TargetGroupArn: awssdk.String("tg-a"),
						Weight:         awssdk.Int32(100),
					},
					{
						TargetGroupArn: awssdk.String("tg-b"),
						Weight:         awssdk.Int32(0),
					},
				},
				rhs: []elbv2types.TargetGroupTuple{
					{
						TargetGroupArn: awssdk.String("tg-a"),
						Weight:         awssdk.Int32(50),
					},
					{
						TargetGroupArn: awssdk.String("tg-b"),
						Weight:         awssdk.Int32(50),
					},
				},
			},
			want: false,
		},
		{
			name: "multiple target groups with same weights but different ARNs",
			args: args{
				lhs: []elbv2types.TargetGroupTuple{
					{
						TargetGroupArn: awssdk.String("tg-a"),
						Weight:         awssdk.Int32(50),
					},
					{
						TargetGroupArn: awssdk.String("tg-b"),
						Weight:         awssdk.Int32(50),
					},
				},
				rhs: []elbv2types.TargetGroupTuple{
					{
						TargetGroupArn: awssdk.String("tg-b"),
						Weight:         awssdk.Int32(50),
					},
					{
						TargetGroupArn: awssdk.String("tg-a"),
						Weight:         awssdk.Int32(50),
					},
				},
			},
			want: true, // Should be equal after sorting by ARN
		},
		{
			name: "target groups with nil ARNs",
			args: args{
				lhs: []elbv2types.TargetGroupTuple{
					{
						TargetGroupArn: nil,
						Weight:         awssdk.Int32(100),
					},
					{
						TargetGroupArn: awssdk.String("tg-a"),
						Weight:         awssdk.Int32(0),
					},
				},
				rhs: []elbv2types.TargetGroupTuple{
					{
						TargetGroupArn: awssdk.String("tg-a"),
						Weight:         awssdk.Int32(0),
					},
					{
						TargetGroupArn: nil,
						Weight:         awssdk.Int32(100),
					},
				},
			},
			want: true, // Should be equal after sorting (nil ARN becomes empty string, sorts first)
		},
		{
			name: "target groups with nil weights",
			args: args{
				lhs: []elbv2types.TargetGroupTuple{
					{
						TargetGroupArn: awssdk.String("tg-a"),
						Weight:         nil,
					},
					{
						TargetGroupArn: awssdk.String("tg-b"),
						Weight:         awssdk.Int32(100),
					},
				},
				rhs: []elbv2types.TargetGroupTuple{
					{
						TargetGroupArn: awssdk.String("tg-b"),
						Weight:         awssdk.Int32(100),
					},
					{
						TargetGroupArn: awssdk.String("tg-a"),
						Weight:         nil,
					},
				},
			},
			want: true, // Should be equal after sorting by ARN
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cmp.Equal(tt.args.lhs, tt.args.rhs, CompareOptionForTargetGroupTuples())
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompareOptionForActions(t *testing.T) {
	type args struct {
		lhs []elbv2types.Action
		rhs []elbv2types.Action
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "two actions slice equals exactly",
			args: args{
				lhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2types.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int32(1),
					},
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int32(2),
					},
				},
				rhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2types.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int32(1),
					},
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int32(2),
					},
				},
			},
			want: true,
		},
		{
			name: "two actions slice are not equal if there actions are not equal",
			args: args{
				lhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2types.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int32(1),
					},
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int32(2),
					},
				},
				rhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2types.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-b"),
							UserPoolClientId: awssdk.String("pool-client-id-b"),
							UserPoolDomain:   awssdk.String("pool-domain-b"),
						},
						Order: awssdk.Int32(1),
					},
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int32(2),
					},
				},
			},
			want: false,
		},
		{
			name: "two actions slice equals when they are equal after sorted by order",
			args: args{
				lhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2types.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int32(1),
					},
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int32(2),
					},
				},
				rhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int32(4),
					},
					{
						Type: elbv2types.ActionTypeEnum("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2types.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int32(3),
					},
				},
			},
			want: true,
		},
		{
			name: "two actions slice are not equals when they are not equal after sorted by order",
			args: args{
				lhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2types.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int32(1),
					},
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int32(2),
					},
				},
				rhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int32(3),
					},
					{
						Type: elbv2types.ActionTypeEnum("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2types.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int32(4),
					},
				},
			},
			want: false,
		},
		{
			name: "actions with weighted target groups equal regardless of target group order",
			args: args{
				lhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
									Weight:         awssdk.Int32(80),
								},
								{
									TargetGroupArn: awssdk.String("tg-b"),
									Weight:         awssdk.Int32(20),
								},
							},
						},
						Order: awssdk.Int32(1),
					},
				},
				rhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-b"),
									Weight:         awssdk.Int32(20),
								},
								{
									TargetGroupArn: awssdk.String("tg-a"),
									Weight:         awssdk.Int32(80),
								},
							},
						},
						Order: awssdk.Int32(1),
					},
				},
			},
			want: true,
		},
		{
			name: "actions with weighted target groups not equal when weights differ",
			args: args{
				lhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
									Weight:         awssdk.Int32(80),
								},
								{
									TargetGroupArn: awssdk.String("tg-b"),
									Weight:         awssdk.Int32(20),
								},
							},
						},
						Order: awssdk.Int32(1),
					},
				},
				rhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
									Weight:         awssdk.Int32(50),
								},
								{
									TargetGroupArn: awssdk.String("tg-b"),
									Weight:         awssdk.Int32(50),
								},
							},
						},
						Order: awssdk.Int32(1),
					},
				},
			},
			want: false,
		},
		{
			name: "multiple actions with weighted target groups equal when sorted by order",
			args: args{
				lhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2types.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int32(1),
					},
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
									Weight:         awssdk.Int32(70),
								},
								{
									TargetGroupArn: awssdk.String("tg-b"),
									Weight:         awssdk.Int32(30),
								},
							},
						},
						Order: awssdk.Int32(2),
					},
				},
				rhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-b"),
									Weight:         awssdk.Int32(30),
								},
								{
									TargetGroupArn: awssdk.String("tg-a"),
									Weight:         awssdk.Int32(70),
								},
							},
						},
						Order: awssdk.Int32(4),
					},
					{
						Type: elbv2types.ActionTypeEnum("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2types.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int32(3),
					},
				},
			},
			want: true,
		},
		{
			name: "actions with zero-weight target groups equal regardless of order",
			args: args{
				lhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
									Weight:         awssdk.Int32(100),
								},
								{
									TargetGroupArn: awssdk.String("tg-b"),
									Weight:         awssdk.Int32(0),
								},
								{
									TargetGroupArn: awssdk.String("tg-c"),
									Weight:         awssdk.Int32(0),
								},
							},
						},
						Order: awssdk.Int32(1),
					},
				},
				rhs: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("forward"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-c"),
									Weight:         awssdk.Int32(0),
								},
								{
									TargetGroupArn: awssdk.String("tg-a"),
									Weight:         awssdk.Int32(100),
								},
								{
									TargetGroupArn: awssdk.String("tg-b"),
									Weight:         awssdk.Int32(0),
								},
							},
						},
						Order:          awssdk.Int32(1),
						TargetGroupArn: awssdk.String("tg-a"), // Should be ignored
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cmp.Equal(tt.args.lhs, tt.args.rhs, CompareOptionForActions(nil, nil))
			assert.Equal(t, tt.want, got)
		})
	}
}
