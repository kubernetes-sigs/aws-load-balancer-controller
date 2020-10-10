package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCompareOptionForRedirectActionConfig(t *testing.T) {
	type args struct {
		lhs *elbv2sdk.RedirectActionConfig
		rhs *elbv2sdk.RedirectActionConfig
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "equals for all fields",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: true,
		},
		{
			name: "host not equals",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("app.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "host not equals with #{host}",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("#{host}"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "nil host equals with #{host}",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("#{host}"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       nil,
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: true,
		},
		{
			name: "path not equals",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/app"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "path not equals with /#{path}",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/#{path}"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "nil path equals with /#{path}",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/#{path}"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       nil,
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: true,
		},
		{
			name: "port not equals",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("443"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "port not equals with #{port}",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("#{port}"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "nil port equals with #{port}",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("#{port}"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       nil,
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: true,
		},
		{
			name: "protocol not equals",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTP"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "protocol not equals with #{protocol}",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("#{protocol}"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "nil protocol equals with #{protocol}",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("#{protocol}"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   nil,
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: true,
		},
		{
			name: "query not equals",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=c"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "query not equals with #{query}",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("#{query}"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: false,
		},
		{
			name: "nil query equals with #{query}",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("#{query}"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      nil,
					StatusCode: awssdk.String("HTTP_301"),
				},
			},
			want: true,
		},
		{
			name: "statusCode not equals",
			args: args{
				lhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_301"),
				},
				rhs: &elbv2sdk.RedirectActionConfig{
					Host:       awssdk.String("www.example.com"),
					Path:       awssdk.String("/home"),
					Port:       awssdk.String("80"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("a=b"),
					StatusCode: awssdk.String("HTTP_302"),
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
		lhs elbv2sdk.Action
		rhs elbv2sdk.Action
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "two actions equals exactly",
			args: args{
				lhs: elbv2sdk.Action{
					Type: awssdk.String("forward"),
					ForwardConfig: &elbv2sdk.ForwardActionConfig{
						TargetGroups: []*elbv2sdk.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
				},
				rhs: elbv2sdk.Action{
					Type: awssdk.String("forward"),
					ForwardConfig: &elbv2sdk.ForwardActionConfig{
						TargetGroups: []*elbv2sdk.TargetGroupTuple{
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
				lhs: elbv2sdk.Action{
					Type: awssdk.String("forward"),
					ForwardConfig: &elbv2sdk.ForwardActionConfig{
						TargetGroups: []*elbv2sdk.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
				},
				rhs: elbv2sdk.Action{
					Type: awssdk.String("forward"),
					ForwardConfig: &elbv2sdk.ForwardActionConfig{
						TargetGroups: []*elbv2sdk.TargetGroupTuple{
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
				lhs: elbv2sdk.Action{
					Type: awssdk.String("forward"),
					ForwardConfig: &elbv2sdk.ForwardActionConfig{
						TargetGroups: []*elbv2sdk.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
				},
				rhs: elbv2sdk.Action{
					Type: awssdk.String("forward"),
					ForwardConfig: &elbv2sdk.ForwardActionConfig{
						TargetGroups: []*elbv2sdk.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
					Order: awssdk.Int64(1),
				},
			},
			want: true,
		},
		{
			name: "two actions are equal irrelevant of their order value",
			args: args{
				lhs: elbv2sdk.Action{
					Type: awssdk.String("forward"),
					ForwardConfig: &elbv2sdk.ForwardActionConfig{
						TargetGroups: []*elbv2sdk.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
					Order: awssdk.Int64(1),
				},
				rhs: elbv2sdk.Action{
					Type: awssdk.String("forward"),
					ForwardConfig: &elbv2sdk.ForwardActionConfig{
						TargetGroups: []*elbv2sdk.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
					Order: awssdk.Int64(2),
				},
			},
			want: true,
		},
		{
			name: "two action are equal irrelevant of their targetGroupARN existence",
			args: args{
				lhs: elbv2sdk.Action{
					Type: awssdk.String("forward"),
					ForwardConfig: &elbv2sdk.ForwardActionConfig{
						TargetGroups: []*elbv2sdk.TargetGroupTuple{
							{
								TargetGroupArn: awssdk.String("tg-a"),
							},
						},
					},
				},
				rhs: elbv2sdk.Action{
					Type: awssdk.String("forward"),
					ForwardConfig: &elbv2sdk.ForwardActionConfig{
						TargetGroups: []*elbv2sdk.TargetGroupTuple{
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cmp.Equal(tt.args.lhs, tt.args.rhs, CompareOptionForAction())
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompareOptionForActions(t *testing.T) {
	type args struct {
		lhs []*elbv2sdk.Action
		rhs []*elbv2sdk.Action
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "two actions slice equals exactly",
			args: args{
				lhs: []*elbv2sdk.Action{
					{
						Type: awssdk.String("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2sdk.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int64(1),
					},
					{
						Type: awssdk.String("forward"),
						ForwardConfig: &elbv2sdk.ForwardActionConfig{
							TargetGroups: []*elbv2sdk.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int64(2),
					},
				},
				rhs: []*elbv2sdk.Action{
					{
						Type: awssdk.String("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2sdk.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int64(1),
					},
					{
						Type: awssdk.String("forward"),
						ForwardConfig: &elbv2sdk.ForwardActionConfig{
							TargetGroups: []*elbv2sdk.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int64(2),
					},
				},
			},
			want: true,
		},
		{
			name: "two actions slice are not equal if there actions are not equal",
			args: args{
				lhs: []*elbv2sdk.Action{
					{
						Type: awssdk.String("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2sdk.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int64(1),
					},
					{
						Type: awssdk.String("forward"),
						ForwardConfig: &elbv2sdk.ForwardActionConfig{
							TargetGroups: []*elbv2sdk.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int64(2),
					},
				},
				rhs: []*elbv2sdk.Action{
					{
						Type: awssdk.String("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2sdk.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-b"),
							UserPoolClientId: awssdk.String("pool-client-id-b"),
							UserPoolDomain:   awssdk.String("pool-domain-b"),
						},
						Order: awssdk.Int64(1),
					},
					{
						Type: awssdk.String("forward"),
						ForwardConfig: &elbv2sdk.ForwardActionConfig{
							TargetGroups: []*elbv2sdk.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int64(2),
					},
				},
			},
			want: false,
		},
		{
			name: "two actions slice equals when they are equal after sorted by order",
			args: args{
				lhs: []*elbv2sdk.Action{
					{
						Type: awssdk.String("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2sdk.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int64(1),
					},
					{
						Type: awssdk.String("forward"),
						ForwardConfig: &elbv2sdk.ForwardActionConfig{
							TargetGroups: []*elbv2sdk.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int64(2),
					},
				},
				rhs: []*elbv2sdk.Action{
					{
						Type: awssdk.String("forward"),
						ForwardConfig: &elbv2sdk.ForwardActionConfig{
							TargetGroups: []*elbv2sdk.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int64(4),
					},
					{
						Type: awssdk.String("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2sdk.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int64(3),
					},
				},
			},
			want: true,
		},
		{
			name: "two actions slice are not equals when they are not equal after sorted by order",
			args: args{
				lhs: []*elbv2sdk.Action{
					{
						Type: awssdk.String("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2sdk.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int64(1),
					},
					{
						Type: awssdk.String("forward"),
						ForwardConfig: &elbv2sdk.ForwardActionConfig{
							TargetGroups: []*elbv2sdk.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int64(2),
					},
				},
				rhs: []*elbv2sdk.Action{
					{
						Type: awssdk.String("forward"),
						ForwardConfig: &elbv2sdk.ForwardActionConfig{
							TargetGroups: []*elbv2sdk.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("tg-a"),
								},
							},
						},
						Order: awssdk.Int64(3),
					},
					{
						Type: awssdk.String("authenticate-cognito"),
						AuthenticateCognitoConfig: &elbv2sdk.AuthenticateCognitoActionConfig{
							UserPoolArn:      awssdk.String("pool-arn-a"),
							UserPoolClientId: awssdk.String("pool-client-id-a"),
							UserPoolDomain:   awssdk.String("pool-domain-a"),
						},
						Order: awssdk.Int64(4),
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cmp.Equal(tt.args.lhs, tt.args.rhs, CompareOptionForActions())
			assert.Equal(t, tt.want, got)
		})
	}
}
