package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"testing"
)

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
