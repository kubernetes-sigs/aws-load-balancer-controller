package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCompareOptionForTransforms(t *testing.T) {
	type args struct {
		lhs []elbv2types.RuleTransform
		rhs []elbv2types.RuleTransform
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "empty transforms should be equal",
			args: args{
				lhs: []elbv2types.RuleTransform{},
				rhs: []elbv2types.RuleTransform{},
			},
			want: true,
		},
		{
			name: "nil transforms should be equal to empty transforms",
			args: args{
				lhs: nil,
				rhs: []elbv2types.RuleTransform{},
			},
			want: true,
		},
		{
			name: "transforms with same url-rewrite config should be equal",
			args: args{
				lhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("url-rewrite"),
						UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("/foo(.*)"),
									Replace: awssdk.String("/bar$1"),
								},
							},
						},
					},
				},
				rhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("url-rewrite"),
						UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("/foo(.*)"),
									Replace: awssdk.String("/bar$1"),
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "transforms with same host-header-rewrite config should be equal",
			args: args{
				lhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("host-header-rewrite"),
						HostHeaderRewriteConfig: &elbv2types.HostHeaderRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("example.com"),
									Replace: awssdk.String("example.org"),
								},
							},
						},
					},
				},
				rhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("host-header-rewrite"),
						HostHeaderRewriteConfig: &elbv2types.HostHeaderRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("example.com"),
									Replace: awssdk.String("example.org"),
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "transforms with different types should not be equal",
			args: args{
				lhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("url-rewrite"),
						UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("/foo(.*)"),
									Replace: awssdk.String("/bar$1"),
								},
							},
						},
					},
				},
				rhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("host-header-rewrite"),
						HostHeaderRewriteConfig: &elbv2types.HostHeaderRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("example.com"),
									Replace: awssdk.String("example.org"),
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "transforms with different url-rewrite config should not be equal",
			args: args{
				lhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("url-rewrite"),
						UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("/foo(.*)"),
									Replace: awssdk.String("/bar$1"),
								},
							},
						},
					},
				},
				rhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("url-rewrite"),
						UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("/foo(.*)"),
									Replace: awssdk.String("/baz$1"),
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "transforms with different host-header-rewrite config should not be equal",
			args: args{
				lhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("host-header-rewrite"),
						HostHeaderRewriteConfig: &elbv2types.HostHeaderRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("example.com"),
									Replace: awssdk.String("example.org"),
								},
							},
						},
					},
				},
				rhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("host-header-rewrite"),
						HostHeaderRewriteConfig: &elbv2types.HostHeaderRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("example.com"),
									Replace: awssdk.String("example.net"),
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "transforms with different number of transforms should not be equal",
			args: args{
				lhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("url-rewrite"),
						UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("/foo(.*)"),
									Replace: awssdk.String("/bar$1"),
								},
							},
						},
					},
				},
				rhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("url-rewrite"),
						UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("/foo(.*)"),
									Replace: awssdk.String("/bar$1"),
								},
							},
						},
					},
					{
						Type: elbv2types.TransformTypeEnum("host-header-rewrite"),
						HostHeaderRewriteConfig: &elbv2types.HostHeaderRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("example.com"),
									Replace: awssdk.String("example.org"),
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "transforms with same configs but different order should be equal",
			args: args{
				lhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("url-rewrite"),
						UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("/foo(.*)"),
									Replace: awssdk.String("/bar$1"),
								},
							},
						},
					},
					{
						Type: elbv2types.TransformTypeEnum("host-header-rewrite"),
						HostHeaderRewriteConfig: &elbv2types.HostHeaderRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("example.com"),
									Replace: awssdk.String("example.org"),
								},
							},
						},
					},
				},
				rhs: []elbv2types.RuleTransform{
					{
						Type: elbv2types.TransformTypeEnum("host-header-rewrite"),
						HostHeaderRewriteConfig: &elbv2types.HostHeaderRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("example.com"),
									Replace: awssdk.String("example.org"),
								},
							},
						},
					},
					{
						Type: elbv2types.TransformTypeEnum("url-rewrite"),
						UrlRewriteConfig: &elbv2types.UrlRewriteConfig{
							Rewrites: []elbv2types.RewriteConfig{
								{
									Regex:   awssdk.String("/foo(.*)"),
									Replace: awssdk.String("/bar$1"),
								},
							},
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cmp.Equal(tt.args.lhs, tt.args.rhs, CompareOptionForTransforms(nil, nil))
			assert.Equal(t, tt.want, got)
		})
	}
}
