package ingress

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultRuleOptimizer_Optimize(t *testing.T) {
	type args struct {
		port     int64
		protocol elbv2model.Protocol
		rules    []Rule
	}
	tests := []struct {
		name    string
		args    args
		want    []Rule
		wantErr error
	}{
		{
			name: "infinite redirect rule should be omitted",
			args: args{
				port:     443,
				protocol: elbv2model.ProtocolHTTPS,
				rules: []Rule{
					{
						Conditions: []elbv2model.RuleCondition{
							{
								Field: elbv2model.RuleConditionFieldPathPattern,
								PathPatternConfig: &elbv2model.PathPatternConditionConfig{
									Values: []string{"/www"},
								},
							},
						},
						Actions: []elbv2model.Action{
							{
								Type: elbv2model.ActionTypeRedirect,
								RedirectConfig: &elbv2model.RedirectActionConfig{
									Path:       awssdk.String("/app"),
									StatusCode: "HTTP_301",
								},
							},
						},
					},
					{
						Conditions: []elbv2model.RuleCondition{
							{
								Field: elbv2model.RuleConditionFieldPathPattern,
								PathPatternConfig: &elbv2model.PathPatternConditionConfig{
									Values: []string{"/*"},
								},
							},
						},
						Actions: []elbv2model.Action{
							{
								Type: elbv2model.ActionTypeRedirect,
								RedirectConfig: &elbv2model.RedirectActionConfig{
									StatusCode: "HTTP_301",
								},
							},
						},
					},
					{
						Conditions: []elbv2model.RuleCondition{
							{
								Field: elbv2model.RuleConditionFieldPathPattern,
								PathPatternConfig: &elbv2model.PathPatternConditionConfig{
									Values: []string{"/app"},
								},
							},
						},
						Actions: []elbv2model.Action{
							{
								Type: elbv2model.ActionTypeFixedResponse,
								FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
									StatusCode: "200",
								},
							},
						},
					},
				},
			},
			want: []Rule{
				{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Path:       awssdk.String("/app"),
								StatusCode: "HTTP_301",
							},
						},
					},
				},
				{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/app"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeFixedResponse,
							FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
								StatusCode: "200",
							},
						},
					},
				},
			},
		},
		{
			name: "rules after a unconditional redirect rule should be omitted",
			args: args{
				port:     443,
				protocol: elbv2model.ProtocolHTTPS,
				rules: []Rule{
					{
						Conditions: []elbv2model.RuleCondition{
							{
								Field: elbv2model.RuleConditionFieldPathPattern,
								PathPatternConfig: &elbv2model.PathPatternConditionConfig{
									Values: []string{"/www"},
								},
							},
						},
						Actions: []elbv2model.Action{
							{
								Type: elbv2model.ActionTypeRedirect,
								RedirectConfig: &elbv2model.RedirectActionConfig{
									Path:       awssdk.String("/app"),
									StatusCode: "HTTP_301",
								},
							},
						},
					},
					{
						Conditions: []elbv2model.RuleCondition{
							{
								Field: elbv2model.RuleConditionFieldPathPattern,
								PathPatternConfig: &elbv2model.PathPatternConditionConfig{
									Values: []string{"/*"},
								},
							},
						},
						Actions: []elbv2model.Action{
							{
								Type: elbv2model.ActionTypeRedirect,
								RedirectConfig: &elbv2model.RedirectActionConfig{
									Host:       awssdk.String("home.example.com"),
									StatusCode: "HTTP_301",
								},
							},
						},
					},
					{
						Conditions: []elbv2model.RuleCondition{
							{
								Field: elbv2model.RuleConditionFieldPathPattern,
								PathPatternConfig: &elbv2model.PathPatternConditionConfig{
									Values: []string{"/app"},
								},
							},
						},
						Actions: []elbv2model.Action{
							{
								Type: elbv2model.ActionTypeFixedResponse,
								FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
									StatusCode: "200",
								},
							},
						},
					},
				},
			},
			want: []Rule{
				{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Path:       awssdk.String("/app"),
								StatusCode: "HTTP_301",
							},
						},
					},
				},
				{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/*"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Host:       awssdk.String("home.example.com"),
								StatusCode: "HTTP_301",
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &defaultRuleOptimizer{
				logger: &log.NullLogger{},
			}
			got, err := o.Optimize(context.Background(), tt.args.port, tt.args.protocol, tt.args.rules)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_isInfiniteRedirectRule(t *testing.T) {
	type args struct {
		port     int64
		protocol elbv2model.Protocol
		rule     Rule
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "is infinite redirect rule when all fields are unset",
			args: args{
				port:     443,
				protocol: elbv2model.ProtocolHTTPS,
				rule: Rule{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldHostHeader,
							HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
								Values: []string{"www.example.com", "app.example.com"},
							},
						},
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www", "/app"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								StatusCode: "HTTP_301",
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "is infinite redirect rule when all fields are set to default value",
			args: args{
				port:     443,
				protocol: elbv2model.ProtocolHTTPS,
				rule: Rule{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldHostHeader,
							HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
								Values: []string{"www.example.com", "app.example.com"},
							},
						},
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www", "/app"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Host:       awssdk.String("#{host}"),
								Path:       awssdk.String("/#{path}"),
								Port:       awssdk.String("#{port}"),
								Protocol:   awssdk.String("#{protocol}"),
								Query:      awssdk.String("#{query}"),
								StatusCode: "HTTP_301",
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "is infinite redirect rule when host didn't differs",
			args: args{
				port:     443,
				protocol: elbv2model.ProtocolHTTPS,
				rule: Rule{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldHostHeader,
							HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
								Values: []string{"www.example.com", "app.example.com"},
							},
						},
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www", "/app"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Host:       awssdk.String("app.example.com"),
								StatusCode: "HTTP_301",
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "isn't infinite redirect rule when host differs",
			args: args{
				port:     443,
				protocol: elbv2model.ProtocolHTTPS,
				rule: Rule{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldHostHeader,
							HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
								Values: []string{"www.example.com", "app.example.com"},
							},
						},
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www", "/app"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Host:       awssdk.String("home.example.com"),
								StatusCode: "HTTP_301",
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "is infinite redirect rule when path didn't differs",
			args: args{
				port:     443,
				protocol: elbv2model.ProtocolHTTPS,
				rule: Rule{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldHostHeader,
							HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
								Values: []string{"www.example.com", "app.example.com"},
							},
						},
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www", "/app"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Path:       awssdk.String("/app"),
								StatusCode: "HTTP_301",
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "isn't infinite redirect rule when path differs",
			args: args{
				port:     443,
				protocol: elbv2model.ProtocolHTTPS,
				rule: Rule{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldHostHeader,
							HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
								Values: []string{"www.example.com", "app.example.com"},
							},
						},
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www", "/app"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Path:       awssdk.String("/home"),
								StatusCode: "HTTP_301",
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "is infinite redirect rule when port didn't differs",
			args: args{
				port:     443,
				protocol: elbv2model.ProtocolHTTPS,
				rule: Rule{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldHostHeader,
							HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
								Values: []string{"www.example.com", "app.example.com"},
							},
						},
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www", "/app"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Port:       awssdk.String("443"),
								StatusCode: "HTTP_301",
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "isn't infinite redirect rule when port differs",
			args: args{
				port:     443,
				protocol: elbv2model.ProtocolHTTPS,
				rule: Rule{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldHostHeader,
							HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
								Values: []string{"www.example.com", "app.example.com"},
							},
						},
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www", "/app"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Port:       awssdk.String("80"),
								StatusCode: "HTTP_301",
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "is infinite redirect rule when protocol didn't differs",
			args: args{
				port:     443,
				protocol: elbv2model.ProtocolHTTPS,
				rule: Rule{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldHostHeader,
							HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
								Values: []string{"www.example.com", "app.example.com"},
							},
						},
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www", "/app"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Protocol:   awssdk.String("HTTPS"),
								StatusCode: "HTTP_301",
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "isn't infinite redirect rule when protocol differs",
			args: args{
				port:     80,
				protocol: elbv2model.ProtocolHTTPS,
				rule: Rule{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldHostHeader,
							HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
								Values: []string{"www.example.com", "app.example.com"},
							},
						},
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www", "/app"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Protocol:   awssdk.String("HTTP"),
								StatusCode: "HTTP_301",
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "isn't infinite redirect rule when query differs",
			args: args{
				port:     80,
				protocol: elbv2model.ProtocolHTTPS,
				rule: Rule{
					Conditions: []elbv2model.RuleCondition{
						{
							Field: elbv2model.RuleConditionFieldHostHeader,
							HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
								Values: []string{"www.example.com", "app.example.com"},
							},
						},
						{
							Field: elbv2model.RuleConditionFieldPathPattern,
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/www", "/app"},
							},
						},
					},
					Actions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeRedirect,
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Query:      awssdk.String("a=b"),
								StatusCode: "HTTP_301",
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
			got := isInfiniteRedirectRule(tt.args.port, tt.args.protocol, tt.args.rule)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_isSupersetConditions(t *testing.T) {
	type args struct {
		lhsConditions []elbv2model.RuleCondition
		rhsConditions []elbv2model.RuleCondition
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "lhs hosts is superSet and lhs conditions nil",
			args: args{
				lhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldHostHeader,
						HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
							Values: []string{"www.example.com", "app.example.com"},
						},
					},
				},
				rhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldHostHeader,
						HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
							Values: []string{"app.example.com"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "lhs hosts isn't superSet and lhs conditions nil",
			args: args{
				lhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldHostHeader,
						HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
							Values: []string{"www.example.com", "app.example.com"},
						},
					},
				},
				rhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldHostHeader,
						HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
							Values: []string{"home.example.com"},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "lhs hosts isn't superSet and lhs conditions nil - due to rhs hosts is empty",
			args: args{
				lhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldHostHeader,
						HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
							Values: []string{"www.example.com", "app.example.com"},
						},
					},
				},
				rhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldSourceIP,
						SourceIPConfig: &elbv2model.SourceIPConditionConfig{
							Values: []string{"192.168.0.0/16"},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "lhs hosts is nil and lhs conditions is superSet",
			args: args{
				lhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/www", "/app"},
						},
					},
				},
				rhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/app"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "lhs hosts is nil and lhs conditions isn't superSet",
			args: args{
				lhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/www", "/app"},
						},
					},
				},
				rhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/home"},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "lhs hosts is nil and lhs conditions isn't superSet - due to rhs conditions is empty",
			args: args{
				lhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/www", "/app"},
						},
					},
				},
				rhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldSourceIP,
						SourceIPConfig: &elbv2model.SourceIPConditionConfig{
							Values: []string{"192.168.0.0/16"},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "lhs hosts is nil and lhs conditions is superSet - due to /*",
			args: args{
				lhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/www", "/*"},
						},
					},
				},
				rhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/home"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "both lhs hosts and lhs conditions exists, and both are superSet",
			args: args{
				lhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldHostHeader,
						HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
							Values: []string{"www.example.com", "app.example.com"},
						},
					},
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/www", "/app"},
						},
					},
				},
				rhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldHostHeader,
						HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
							Values: []string{"app.example.com"},
						},
					},
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/app"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "both lhs hosts and lhs conditions exists, but only hosts is superSet",
			args: args{
				lhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldHostHeader,
						HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
							Values: []string{"www.example.com", "app.example.com"},
						},
					},
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/www", "/app"},
						},
					},
				},
				rhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldHostHeader,
						HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
							Values: []string{"app.example.com"},
						},
					},
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/home"},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "both lhs hosts and lhs conditions exists, but only conditions is superSet",
			args: args{
				lhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldHostHeader,
						HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
							Values: []string{"www.example.com", "app.example.com"},
						},
					},
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/www", "/app"},
						},
					},
				},
				rhsConditions: []elbv2model.RuleCondition{
					{
						Field: elbv2model.RuleConditionFieldHostHeader,
						HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
							Values: []string{"home.example.com"},
						},
					},
					{
						Field: elbv2model.RuleConditionFieldPathPattern,
						PathPatternConfig: &elbv2model.PathPatternConditionConfig{
							Values: []string{"/app"},
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSupersetConditions(tt.args.lhsConditions, tt.args.rhsConditions)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_findRedirectActionConfig(t *testing.T) {
	type args struct {
		actions []elbv2model.Action
	}
	tests := []struct {
		name string
		args args
		want *elbv2model.RedirectActionConfig
	}{
		{
			name: "redirectAction",
			args: args{
				actions: []elbv2model.Action{
					{
						Type: elbv2model.ActionTypeRedirect,
						RedirectConfig: &elbv2model.RedirectActionConfig{
							StatusCode: "HTTP_301",
						},
					},
				},
			},
			want: &elbv2model.RedirectActionConfig{
				StatusCode: "HTTP_301",
			},
		},
		{
			name: "redirectAction with Auth",
			args: args{
				actions: []elbv2model.Action{
					{
						Type:                   elbv2model.ActionTypeAuthenticateOIDC,
						AuthenticateOIDCConfig: &elbv2model.AuthenticateOIDCActionConfig{},
					},
					{
						Type: elbv2model.ActionTypeRedirect,
						RedirectConfig: &elbv2model.RedirectActionConfig{
							StatusCode: "HTTP_301",
						},
					},
				},
			},
			want: &elbv2model.RedirectActionConfig{
				StatusCode: "HTTP_301",
			},
		},
		{
			name: "fixed response action",
			args: args{
				actions: []elbv2model.Action{
					{
						Type: elbv2model.ActionTypeFixedResponse,
						FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
							StatusCode: "200",
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "nil actions",
			args: args{
				actions: nil,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findRedirectActionConfig(tt.args.actions)
			assert.Equal(t, tt.want, got)
		})
	}
}
