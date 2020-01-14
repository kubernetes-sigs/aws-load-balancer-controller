package ls

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/conditions"
	"github.com/stretchr/testify/assert"
)

func Test_actionsMatches(t *testing.T) {
	type args struct {
		desired []*elbv2.Action
		current []*elbv2.Action
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "nil and empty conditions matches",
			args: args{
				desired: nil,
				current: []*elbv2.Action{},
			},
			want: true,
		},
		{
			name: "actions matches with order",
			args: args{
				desired: []*elbv2.Action{
					{
						Type: aws.String(elbv2.ActionTypeEnumAuthenticateOidc),
						AuthenticateOidcConfig: &elbv2.AuthenticateOidcActionConfig{
							ClientId: aws.String("oidc-client"),
						},
						Order: aws.Int64(1),
					},
					{
						Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
						FixedResponseConfig: &elbv2.FixedResponseActionConfig{
							MessageBody: aws.String("hello world!"),
							StatusCode:  aws.String("200"),
						},
						Order: aws.Int64(2),
					},
				},
				current: []*elbv2.Action{
					{
						Type: aws.String(elbv2.ActionTypeEnumAuthenticateOidc),
						AuthenticateOidcConfig: &elbv2.AuthenticateOidcActionConfig{
							ClientId: aws.String("oidc-client"),
						},
						Order: aws.Int64(1),
					},
					{
						Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
						FixedResponseConfig: &elbv2.FixedResponseActionConfig{
							MessageBody: aws.String("hello world!"),
							StatusCode:  aws.String("200"),
						},
						Order: aws.Int64(2),
					},
				},
			},
			want: true,
		},
		{
			name: "actions matches without order",
			args: args{
				desired: []*elbv2.Action{
					{
						Type: aws.String(elbv2.ActionTypeEnumAuthenticateOidc),
						AuthenticateOidcConfig: &elbv2.AuthenticateOidcActionConfig{
							ClientId: aws.String("oidc-client"),
						},
						Order: aws.Int64(1),
					},
					{
						Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
						FixedResponseConfig: &elbv2.FixedResponseActionConfig{
							MessageBody: aws.String("hello world!"),
							StatusCode:  aws.String("200"),
						},
						Order: aws.Int64(2),
					},
				},
				current: []*elbv2.Action{
					{
						Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
						FixedResponseConfig: &elbv2.FixedResponseActionConfig{
							MessageBody: aws.String("hello world!"),
							StatusCode:  aws.String("200"),
						},
						Order: aws.Int64(2),
					},
					{
						Type: aws.String(elbv2.ActionTypeEnumAuthenticateOidc),
						AuthenticateOidcConfig: &elbv2.AuthenticateOidcActionConfig{
							ClientId: aws.String("oidc-client"),
						},
						Order: aws.Int64(1),
					},
				},
			},
			want: true,
		},
		{
			name: "actions mismatches - mismatched order",
			args: args{
				desired: []*elbv2.Action{
					{
						Type: aws.String(elbv2.ActionTypeEnumAuthenticateOidc),
						AuthenticateOidcConfig: &elbv2.AuthenticateOidcActionConfig{
							ClientId: aws.String("oidc-client"),
						},
						Order: aws.Int64(1),
					},
					{
						Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
						FixedResponseConfig: &elbv2.FixedResponseActionConfig{
							MessageBody: aws.String("hello world!"),
							StatusCode:  aws.String("200"),
						},
						Order: aws.Int64(2),
					},
				},
				current: []*elbv2.Action{
					{
						Type: aws.String(elbv2.ActionTypeEnumAuthenticateOidc),
						AuthenticateOidcConfig: &elbv2.AuthenticateOidcActionConfig{
							ClientId: aws.String("oidc-client"),
						},
						Order: aws.Int64(2),
					},
					{
						Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
						FixedResponseConfig: &elbv2.FixedResponseActionConfig{
							MessageBody: aws.String("hello world!"),
							StatusCode:  aws.String("200"),
						},
						Order: aws.Int64(1),
					},
				},
			},
			want: false,
		},
		{
			name: "actions mismatches - mismatched action",
			args: args{
				desired: []*elbv2.Action{
					{
						Type: aws.String(elbv2.ActionTypeEnumAuthenticateOidc),
						AuthenticateOidcConfig: &elbv2.AuthenticateOidcActionConfig{
							ClientId: aws.String("oidc-client"),
						},
						Order: aws.Int64(1),
					},
					{
						Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
						FixedResponseConfig: &elbv2.FixedResponseActionConfig{
							MessageBody: aws.String("hello world!"),
							StatusCode:  aws.String("200"),
						},
						Order: aws.Int64(2),
					},
				},
				current: []*elbv2.Action{
					{
						Type: aws.String(elbv2.ActionTypeEnumAuthenticateOidc),
						AuthenticateOidcConfig: &elbv2.AuthenticateOidcActionConfig{
							ClientId: aws.String("oidc-client"),
						},
						Order: aws.Int64(1),
					},
					{
						Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
						FixedResponseConfig: &elbv2.FixedResponseActionConfig{
							MessageBody: aws.String("hello world!"),
							StatusCode:  aws.String("201"),
						},
						Order: aws.Int64(2),
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := actionsMatches(tt.args.desired, tt.args.current); got != tt.want {
				t.Errorf("actionsMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_conditionsMatches(t *testing.T) {
	type args struct {
		desired []*elbv2.RuleCondition
		current []*elbv2.RuleCondition
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "nil and empty conditions matches",
			args: args{
				desired: nil,
				current: []*elbv2.RuleCondition{},
			},
			want: true,
		},
		{
			name: "conditions matches with order",
			args: args{
				desired: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"a.example.com", "b.example.com"}),
						},
					},
					{
						Field: aws.String(conditions.FieldQueryString),
						QueryStringConfig: &elbv2.QueryStringConditionConfig{
							Values: []*elbv2.QueryStringKeyValuePair{
								{
									Key:   aws.String("version"),
									Value: aws.String("v1"),
								},
								{
									Value: aws.String("example"),
								},
							},
						},
					},
					{
						Field: aws.String(conditions.FieldQueryString),
						QueryStringConfig: &elbv2.QueryStringConditionConfig{
							Values: []*elbv2.QueryStringKeyValuePair{
								{
									Key:   aws.String("username"),
									Value: aws.String("m00nf1sh"),
								},
							},
						},
					},
				},
				current: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"a.example.com", "b.example.com"}),
						},
					},
					{
						Field: aws.String(conditions.FieldQueryString),
						QueryStringConfig: &elbv2.QueryStringConditionConfig{
							Values: []*elbv2.QueryStringKeyValuePair{
								{
									Key:   aws.String("version"),
									Value: aws.String("v1"),
								},
								{
									Value: aws.String("example"),
								},
							},
						},
					},
					{
						Field: aws.String(conditions.FieldQueryString),
						QueryStringConfig: &elbv2.QueryStringConditionConfig{
							Values: []*elbv2.QueryStringKeyValuePair{
								{
									Key:   aws.String("username"),
									Value: aws.String("m00nf1sh"),
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "conditions matches without order",
			args: args{
				desired: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"a.example.com", "b.example.com"}),
						},
					},
					{
						Field: aws.String(conditions.FieldQueryString),
						QueryStringConfig: &elbv2.QueryStringConditionConfig{
							Values: []*elbv2.QueryStringKeyValuePair{
								{
									Key:   aws.String("version"),
									Value: aws.String("v1"),
								},
								{
									Value: aws.String("example"),
								},
							},
						},
					},
					{
						Field: aws.String(conditions.FieldQueryString),
						QueryStringConfig: &elbv2.QueryStringConditionConfig{
							Values: []*elbv2.QueryStringKeyValuePair{
								{
									Key:   aws.String("username"),
									Value: aws.String("m00nf1sh"),
								},
							},
						},
					},
				},
				current: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"b.example.com", "a.example.com"}),
						},
					},
					{
						Field: aws.String(conditions.FieldQueryString),
						QueryStringConfig: &elbv2.QueryStringConditionConfig{
							Values: []*elbv2.QueryStringKeyValuePair{
								{
									Key:   aws.String("username"),
									Value: aws.String("m00nf1sh"),
								},
							},
						},
					},
					{
						Field: aws.String(conditions.FieldQueryString),
						QueryStringConfig: &elbv2.QueryStringConditionConfig{
							Values: []*elbv2.QueryStringKeyValuePair{
								{
									Value: aws.String("example"),
								},
								{
									Key:   aws.String("version"),
									Value: aws.String("v1"),
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "conditions mismatch",
			args: args{
				desired: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"a.example.com", "b.example.com"}),
						},
					},
				},
				current: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"a.example.com", "c.example.com"}),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "conditions mismatch - missing condition",
			args: args{
				desired: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"a.example.com", "b.example.com"}),
						},
					},
					{
						Field: aws.String(conditions.FieldQueryString),
						QueryStringConfig: &elbv2.QueryStringConditionConfig{
							Values: []*elbv2.QueryStringKeyValuePair{
								{
									Key:   aws.String("username"),
									Value: aws.String("m00nf1sh"),
								},
							},
						},
					},
				},
				current: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"a.example.com", "b.example.com"}),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "conditions mismatch - extra condition",
			args: args{
				desired: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"a.example.com", "b.example.com"}),
						},
					},
				},
				current: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"a.example.com", "b.example.com"}),
						},
					},
					{
						Field: aws.String(conditions.FieldQueryString),
						QueryStringConfig: &elbv2.QueryStringConditionConfig{
							Values: []*elbv2.QueryStringKeyValuePair{
								{
									Key:   aws.String("username"),
									Value: aws.String("m00nf1sh"),
								},
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
			if got := conditionsMatches(tt.args.desired, tt.args.current); got != tt.want {
				t.Errorf("conditionsMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_sliceMatches(t *testing.T) {
	type args struct {
		desired []*string
		current []*string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "nil and empty slice matches",
			args: args{
				desired: nil,
				current: []*string{},
			},
			want: true,
		},
		{
			name: "elements matches with order",
			args: args{
				desired: aws.StringSlice([]string{"a", "b", "c"}),
				current: aws.StringSlice([]string{"a", "b", "c"}),
			},
			want: true,
		},
		{
			name: "elements matches without order",
			args: args{
				desired: aws.StringSlice([]string{"a", "b", "c"}),
				current: aws.StringSlice([]string{"b", "a", "c"}),
			},
			want: true,
		},
		{
			name: "elements length mismatch",
			args: args{
				desired: aws.StringSlice([]string{"a", "b"}),
				current: aws.StringSlice([]string{"a", "b", "c"}),
			},
			want: false,
		},
		{
			name: "elements occurrence mismatch",
			args: args{
				desired: aws.StringSlice([]string{"a", "a", "b"}),
				current: aws.StringSlice([]string{"a", "b", "c"}),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stringSliceMatches(tt.args.desired, tt.args.current); got != tt.want {
				t.Errorf("sliceMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_httpHeaderConditionConfigMatches(t *testing.T) {
	type args struct {
		desired *elbv2.HttpHeaderConditionConfig
		current *elbv2.HttpHeaderConditionConfig
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "header condition mismatch if desired nil while current non-nil",
			args: args{
				desired: nil,
				current: &elbv2.HttpHeaderConditionConfig{
					HttpHeaderName: aws.String("header1"),
					Values:         aws.StringSlice([]string{"value1"}),
				},
			},
			want: false,
		},
		{
			name: "header condition mismatch if header name mismatch",
			args: args{
				desired: &elbv2.HttpHeaderConditionConfig{
					HttpHeaderName: aws.String("header1"),
					Values:         aws.StringSlice([]string{"value1"}),
				},
				current: &elbv2.HttpHeaderConditionConfig{
					HttpHeaderName: aws.String("header2"),
					Values:         aws.StringSlice([]string{"value1"}),
				},
			},
			want: false,
		},
		{
			name: "header condition mismatch if header values mismatch",
			args: args{
				desired: &elbv2.HttpHeaderConditionConfig{
					HttpHeaderName: aws.String("header1"),
					Values:         aws.StringSlice([]string{"value1"}),
				},
				current: &elbv2.HttpHeaderConditionConfig{
					HttpHeaderName: aws.String("header1"),
					Values:         aws.StringSlice([]string{"value2"}),
				},
			},
			want: false,
		},
		{
			name: "header condition matches without values order",
			args: args{
				desired: &elbv2.HttpHeaderConditionConfig{
					HttpHeaderName: aws.String("header1"),
					Values:         aws.StringSlice([]string{"value1", "value2"}),
				},
				current: &elbv2.HttpHeaderConditionConfig{
					HttpHeaderName: aws.String("header1"),
					Values:         aws.StringSlice([]string{"value2", "value1"}),
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := httpHeaderConditionConfigMatches(tt.args.desired, tt.args.current); got != tt.want {
				t.Errorf("httpHeaderConditionConfigMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_queryStringConditionConfigMatches(t *testing.T) {
	type args struct {
		desired *elbv2.QueryStringConditionConfig
		current *elbv2.QueryStringConditionConfig
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "query string condition mismatch if desired nil while current non-nil",
			args: args{
				desired: nil,
				current: &elbv2.QueryStringConditionConfig{Values: []*elbv2.QueryStringKeyValuePair{
					{
						Key:   aws.String("param1"),
						Value: aws.String("value1"),
					},
				}},
			},
			want: false,
		},
		{
			name: "query string condition mismatch if desired key-pair is less than current",
			args: args{
				desired: &elbv2.QueryStringConditionConfig{Values: []*elbv2.QueryStringKeyValuePair{
					{
						Key:   aws.String("param1"),
						Value: aws.String("value1"),
					},
				}},
				current: &elbv2.QueryStringConditionConfig{Values: []*elbv2.QueryStringKeyValuePair{
					{
						Key:   aws.String("param1"),
						Value: aws.String("value1"),
					},
					{
						Key:   aws.String("param2"),
						Value: aws.String("value2"),
					},
				}},
			},
			want: false,
		},
		{
			name: "query string condition mismatch if desired key-pair is more than current",
			args: args{
				desired: &elbv2.QueryStringConditionConfig{Values: []*elbv2.QueryStringKeyValuePair{
					{
						Key:   aws.String("param1"),
						Value: aws.String("value1"),
					},
					{
						Key:   aws.String("param2"),
						Value: aws.String("value2"),
					},
				}},
				current: &elbv2.QueryStringConditionConfig{Values: []*elbv2.QueryStringKeyValuePair{
					{
						Key:   aws.String("param1"),
						Value: aws.String("value1"),
					},
				}},
			},
			want: false,
		},
		{
			name: "query string condition mismatch if desired and current keyPair key mismatch",
			args: args{
				desired: &elbv2.QueryStringConditionConfig{Values: []*elbv2.QueryStringKeyValuePair{
					{
						Key:   aws.String("param1"),
						Value: aws.String("value1"),
					},
				}},
				current: &elbv2.QueryStringConditionConfig{Values: []*elbv2.QueryStringKeyValuePair{
					{
						Key:   aws.String("param2"),
						Value: aws.String("value1"),
					},
				}},
			},
			want: false,
		},
		{
			name: "query string condition mismatch if desired and current keyPair value mismatch",
			args: args{
				desired: &elbv2.QueryStringConditionConfig{Values: []*elbv2.QueryStringKeyValuePair{
					{
						Key:   aws.String("param1"),
						Value: aws.String("value1"),
					},
				}},
				current: &elbv2.QueryStringConditionConfig{Values: []*elbv2.QueryStringKeyValuePair{
					{
						Key:   aws.String("param1"),
						Value: aws.String("value2"),
					},
				}},
			},
			want: false,
		},
		{
			name: "query string condition matches if desired and current keyPair matches without order",
			args: args{
				desired: &elbv2.QueryStringConditionConfig{Values: []*elbv2.QueryStringKeyValuePair{
					{
						Key:   aws.String("param1"),
						Value: aws.String("value1"),
					},
					{
						Key:   aws.String("param2"),
						Value: aws.String("value2"),
					},
				}},
				current: &elbv2.QueryStringConditionConfig{Values: []*elbv2.QueryStringKeyValuePair{
					{
						Key:   aws.String("param2"),
						Value: aws.String("value2"),
					},
					{
						Key:   aws.String("param1"),
						Value: aws.String("value1"),
					},
				}},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := queryStringConditionConfigMatches(tt.args.desired, tt.args.current); got != tt.want {
				t.Errorf("queryStringConditionConfigMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_sortActions(t *testing.T) {
	for _, tc := range []struct {
		name            string
		actions         []*elbv2.Action
		expectedActions []*elbv2.Action
	}{
		{
			name: "sort based on action order",
			actions: []*elbv2.Action{
				{
					Order: aws.Int64(3),
				},
				{
					Order: aws.Int64(1),
				},
				{
					Order: aws.Int64(2),
				},
			},
			expectedActions: []*elbv2.Action{
				{
					Order: aws.Int64(1),
				},
				{
					Order: aws.Int64(2),
				},
				{
					Order: aws.Int64(3),
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actions := sortedActions(tc.actions)
			assert.Equal(t, tc.expectedActions, actions)
		})
	}
}
