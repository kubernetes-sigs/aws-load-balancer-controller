package routeutils

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"regexp"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

func Test_BuildRoutingRuleTransforms(t *testing.T) {
	exact := gwv1.PathMatchExact
	testCases := []struct {
		name     string
		route    RouteDescriptor
		rule     RulePrecedence
		expected []elbv2.Transform
	}{
		{
			name:     "unsupported route",
			route:    &mockRoute{routeKind: GRPCRouteKind},
			rule:     RulePrecedence{},
			expected: []elbv2.Transform{},
		},
		{
			name: "no transforms",
			route: &mockRoute{
				routeKind: HTTPRouteKind,
			},
			rule: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: convertHTTPRouteRule(&gwv1.HTTPRouteRule{
						Matches: []gwv1.HTTPRouteMatch{
							{
								Path: &gwv1.HTTPPathMatch{
									Type:  &exact,
									Value: awssdk.String("/foo"),
								},
							},
						},
					}, nil, nil),
				},
			},
		},
		{
			name: "path rewrite",
			route: &mockRoute{
				routeKind: HTTPRouteKind,
			},
			rule: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: convertHTTPRouteRule(&gwv1.HTTPRouteRule{
						Matches: []gwv1.HTTPRouteMatch{
							{
								Path: &gwv1.HTTPPathMatch{
									Type:  &exact,
									Value: awssdk.String("/foo"),
								},
							},
						},
						Filters: []gwv1.HTTPRouteFilter{
							{
								URLRewrite: &gwv1.HTTPURLRewriteFilter{
									Hostname: nil,
									Path: &gwv1.HTTPPathModifier{
										Type:            gwv1.FullPathHTTPPathModifier,
										ReplaceFullPath: awssdk.String("/bar"),
									},
								},
							},
						},
					}, nil, nil),
				},
			},
			expected: []elbv2.Transform{
				{
					Type: elbv2.TransformTypeUrlRewrite,
					UrlRewriteConfig: &elbv2.RewriteConfigObject{
						Rewrites: []elbv2.RewriteConfig{
							{
								Regex:   "^([^?]*)",
								Replace: "/bar",
							},
						},
					},
				},
			},
		},
		{
			name: "header rewrite",
			route: &mockRoute{
				routeKind: HTTPRouteKind,
			},
			rule: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: convertHTTPRouteRule(&gwv1.HTTPRouteRule{
						Matches: []gwv1.HTTPRouteMatch{
							{
								Path: &gwv1.HTTPPathMatch{
									Type:  &exact,
									Value: awssdk.String("/foo"),
								},
							},
						},
						Filters: []gwv1.HTTPRouteFilter{
							{
								URLRewrite: &gwv1.HTTPURLRewriteFilter{
									Hostname: (*gwv1.PreciseHostname)(awssdk.String("foo.com")),
								},
							},
						},
					}, nil, nil),
				},
			},
			expected: []elbv2.Transform{
				{
					Type: elbv2.TransformTypeHostHeaderRewrite,
					HostHeaderRewriteConfig: &elbv2.RewriteConfigObject{
						Rewrites: []elbv2.RewriteConfig{
							{
								Regex:   ".*",
								Replace: "foo.com",
							},
						},
					},
				},
			},
		},
		{
			name: "header and url rewrite",
			route: &mockRoute{
				routeKind: HTTPRouteKind,
			},
			rule: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: convertHTTPRouteRule(&gwv1.HTTPRouteRule{
						Matches: []gwv1.HTTPRouteMatch{
							{
								Path: &gwv1.HTTPPathMatch{
									Type:  &exact,
									Value: awssdk.String("/foo"),
								},
							},
						},
						Filters: []gwv1.HTTPRouteFilter{
							{
								URLRewrite: &gwv1.HTTPURLRewriteFilter{
									Hostname: (*gwv1.PreciseHostname)(awssdk.String("foo.com")),
									Path: &gwv1.HTTPPathModifier{
										Type:            gwv1.FullPathHTTPPathModifier,
										ReplaceFullPath: awssdk.String("/bar"),
									},
								},
							},
						},
					}, nil, nil),
				},
			},
			expected: []elbv2.Transform{
				{
					Type: elbv2.TransformTypeUrlRewrite,
					UrlRewriteConfig: &elbv2.RewriteConfigObject{
						Rewrites: []elbv2.RewriteConfig{
							{
								Regex:   "^([^?]*)",
								Replace: "/bar",
							},
						},
					},
				},
				{
					Type: elbv2.TransformTypeHostHeaderRewrite,
					HostHeaderRewriteConfig: &elbv2.RewriteConfigObject{
						Rewrites: []elbv2.RewriteConfig{
							{
								Regex:   ".*",
								Replace: "foo.com",
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := BuildRoutingRuleTransforms(tc.route, tc.rule)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_generateURLRewritePathTransform(t *testing.T) {

	type pathWriteCase struct {
		input  string
		output string
	}

	prefix := gwv1.PathMatchPathPrefix
	testCases := []struct {
		name           string
		gwPathModifier gwv1.HTTPPathModifier
		httpMatch      *gwv1.HTTPRouteMatch
		expected       elbv2.Transform
		rewriteCases   []pathWriteCase
	}{
		{
			name: "full path rewrite",
			gwPathModifier: gwv1.HTTPPathModifier{
				Type:            gwv1.FullPathHTTPPathModifier,
				ReplaceFullPath: awssdk.String("/cat"),
			},
			httpMatch: &gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{},
			},
			expected: elbv2.Transform{
				Type: elbv2.TransformTypeUrlRewrite,
				UrlRewriteConfig: &elbv2.RewriteConfigObject{
					Rewrites: []elbv2.RewriteConfig{
						{
							Regex:   "^([^?]*)",
							Replace: "/cat",
						},
					},
				},
			},
			rewriteCases: []pathWriteCase{
				{
					input:  "/foo",
					output: "/cat",
				},
				{
					input:  "/foo/bar/baz/bat/",
					output: "/cat",
				},
				{
					input:  "/",
					output: "/cat",
				},
				{
					input:  "/foo?q1=q2",
					output: "/cat?q1=q2",
				},
				{
					input:  "/foo?q1=q2&q3=q4",
					output: "/cat?q1=q2&q3=q4",
				},
			},
		},
		{
			name: "prefix path rewrite",
			httpMatch: &gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  &prefix,
					Value: awssdk.String("/foo"),
				},
			},
			gwPathModifier: gwv1.HTTPPathModifier{
				Type:               gwv1.PrefixMatchHTTPPathModifier,
				ReplacePrefixMatch: awssdk.String("/cat"),
			},
			expected: elbv2.Transform{
				Type: elbv2.TransformTypeUrlRewrite,
				UrlRewriteConfig: &elbv2.RewriteConfigObject{
					Rewrites: []elbv2.RewriteConfig{
						{
							Regex:   "(^/foo(/)?)",
							Replace: "/cat$2",
						},
					},
				},
			},
			rewriteCases: []pathWriteCase{
				{
					input:  "/foo",
					output: "/cat",
				},
				{
					input:  "/foo/bar/baz/bat/",
					output: "/cat/bar/baz/bat/",
				},
				{
					input:  "/foo?q1=q2",
					output: "/cat?q1=q2",
				},
				{
					input:  "/foo?q1=q2&q3=q4",
					output: "/cat?q1=q2&q3=q4",
				},
				{
					input:  "/foo/bar",
					output: "/cat/bar",
				},
			},
		},
		{
			name: "prefix path rewrite with explicit '/' on suffix",
			httpMatch: &gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  &prefix,
					Value: awssdk.String("/foo"),
				},
			},
			gwPathModifier: gwv1.HTTPPathModifier{
				Type:               gwv1.PrefixMatchHTTPPathModifier,
				ReplacePrefixMatch: awssdk.String("/cat/"),
			},
			expected: elbv2.Transform{
				Type: elbv2.TransformTypeUrlRewrite,
				UrlRewriteConfig: &elbv2.RewriteConfigObject{
					Rewrites: []elbv2.RewriteConfig{
						{
							Regex:   "(^/foo(/)?)",
							Replace: "/cat/",
						},
					},
				},
			},
			rewriteCases: []pathWriteCase{
				{
					input:  "/foo",
					output: "/cat/",
				},
				{
					input:  "/foo/bar/baz/bat/",
					output: "/cat/bar/baz/bat/",
				},
				{
					input:  "/foo?q1=q2",
					output: "/cat/?q1=q2",
				},
				{
					input:  "/foo?q1=q2&q3=q4",
					output: "/cat/?q1=q2&q3=q4",
				},
				{
					input:  "/foo/bar",
					output: "/cat/bar",
				},
			},
		},
		{
			name: "prefix path rewrite - empty replace",
			httpMatch: &gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  &prefix,
					Value: awssdk.String("/foo"),
				},
			},
			gwPathModifier: gwv1.HTTPPathModifier{
				Type:               gwv1.PrefixMatchHTTPPathModifier,
				ReplacePrefixMatch: awssdk.String(""),
			},
			expected: elbv2.Transform{
				Type: elbv2.TransformTypeUrlRewrite,
				UrlRewriteConfig: &elbv2.RewriteConfigObject{
					Rewrites: []elbv2.RewriteConfig{
						{
							Regex:   "(^/foo(/)?)",
							Replace: "/",
						},
					},
				},
			},
			rewriteCases: []pathWriteCase{
				{
					input:  "/foo",
					output: "/",
				},
				{
					input:  "/foo/bar/baz/bat/",
					output: "/bar/baz/bat/",
				},
				{
					input:  "/foo?q1=q2",
					output: "/?q1=q2",
				},
				{
					input:  "/foo?q1=q2&q3=q4",
					output: "/?q1=q2&q3=q4",
				},
				{
					input:  "/foo/bar",
					output: "/bar",
				},
				{
					input:  "/foo/bar/",
					output: "/bar/",
				},
			},
		},
		{
			name: "prefix path rewrite - '/' replace",
			httpMatch: &gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  &prefix,
					Value: awssdk.String("/foo"),
				},
			},
			gwPathModifier: gwv1.HTTPPathModifier{
				Type:               gwv1.PrefixMatchHTTPPathModifier,
				ReplacePrefixMatch: awssdk.String("/"),
			},
			expected: elbv2.Transform{
				Type: elbv2.TransformTypeUrlRewrite,
				UrlRewriteConfig: &elbv2.RewriteConfigObject{
					Rewrites: []elbv2.RewriteConfig{
						{
							Regex:   "(^/foo(/)?)",
							Replace: "/",
						},
					},
				},
			},
			rewriteCases: []pathWriteCase{
				{
					input:  "/foo",
					output: "/",
				},
				{
					input:  "/foo/bar/baz/bat/",
					output: "/bar/baz/bat/",
				},
				{
					input:  "/foo?q1=q2",
					output: "/?q1=q2",
				},
				{
					input:  "/foo?q1=q2&q3=q4",
					output: "/?q1=q2&q3=q4",
				},
				{
					input:  "/foo/bar",
					output: "/bar",
				},
				{
					input:  "/foo/bar/",
					output: "/bar/",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generateURLRewritePathTransform(tc.gwPathModifier, tc.httpMatch)
			assert.Equal(t, tc.expected, result)

			for _, rwCase := range tc.rewriteCases {
				re, err := regexp.Compile(result.UrlRewriteConfig.Rewrites[0].Regex)
				assert.NoError(t, err)
				rewriteValue := re.ReplaceAllString(rwCase.input, result.UrlRewriteConfig.Rewrites[0].Replace)
				assert.Equal(t, rwCase.output, rewriteValue)
			}
		})
	}
}

func Test_generateHostHeaderRewriteTransform(t *testing.T) {
	testCases := []struct {
		name     string
		hostname gwv1.PreciseHostname
		expected elbv2.Transform
	}{
		{
			name:     "with header rewrite",
			hostname: "foo.com",
			expected: elbv2.Transform{
				Type: elbv2.TransformTypeHostHeaderRewrite,
				HostHeaderRewriteConfig: &elbv2.RewriteConfigObject{
					Rewrites: []elbv2.RewriteConfig{
						{
							Regex:   ".*",
							Replace: "foo.com",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generateHostHeaderRewriteTransform(tc.hostname)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_BuildListenerRuleConfigTransforms(t *testing.T) {
	testCases := []struct {
		name     string
		rule     RulePrecedence
		expected []elbv2.Transform
	}{
		{
			name: "nil listener rule config",
			rule: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: convertHTTPRouteRule(&gwv1.HTTPRouteRule{}, nil, nil),
				},
			},
			expected: nil,
		},
		{
			name: "empty transforms in listener rule config",
			rule: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: convertHTTPRouteRule(&gwv1.HTTPRouteRule{}, nil, &elbv2gw.ListenerRuleConfiguration{
						Spec: elbv2gw.ListenerRuleConfigurationSpec{
							Transforms: []elbv2gw.ListenerRuleTransform{},
						},
					}),
				},
			},
			expected: nil,
		},
		{
			name: "host header rewrite transform from CRD",
			rule: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: convertHTTPRouteRule(&gwv1.HTTPRouteRule{}, nil, &elbv2gw.ListenerRuleConfiguration{
						Spec: elbv2gw.ListenerRuleConfigurationSpec{
							Transforms: []elbv2gw.ListenerRuleTransform{
								{
									Type: elbv2gw.ListenerRuleTransformTypeHostHeaderRewrite,
									HostHeaderRewriteConfig: &elbv2gw.ListenerRuleHostHeaderRewriteConfig{
										Rewrites: []elbv2gw.ListenerRuleRewriteConfig{
											{
												Regex:   ".*",
												Replace: "tenant.example.com",
											},
										},
									},
								},
							},
						},
					}),
				},
			},
			expected: []elbv2.Transform{
				{
					Type: elbv2.TransformTypeHostHeaderRewrite,
					HostHeaderRewriteConfig: &elbv2.RewriteConfigObject{
						Rewrites: []elbv2.RewriteConfig{
							{
								Regex:   ".*",
								Replace: "tenant.example.com",
							},
						},
					},
				},
			},
		},
		{
			name: "host header rewrite with source header from CRD",
			rule: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: convertHTTPRouteRule(&gwv1.HTTPRouteRule{}, nil, &elbv2gw.ListenerRuleConfiguration{
						Spec: elbv2gw.ListenerRuleConfigurationSpec{
							Transforms: []elbv2gw.ListenerRuleTransform{
								{
									Type: elbv2gw.ListenerRuleTransformTypeHostHeaderRewrite,
									HostHeaderRewriteConfig: &elbv2gw.ListenerRuleHostHeaderRewriteConfig{
										Rewrites: []elbv2gw.ListenerRuleRewriteConfig{
											{
												Regex:   ".*",
												Replace: "$0",
											},
										},
										SourceHeader: awssdk.String("X-School-Domain"),
									},
								},
							},
						},
					}),
				},
			},
			expected: []elbv2.Transform{
				{
					Type: elbv2.TransformTypeHostHeaderRewrite,
					HostHeaderRewriteConfig: &elbv2.RewriteConfigObject{
						Rewrites: []elbv2.RewriteConfig{
							{
								Regex:   ".*",
								Replace: "$0",
							},
						},
						SourceHeader: awssdk.String("X-School-Domain"),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := BuildListenerRuleConfigTransforms(tc.rule)
			assert.Equal(t, tc.expected, result)
		})
	}
}
