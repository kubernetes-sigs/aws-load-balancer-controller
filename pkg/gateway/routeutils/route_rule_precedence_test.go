package routeutils

import (
	"math"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	defaultHostname = "example.com"
)

func Test_SortAllRulesByPrecedence(t *testing.T) {

	httpOneRuleNoMatch := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{
				Name:      "httpOneRuleNoMatch",
				Namespace: "ns",
			},
		},
		rules: []RouteRule{
			&convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{},
			},
		},
	}

	httpOneRuleOneMatch := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{
				Name:      "httpOneRuleOneMatch",
				Namespace: "ns",
			},
		},
		rules: []RouteRule{
			&convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  (*gwv1.PathMatchType)(awssdk.String("Exact")),
								Value: awssdk.String("/foo"),
							},
						},
					},
				},
			},
		},
	}

	httpOneRuleMultipleMatches := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{
				Name:      "httpOneRuleMultipleMatches",
				Namespace: "ns",
			},
		},
		rules: []RouteRule{
			&convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  (*gwv1.PathMatchType)(awssdk.String("Exact")),
								Value: awssdk.String("/foo"),
							},
						},
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  (*gwv1.PathMatchType)(awssdk.String("PathPrefix")),
								Value: awssdk.String("/other-route"),
							},
						},
					},
				},
			},
		},
	}

	grpcOneRuleNoMatch := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{
				Name:      "grpcOneRuleNoMatch",
				Namespace: "ns",
			},
		},
		rules: []RouteRule{
			&convertedGRPCRouteRule{
				rule: &gwv1.GRPCRouteRule{},
			},
		},
	}

	grpcOneRuleOneMatch := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{
				Name:      "grpcOneRuleOneMatch",
				Namespace: "ns",
			},
		},
		rules: []RouteRule{
			&convertedGRPCRouteRule{
				rule: &gwv1.GRPCRouteRule{
					Matches: []gwv1.GRPCRouteMatch{
						{
							Method: &gwv1.GRPCMethodMatch{
								Type:    (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")),
								Service: awssdk.String("echo/echoservice"),
								Method:  awssdk.String("post"),
							},
						},
					},
				},
			},
		},
	}

	grpcOneRuleMultipleMatches := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{
				Name:      "grpcOneRuleMultipleMatches",
				Namespace: "ns",
			},
		},
		rules: []RouteRule{
			&convertedGRPCRouteRule{
				rule: &gwv1.GRPCRouteRule{
					Matches: []gwv1.GRPCRouteMatch{
						{
							Method: &gwv1.GRPCMethodMatch{
								Type:    (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")),
								Service: awssdk.String("echo/echoservice"),
								Method:  awssdk.String("post"),
							},
						},
						{
							Method: &gwv1.GRPCMethodMatch{
								Type:    (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")),
								Service: awssdk.String("echo/otherservice"),
								Method:  awssdk.String("othermethod"),
							},
						},
					},
				},
			},
		},
	}
	// Multi-route test fixtures: multiple HTTP routes
	httpCatchAll := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{Name: "http-catchall", Namespace: "ns"},
			Spec:       gwv1.HTTPRouteSpec{Hostnames: []gwv1.Hostname{"app.example.com"}},
		},
		rules: []RouteRule{&convertedHTTPRouteRule{rule: &gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{{Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("PathPrefix")), Value: awssdk.String("/")}}},
		}}},
	}
	httpUsers := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{Name: "http-users", Namespace: "ns"},
			Spec:       gwv1.HTTPRouteSpec{Hostnames: []gwv1.Hostname{"app.example.com"}},
		},
		rules: []RouteRule{&convertedHTTPRouteRule{rule: &gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{{Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("Exact")), Value: awssdk.String("/api/users")}}},
		}}},
	}
	httpAPI := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{Name: "http-api", Namespace: "ns"},
			Spec:       gwv1.HTTPRouteSpec{Hostnames: []gwv1.Hostname{"app.example.com"}},
		},
		rules: []RouteRule{&convertedHTTPRouteRule{rule: &gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{{Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("PathPrefix")), Value: awssdk.String("/api")}}},
		}}},
	}
	// Multi-route test fixtures: multiple GRPC routes
	grpcCatchAll := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{Name: "grpc-catchall", Namespace: "ns"},
			Spec:       gwv1.GRPCRouteSpec{Hostnames: []gwv1.Hostname{"grpc.example.com"}},
		},
		rules: []RouteRule{&convertedGRPCRouteRule{rule: &gwv1.GRPCRouteRule{}}},
	}
	grpcShortSvc := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{Name: "grpc-specific", Namespace: "ns"},
			Spec:       gwv1.GRPCRouteSpec{Hostnames: []gwv1.Hostname{"grpc.example.com"}},
		},
		rules: []RouteRule{&convertedGRPCRouteRule{rule: &gwv1.GRPCRouteRule{
			Matches: []gwv1.GRPCRouteMatch{{Method: &gwv1.GRPCMethodMatch{Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")), Service: awssdk.String("pkg.MyService"), Method: awssdk.String("DoWork")}}},
		}}},
	}
	grpcLongSvc := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{Name: "grpc-long-svc", Namespace: "ns"},
			Spec:       gwv1.GRPCRouteSpec{Hostnames: []gwv1.Hostname{"grpc.example.com"}},
		},
		rules: []RouteRule{&convertedGRPCRouteRule{rule: &gwv1.GRPCRouteRule{
			Matches: []gwv1.GRPCRouteMatch{{Method: &gwv1.GRPCMethodMatch{Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")), Service: awssdk.String("com.company.longpkg.MyService"), Method: awssdk.String("Execute")}}},
		}}},
	}
	// Multi-route test fixtures: mixed HTTP + GRPC
	httpRoot := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{Name: "http-root", Namespace: "ns"},
			Spec:       gwv1.HTTPRouteSpec{Hostnames: []gwv1.Hostname{"shared.example.com"}},
		},
		rules: []RouteRule{&convertedHTTPRouteRule{rule: &gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{{Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("PathPrefix")), Value: awssdk.String("/")}}},
		}}},
	}
	httpStatus := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{Name: "http-status", Namespace: "ns"},
			Spec:       gwv1.HTTPRouteSpec{Hostnames: []gwv1.Hostname{"shared.example.com"}},
		},
		rules: []RouteRule{&convertedHTTPRouteRule{rule: &gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{{Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("Exact")), Value: awssdk.String("/status")}}},
		}}},
	}
	grpcEcho := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{Name: "grpc-echo", Namespace: "ns"},
			Spec:       gwv1.GRPCRouteSpec{Hostnames: []gwv1.Hostname{"shared.example.com"}},
		},
		rules: []RouteRule{&convertedGRPCRouteRule{rule: &gwv1.GRPCRouteRule{
			Matches: []gwv1.GRPCRouteMatch{{Method: &gwv1.GRPCMethodMatch{Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")), Service: awssdk.String("my.EchoService"), Method: awssdk.String("Echo")}}},
		}}},
	}
	// Multi-route test fixtures: mixed HTTP (with method) + GRPC (with headers)
	httpWithMethod := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{Name: "http-with-method", Namespace: "ns"},
			Spec:       gwv1.HTTPRouteSpec{Hostnames: []gwv1.Hostname{"mixed.example.com"}},
		},
		rules: []RouteRule{&convertedHTTPRouteRule{rule: &gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{{
				Path:   &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("Exact")), Value: awssdk.String("/api/items")},
				Method: (*gwv1.HTTPMethod)(awssdk.String("POST")),
			}},
		}}},
	}
	httpNoMethod := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{Name: "http-no-method", Namespace: "ns"},
			Spec:       gwv1.HTTPRouteSpec{Hostnames: []gwv1.Hostname{"mixed.example.com"}},
		},
		rules: []RouteRule{&convertedHTTPRouteRule{rule: &gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{{
				Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("Exact")), Value: awssdk.String("/api/items")},
				Headers: []gwv1.HTTPHeaderMatch{
					{Name: "x-api-key", Value: "secret"},
					{Name: "x-version", Value: "v2"},
					{Name: "x-tenant", Value: "acme"},
				},
			}},
		}}},
	}
	grpcWithHeaders := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{Name: "grpc-with-headers", Namespace: "ns"},
			Spec:       gwv1.GRPCRouteSpec{Hostnames: []gwv1.Hostname{"mixed.example.com"}},
		},
		rules: []RouteRule{&convertedGRPCRouteRule{rule: &gwv1.GRPCRouteRule{
			Matches: []gwv1.GRPCRouteMatch{{
				Method: &gwv1.GRPCMethodMatch{Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")), Service: awssdk.String("api.Items"), Method: awssdk.String("")},
				Headers: []gwv1.GRPCHeaderMatch{
					{Name: "x-request-id", Value: "abc"},
					{Name: "x-trace", Value: "on"},
				},
			}},
		}}},
	}
	// Multi-route test fixtures: mixed routes with different path match types
	// Tests: prefix HTTP vs exact GRPC, regex GRPC vs exact HTTP, HTTP with query params
	httpPrefixAPI := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{Name: "http-prefix-api", Namespace: "ns"},
			Spec:       gwv1.HTTPRouteSpec{Hostnames: []gwv1.Hostname{"variety.example.com"}},
		},
		rules: []RouteRule{&convertedHTTPRouteRule{rule: &gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{{
				Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("PathPrefix")), Value: awssdk.String("/api/v2/orders")},
			}},
		}}},
	}
	httpExactWithQueryParams := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{Name: "http-exact-qp", Namespace: "ns"},
			Spec:       gwv1.HTTPRouteSpec{Hostnames: []gwv1.Hostname{"variety.example.com"}},
		},
		rules: []RouteRule{&convertedHTTPRouteRule{rule: &gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{{
				Path:        &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("Exact")), Value: awssdk.String("/search")},
				QueryParams: []gwv1.HTTPQueryParamMatch{{Name: "q", Value: "test"}, {Name: "page", Value: "1"}},
			}},
		}}},
	}
	grpcRegexRoute := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{Name: "grpc-regex", Namespace: "ns"},
			Spec:       gwv1.GRPCRouteSpec{Hostnames: []gwv1.Hostname{"variety.example.com"}},
		},
		rules: []RouteRule{&convertedGRPCRouteRule{rule: &gwv1.GRPCRouteRule{
			Matches: []gwv1.GRPCRouteMatch{{
				Method: &gwv1.GRPCMethodMatch{Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("RegularExpression")), Service: awssdk.String("com.api.*"), Method: awssdk.String("Get.*")},
			}},
		}}},
	}
	grpcExactLong := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{Name: "grpc-exact-long", Namespace: "ns"},
			Spec:       gwv1.GRPCRouteSpec{Hostnames: []gwv1.Hostname{"variety.example.com"}},
		},
		rules: []RouteRule{&convertedGRPCRouteRule{rule: &gwv1.GRPCRouteRule{
			Matches: []gwv1.GRPCRouteMatch{{
				Method: &gwv1.GRPCMethodMatch{Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")), Service: awssdk.String("com.company.api.OrderService"), Method: awssdk.String("CreateOrder")},
			}},
		}}},
	}

	testCases := []struct {
		name   string
		input  []RouteDescriptor
		output []RulePrecedence
		port   int32
	}{
		{
			name:  "no routes",
			input: make([]RouteDescriptor, 0),
		},
		{
			name: "one http route, no rules attached",
			input: []RouteDescriptor{
				&httpRouteDescription{
					route: &gwv1.HTTPRoute{
						ObjectMeta: v1.ObjectMeta{
							Name:      "http1",
							Namespace: "ns",
						},
					},
				},
			},
		},
		{
			name: "one http route, one rule attached",
			input: []RouteDescriptor{
				httpOneRuleNoMatch,
			},
			output: []RulePrecedence{
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteNamespacedName:  "ns/httpOneRuleNoMatch",
						RouteDescriptor:      httpOneRuleNoMatch,
						Rule:                 httpOneRuleNoMatch.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     math.MaxInt,
						RouteCreateTimestamp: httpOneRuleNoMatch.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{},
					HTTPMatch:        &gwv1.HTTPRouteMatch{},
				},
			},
		},
		{
			name: "one http route, one rule attached with match",
			input: []RouteDescriptor{
				httpOneRuleOneMatch,
			},
			output: []RulePrecedence{
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteNamespacedName:  "ns/httpOneRuleOneMatch",
						RouteDescriptor:      httpOneRuleOneMatch,
						Rule:                 httpOneRuleOneMatch.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpOneRuleOneMatch.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{
						PathType:   3,
						PathLength: 4,
					},
					HTTPMatch: &gwv1.HTTPRouteMatch{
						Path: &gwv1.HTTPPathMatch{
							Type:  (*gwv1.PathMatchType)(awssdk.String("Exact")),
							Value: awssdk.String("/foo"),
						},
					},
				},
			},
		},
		{
			name: "one http route, one rule attached with multiple matches",
			input: []RouteDescriptor{
				httpOneRuleMultipleMatches,
			},
			output: []RulePrecedence{
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteNamespacedName:  "ns/httpOneRuleMultipleMatches",
						RouteDescriptor:      httpOneRuleMultipleMatches,
						Rule:                 httpOneRuleMultipleMatches.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpOneRuleMultipleMatches.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{
						PathType:   3,
						PathLength: 4,
					},
					HTTPMatch: &gwv1.HTTPRouteMatch{
						Path: &gwv1.HTTPPathMatch{
							Type:  (*gwv1.PathMatchType)(awssdk.String("Exact")),
							Value: awssdk.String("/foo"),
						},
					},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteNamespacedName:  "ns/httpOneRuleMultipleMatches",
						RouteDescriptor:      httpOneRuleMultipleMatches,
						Rule:                 httpOneRuleMultipleMatches.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     1,
						RouteCreateTimestamp: httpOneRuleMultipleMatches.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{
						PathType:   2,
						PathLength: 12,
					},
					HTTPMatch: &gwv1.HTTPRouteMatch{
						Path: &gwv1.HTTPPathMatch{
							Type:  (*gwv1.PathMatchType)(awssdk.String("PathPrefix")),
							Value: awssdk.String("/other-route"),
						},
					},
				},
			},
		},
		{
			name: "one grpc route, no rules attached",
			input: []RouteDescriptor{
				&grpcRouteDescription{
					route: &gwv1.GRPCRoute{
						ObjectMeta: v1.ObjectMeta{
							Name:      "grpc1",
							Namespace: "ns",
						},
					},
				},
			},
		},
		{
			name: "one grpc route, one rule attached",
			input: []RouteDescriptor{
				grpcOneRuleNoMatch,
			},
			output: []RulePrecedence{
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteNamespacedName:  "ns/grpcOneRuleNoMatch",
						RouteDescriptor:      grpcOneRuleNoMatch,
						Rule:                 grpcOneRuleNoMatch.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     math.MaxInt,
						RouteCreateTimestamp: grpcOneRuleNoMatch.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{},
					GRPCMatch:        &gwv1.GRPCRouteMatch{},
				},
			},
		},
		{
			name: "one grpc route, one rule attached with match",
			input: []RouteDescriptor{
				grpcOneRuleOneMatch,
			},
			output: []RulePrecedence{
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteNamespacedName:  "ns/grpcOneRuleOneMatch",
						RouteDescriptor:      grpcOneRuleOneMatch,
						Rule:                 grpcOneRuleOneMatch.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: grpcOneRuleOneMatch.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{
						PathType:        3,
						PathLength:      16,
						SecondaryLength: 4,
					},
					GRPCMatch: &gwv1.GRPCRouteMatch{
						Method: &gwv1.GRPCMethodMatch{
							Type:    (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")),
							Service: awssdk.String("echo/echoservice"),
							Method:  awssdk.String("post"),
						},
					},
				},
			},
		},
		{
			name: "one grpc route, one rule attached with multiple matches",
			input: []RouteDescriptor{
				grpcOneRuleMultipleMatches,
			},
			output: []RulePrecedence{
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteNamespacedName:  "ns/grpcOneRuleMultipleMatches",
						RouteDescriptor:      grpcOneRuleMultipleMatches,
						Rule:                 grpcOneRuleMultipleMatches.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     1,
						RouteCreateTimestamp: grpcOneRuleMultipleMatches.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{
						PathType:        3,
						PathLength:      17,
						SecondaryLength: 11,
					},
					GRPCMatch: &gwv1.GRPCRouteMatch{
						Method: &gwv1.GRPCMethodMatch{
							Type:    (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")),
							Service: awssdk.String("echo/otherservice"),
							Method:  awssdk.String("othermethod"),
						},
					},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteNamespacedName:  "ns/grpcOneRuleMultipleMatches",
						RouteDescriptor:      grpcOneRuleMultipleMatches,
						Rule:                 grpcOneRuleMultipleMatches.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: grpcOneRuleMultipleMatches.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{
						PathType:        3,
						PathLength:      16,
						SecondaryLength: 4,
					},
					GRPCMatch: &gwv1.GRPCRouteMatch{
						Method: &gwv1.GRPCMethodMatch{
							Type:    (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")),
							Service: awssdk.String("echo/echoservice"),
							Method:  awssdk.String("post"),
						},
					},
				},
			},
		},
		{
			name: "multiple http routes sorted by specificity",
			input: []RouteDescriptor{
				httpCatchAll,
				httpUsers,
				httpAPI,
			},
			output: []RulePrecedence{
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      httpUsers,
						Rule:                 httpUsers.rules[0],
						Hostname:             "app.example.com",
						RouteNamespacedName:  "ns/http-users",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpUsers.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 3, PathLength: 10},
					HTTPMatch:        &gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("Exact")), Value: awssdk.String("/api/users")}},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      httpAPI,
						Rule:                 httpAPI.rules[0],
						Hostname:             "app.example.com",
						RouteNamespacedName:  "ns/http-api",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpAPI.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 2, PathLength: 4},
					HTTPMatch:        &gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("PathPrefix")), Value: awssdk.String("/api")}},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      httpCatchAll,
						Rule:                 httpCatchAll.rules[0],
						Hostname:             "app.example.com",
						RouteNamespacedName:  "ns/http-catchall",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpCatchAll.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 2, PathLength: 1},
					HTTPMatch:        &gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("PathPrefix")), Value: awssdk.String("/")}},
				},
			},
		},
		{
			name: "multiple grpc routes sorted by specificity",
			input: []RouteDescriptor{
				grpcCatchAll,
				grpcShortSvc,
				grpcLongSvc,
			},
			output: []RulePrecedence{
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      grpcLongSvc,
						Rule:                 grpcLongSvc.rules[0],
						Hostname:             "grpc.example.com",
						RouteNamespacedName:  "ns/grpc-long-svc",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: grpcLongSvc.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 3, PathLength: 29, SecondaryLength: 7},
					GRPCMatch: &gwv1.GRPCRouteMatch{Method: &gwv1.GRPCMethodMatch{
						Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")), Service: awssdk.String("com.company.longpkg.MyService"), Method: awssdk.String("Execute"),
					}},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      grpcShortSvc,
						Rule:                 grpcShortSvc.rules[0],
						Hostname:             "grpc.example.com",
						RouteNamespacedName:  "ns/grpc-specific",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: grpcShortSvc.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 3, PathLength: 13, SecondaryLength: 6},
					GRPCMatch: &gwv1.GRPCRouteMatch{Method: &gwv1.GRPCMethodMatch{
						Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")), Service: awssdk.String("pkg.MyService"), Method: awssdk.String("DoWork"),
					}},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      grpcCatchAll,
						Rule:                 grpcCatchAll.rules[0],
						Hostname:             "grpc.example.com",
						RouteNamespacedName:  "ns/grpc-catchall",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     math.MaxInt,
						RouteCreateTimestamp: grpcCatchAll.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{},
					GRPCMatch:        &gwv1.GRPCRouteMatch{},
				},
			},
		},
		{
			name: "mixed http and grpc routes sorted by cross-kind specificity",
			input: []RouteDescriptor{
				httpRoot,
				grpcEcho,
				httpStatus,
			},
			output: []RulePrecedence{
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      grpcEcho,
						Rule:                 grpcEcho.rules[0],
						Hostname:             "shared.example.com",
						RouteNamespacedName:  "ns/grpc-echo",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: grpcEcho.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 3, PathLength: 14, SecondaryLength: 4},
					GRPCMatch: &gwv1.GRPCRouteMatch{Method: &gwv1.GRPCMethodMatch{
						Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")), Service: awssdk.String("my.EchoService"), Method: awssdk.String("Echo"),
					}},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      httpStatus,
						Rule:                 httpStatus.rules[0],
						Hostname:             "shared.example.com",
						RouteNamespacedName:  "ns/http-status",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpStatus.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 3, PathLength: 7},
					HTTPMatch:        &gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("Exact")), Value: awssdk.String("/status")}},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      httpRoot,
						Rule:                 httpRoot.rules[0],
						Hostname:             "shared.example.com",
						RouteNamespacedName:  "ns/http-root",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpRoot.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 2, PathLength: 1},
					HTTPMatch:        &gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("PathPrefix")), Value: awssdk.String("/")}},
				},
			},
		},
		{
			name: "mixed routes with method-vs-header tie on same path length",
			input: []RouteDescriptor{
				grpcWithHeaders,
				httpNoMethod,
				httpWithMethod,
			},
			output: []RulePrecedence{
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      httpWithMethod,
						Rule:                 httpWithMethod.rules[0],
						Hostname:             "mixed.example.com",
						RouteNamespacedName:  "ns/http-with-method",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpWithMethod.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 3, PathLength: 10, HasMethod: true},
					HTTPMatch: &gwv1.HTTPRouteMatch{
						Path:   &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("Exact")), Value: awssdk.String("/api/items")},
						Method: (*gwv1.HTTPMethod)(awssdk.String("POST")),
					},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      httpNoMethod,
						Rule:                 httpNoMethod.rules[0],
						Hostname:             "mixed.example.com",
						RouteNamespacedName:  "ns/http-no-method",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpNoMethod.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 3, PathLength: 10, HeaderCount: 3},
					HTTPMatch: &gwv1.HTTPRouteMatch{
						Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("Exact")), Value: awssdk.String("/api/items")},
						Headers: []gwv1.HTTPHeaderMatch{
							{Name: "x-api-key", Value: "secret"},
							{Name: "x-version", Value: "v2"},
							{Name: "x-tenant", Value: "acme"},
						},
					},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      grpcWithHeaders,
						Rule:                 grpcWithHeaders.rules[0],
						Hostname:             "mixed.example.com",
						RouteNamespacedName:  "ns/grpc-with-headers",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: grpcWithHeaders.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 3, PathLength: 9, SecondaryLength: 0, HeaderCount: 2},
					GRPCMatch: &gwv1.GRPCRouteMatch{
						Method: &gwv1.GRPCMethodMatch{Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")), Service: awssdk.String("api.Items"), Method: awssdk.String("")},
						Headers: []gwv1.GRPCHeaderMatch{
							{Name: "x-request-id", Value: "abc"},
							{Name: "x-trace", Value: "on"},
						},
					},
				},
			},
		},
		{
			// This test verifies deterministic cross-kind ordering with varied match types:
			// - GRPC exact with long path (highest specificity)
			// - HTTP exact with query params
			// - HTTP prefix with long path (prefix < exact in path type)
			// - GRPC regex (lowest path type among non-empty matches)
			// Expected order: exact types first sorted by path length, then prefix, then regex
			name: "mixed routes with different path match types",
			input: []RouteDescriptor{
				httpPrefixAPI,
				grpcRegexRoute,
				grpcExactLong,
				httpExactWithQueryParams,
			},
			output: []RulePrecedence{
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      grpcExactLong,
						Rule:                 grpcExactLong.rules[0],
						Hostname:             "variety.example.com",
						RouteNamespacedName:  "ns/grpc-exact-long",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: grpcExactLong.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 3, PathLength: 28, SecondaryLength: 11},
					GRPCMatch: &gwv1.GRPCRouteMatch{
						Method: &gwv1.GRPCMethodMatch{Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")), Service: awssdk.String("com.company.api.OrderService"), Method: awssdk.String("CreateOrder")},
					},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      httpExactWithQueryParams,
						Rule:                 httpExactWithQueryParams.rules[0],
						Hostname:             "variety.example.com",
						RouteNamespacedName:  "ns/http-exact-qp",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpExactWithQueryParams.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 3, PathLength: 7, QueryParamCount: 2},
					HTTPMatch: &gwv1.HTTPRouteMatch{
						Path:        &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("Exact")), Value: awssdk.String("/search")},
						QueryParams: []gwv1.HTTPQueryParamMatch{{Name: "q", Value: "test"}, {Name: "page", Value: "1"}},
					},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      httpPrefixAPI,
						Rule:                 httpPrefixAPI.rules[0],
						Hostname:             "variety.example.com",
						RouteNamespacedName:  "ns/http-prefix-api",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpPrefixAPI.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 2, PathLength: 14},
					HTTPMatch: &gwv1.HTTPRouteMatch{
						Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String("PathPrefix")), Value: awssdk.String("/api/v2/orders")},
					},
				},
				{
					CommonRulePrecedence: CommonRulePrecedence{
						RouteDescriptor:      grpcRegexRoute,
						Rule:                 grpcRegexRoute.rules[0],
						Hostname:             "variety.example.com",
						RouteNamespacedName:  "ns/grpc-regex",
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: grpcRegexRoute.GetRouteCreateTimestamp(),
					},
					PrecedenceFactor: &RulePrecedenceFactor{PathType: 1, PathLength: 9, SecondaryLength: 5},
					GRPCMatch: &gwv1.GRPCRouteMatch{
						Method: &gwv1.GRPCMethodMatch{Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("RegularExpression")), Service: awssdk.String("com.api.*"), Method: awssdk.String("Get.*")},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := SortAllRulesByPrecedence(tc.input, tc.port)
			assert.Equal(t, tc.output, result)
		})
	}
}

func Test_compareRulePrecedence_HTTP(t *testing.T) {
	tests := []struct {
		name    string
		ruleOne RulePrecedence
		ruleTwo RulePrecedence
		want    bool
		reason  string
	}{
		{
			name: "hostname - exact vs wildcard",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "api.example.com",
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "*.example.com",
				},
			},
			want:   true,
			reason: "exact hostname has higher precedence than wildcard",
		},
		{
			name: "path type - exact vs prefix",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType: 3,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType: 1,
				},
			},
			want:   true,
			reason: "exact path has higher precedence than prefix",
		},
		{
			name: "path length precedence",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   1,
					PathLength: 10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   1,
					PathLength: 5,
				},
			},
			want:   true,
			reason: "longer path has higher precedence",
		},
		{
			name: "http route method precedence",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   1,
					PathLength: 5,
					HasMethod:  true,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   1,
					PathLength: 5,
					HasMethod:  false,
				},
			},
			want:   true,
			reason: "rule with method has higher precedence",
		},
		{
			name: "host-specific vs catch-all (empty hostname) - equal path, catch-all older",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             defaultHostname,
					RouteCreateTimestamp: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   2,
					PathLength: 1,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					RouteCreateTimestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   2,
					PathLength: 1,
				},
			},
			want:   true,
			reason: "host-specific rule outranks catch-all even when the catch-all is older and paths are equal",
		},
		{
			name: "catch-all (empty hostname) vs host-specific - equal path, catch-all older",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					RouteCreateTimestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   2,
					PathLength: 1,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             defaultHostname,
					RouteCreateTimestamp: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   2,
					PathLength: 1,
				},
			},
			want:   false,
			reason: "catch-all with an empty hostname list is least specific and must not outrank a host-specific rule",
		},
		{
			name: "host-specific (short path) vs catch-all (empty hostname, long path)",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   2,
					PathLength: 1,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   2,
					PathLength: 20,
				},
			},
			want:   true,
			reason: "host-specific rule outranks catch-all even when the catch-all has a longer path",
		},
		{
			// Regression for the HTTPRouteMatchingAcrossRoutes conformance test:
			// With per-hostname splitting each unit carries a single hostname; when
			// the hostname ties, the longer path wins.
			name: "same hostname, longer path wins",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "example.com",
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   2, // prefix "/v2"
					PathLength: 3,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "example.com",
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   2, // prefix "/"
					PathLength: 1,
				},
			},
			want:   true,
			reason: "longer path wins when the hostname ties",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareRulePrecedenceUnified(tt.ruleOne, tt.ruleTwo)
			assert.Equal(t, tt.want, got, tt.reason)
		})
	}
}

func Test_compareRulePrecedence_GRPC(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	earlier := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		ruleOne RulePrecedence
		ruleTwo RulePrecedence
		want    bool
		reason  string
	}{
		{
			name: "hostname - exact vs wildcard",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "api.example.com",
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "*.example.com",
				},
			},
			want:   true,
			reason: "exact hostname has higher precedence than wildcard",
		},
		{
			name: "grpc route service precedence",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   1,
					PathLength: 10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   1,
					PathLength: 5,
				},
			},
			want:   true,
			reason: "rule with longer service length has higher precedence",
		},
		{
			name: "grpc header count precedence",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:    1,
					HeaderCount: 10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:    1,
					HeaderCount: 5,
				},
			},
			want:   true,
			reason: "more headers has higher precedence",
		},
		{
			name: "grpc method precedence",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        1,
					SecondaryLength: 10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        1,
					SecondaryLength: 5,
				},
			},
			want:   true,
			reason: "rules with longer method length has higher precedence",
		},
		{
			name: "grpc service precedence over method",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   1,
					PathLength: 5,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: defaultHostname,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        1,
					SecondaryLength: 10,
				},
			},
			want:   true,
			reason: "rules with service has higher precedence than method",
		},
		{
			name: "creation timestamp precedence",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             defaultHostname,
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        1,
					PathLength:      10,
					SecondaryLength: 10,
					HeaderCount:     10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             defaultHostname,
					RouteCreateTimestamp: now,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        1,
					PathLength:      10,
					SecondaryLength: 10,
					HeaderCount:     10,
				},
			},
			want:   true,
			reason: "earlier creation time has higher precedence",
		},
		{
			name: "rule index precedence",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:         defaultHostname,
					RuleIndexInRoute: 1,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        1,
					PathLength:      10,
					SecondaryLength: 10,
					HeaderCount:     10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:         defaultHostname,
					RuleIndexInRoute: 3,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        1,
					PathLength:      10,
					SecondaryLength: 10,
					HeaderCount:     10,
				},
			},
			want:   true,
			reason: "lower rule index has higher precedence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareRulePrecedenceUnified(tt.ruleOne, tt.ruleTwo)
			assert.Equal(t, tt.want, got, tt.reason)
		})
	}
}

// Test getHostnamePrecedenceOrder
func Test_getHostnamePrecedenceOrder(t *testing.T) {
	tests := []struct {
		name        string
		hostnameOne string
		hostnameTwo string
		want        int
		description string
	}{
		{
			name:        "non-wildcard vs wildcard",
			hostnameOne: "example.com",
			hostnameTwo: "*.example.com",
			want:        -1,
			description: "non-wildcard should have higher precedence than wildcard",
		},
		{
			name:        "wildcard vs non-wildcard",
			hostnameOne: "*.example.com",
			hostnameTwo: "example.com",
			want:        1,
			description: "wildcard should have lower precedence than non-wildcard",
		},
		{
			name:        "both non-wildcard - first longer",
			hostnameOne: "test.example.com",
			hostnameTwo: "example.com",
			want:        -1,
			description: "longer hostname should have higher precedence",
		},
		{
			name:        "both non-wildcard - second longer",
			hostnameOne: "example.com",
			hostnameTwo: "test.example.com",
			want:        1,
			description: "shorter hostname should have lower precedence",
		},
		{
			name:        "both wildcard - first longer",
			hostnameOne: "*.test.example.com",
			hostnameTwo: "*.example.com",
			want:        -1,
			description: "longer wildcard hostname should have higher precedence",
		},
		{
			name:        "both wildcard - second longer",
			hostnameOne: "*.example.com",
			hostnameTwo: "*.test.example.com",
			want:        1,
			description: "shorter wildcard hostname should have lower precedence",
		},
		{
			name:        "equal length non-wildcard",
			hostnameOne: "test1.com",
			hostnameTwo: "test2.com",
			want:        0,
			description: "equal length hostnames should have equal precedence",
		},
		{
			name:        "equal length wildcard",
			hostnameOne: "*.test1.com",
			hostnameTwo: "*.test2.com",
			want:        0,
			description: "equal length wildcard hostnames should have equal precedence",
		},
		{
			name:        "empty strings",
			hostnameOne: "",
			hostnameTwo: "",
			want:        0,
			description: "empty strings should have equal precedence",
		},
		{
			name:        "one empty string - first",
			hostnameOne: "",
			hostnameTwo: "example.com",
			want:        1,
			description: "empty string should have lower precedence",
		},
		{
			name:        "one empty string - second",
			hostnameOne: "example.com",
			hostnameTwo: "",
			want:        -1,
			description: "non-empty string should have higher precedence than empty",
		},
		{
			name:        "empty vs wildcard - empty is catch-all, least specific",
			hostnameOne: "",
			hostnameTwo: "*.example.com",
			want:        1,
			description: "an empty hostname (catch-all) must lose even to a wildcard",
		},
		{
			name:        "wildcard vs empty - wildcard beats catch-all",
			hostnameOne: "*.example.com",
			hostnameTwo: "",
			want:        -1,
			description: "a wildcard must beat an empty hostname (catch-all)",
		},
		{
			name:        "one hostname has more dots",
			hostnameOne: "*.example.com",
			hostnameTwo: "*.t.exa.com",
			want:        1,
			description: "hostname with more dots should have higher precedence even if it has less character",
		},
		{
			name:        "two hostnames have same number of dots, one has more characters",
			hostnameOne: "*.t.example.com",
			hostnameTwo: "*.t.exa.com",
			want:        -1,
			description: "hostname with more characters should have higher precedence order if they have same number of dots",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getHostnamePrecedenceOrder(tt.hostnameOne, tt.hostnameTwo)
			if got != tt.want {
				t.Errorf("GetHostnamePrecedenceOrder() = %v, want %v\nDescription: %s\nHostname1: %q\nHostname2: %q",
					got, tt.want, tt.description, tt.hostnameOne, tt.hostnameTwo)
			}
		})
	}
}

func Test_compareRulePrecedence_CrossKind(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	earlier := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		ruleOne RulePrecedence
		ruleTwo RulePrecedence
		want    bool
		reason  string
	}{
		{
			name: "specific GRPCRoute wins over catch-all HTTPRoute - reported vulnerability scenario",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: now, // attacker route is newer
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   2, // prefix
					PathLength: 1, // "/" which becomes "/*"
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier, // victim route is older
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        3,  // exact
					PathLength:      20, // "victim.pkg.EchoService"
					SecondaryLength: 4,  // "Echo"
				},
			},
			want:   false,
			reason: "specific GRPCRoute (exact, long path) must have higher precedence than catch-all HTTPRoute (prefix, short path)",
		},
		{
			name: "specific GRPCRoute higher precedence than catch-all HTTPRoute (reversed arg order)",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        3,  // exact
					PathLength:      20, // "victim.pkg.EchoService"
					SecondaryLength: 4,  // "Echo"
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: now,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   2, // prefix
					PathLength: 1, // "/"
				},
			},
			want:   true,
			reason: "specific GRPCRoute (exact) must outrank catch-all HTTPRoute (prefix)",
		},
		{
			name: "exact HTTPRoute wins over regex GRPCRoute",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "api.example.com",
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   3,  // exact
					PathLength: 20, // "/specific/endpoint"
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "api.example.com",
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        1, // regex
					PathLength:      5,
					SecondaryLength: 3,
				},
			},
			want:   true,
			reason: "exact HTTP path has higher precedence than regex GRPC",
		},
		{
			name: "same path type and length - falls through to timestamp (HTTP older wins)",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   3, // exact
					PathLength: 10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: now,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        3, // exact
					PathLength:      5,
					SecondaryLength: 3, // effective length = 5+3+2 = 10
				},
			},
			want:   true,
			reason: "when specificity is equal, older route (earlier timestamp) wins",
		},
		{
			name: "same path type and length - falls through to timestamp (GRPC older wins)",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: now,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   3, // exact
					PathLength: 10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   3,  // exact
					PathLength: 10, // GRPC service length 10, no method — same specificity as HTTP path length 10
				},
			},
			want:   false,
			reason: "when specificity is equal, older route (earlier timestamp) wins - GRPC is older here",
		},
		{
			name: "GRPC longer effective path wins over shorter HTTP path (same type)",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "shared.example.com",
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   3, // exact
					PathLength: 5,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "shared.example.com",
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        3,  // exact
					PathLength:      15, // effective = 15+5+2 = 22
					SecondaryLength: 5,
				},
			},
			want:   false,
			reason: "GRPC with longer effective path wins over HTTP with shorter path",
		},
		{
			name: "HTTP with headers wins over GRPC without headers (same path specificity)",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "shared.example.com",
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:    3, // exact
					PathLength:  10,
					HeaderCount: 2,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "shared.example.com",
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        3, // exact
					PathLength:      5,
					SecondaryLength: 3, // effective = 10
					HeaderCount:     0,
				},
			},
			want:   true,
			reason: "more headers gives higher precedence when path specificity is equal",
		},
		{
			name: "hostname specificity takes priority over path specificity across kinds",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "specific.example.com",
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:   2, // prefix
					PathLength: 1,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname: "*.example.com",
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        3, // exact
					PathLength:      20,
					SecondaryLength: 10,
				},
			},
			want:   true,
			reason: "exact hostname always wins over wildcard hostname regardless of path specificity",
		},
		{
			name: "HTTP with method beats GRPC with more headers (hasMethod before headerCount)",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:    3,
					PathLength:  10,
					HasMethod:   true,
					HeaderCount: 0,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        3,
					PathLength:      4,
					SecondaryLength: 4, // effective=10
					HeaderCount:     5,
				},
			},
			want:   true,
			reason: "HTTP with hasMethod=true wins over GRPC with more headers because hasMethod is checked before headerCount",
		},
		{
			name: "GRPC with many headers does NOT beat HTTP with method",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        3,
					PathLength:      4,
					SecondaryLength: 4, // effective=10
					HeaderCount:     10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:    3,
					PathLength:  10,
					HasMethod:   true,
					HeaderCount: 0,
				},
			},
			want:   false,
			reason: "GRPC (no method flag) cannot beat HTTP with hasMethod=true regardless of header count",
		},
		{
			name: "both without method - header count decides cross-kind",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:    3,
					PathLength:  10,
					HasMethod:   false,
					HeaderCount: 3,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        3,
					PathLength:      4,
					SecondaryLength: 4, // effective=10
					HeaderCount:     1,
				},
			},
			want:   true,
			reason: "when both have hasMethod=false, header count decides (3>1)",
		},
		{
			name: "GRPC with more headers vs HTTP no method fewer headers",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:    3,
					PathLength:  10, // GRPC service length 10, no method — same specificity as HTTP path length 10
					HeaderCount: 4,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:    3,
					PathLength:  10,
					HasMethod:   false,
					HeaderCount: 1,
				},
			},
			want:   true,
			reason: "when neither has method, GRPC with more headers (4>1) wins",
		},
		{
			name: "HTTP no method vs GRPC equal headers - query params break tie",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        3,
					PathLength:      10,
					HasMethod:       false,
					HeaderCount:     2,
					QueryParamCount: 3,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:        3,
					PathLength:      4,
					SecondaryLength: 4, // effective=10
					HeaderCount:     2,
				},
			},
			want:   true,
			reason: "when hasMethod and headerCount both tie, query params break the tie (HTTP has 3, GRPC has 0)",
		},
		{
			name: "all specificity factors equal - namespaced name breaks tie",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
					RouteNamespacedName:  "ns/http-rule",
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:    3,
					PathLength:  10,
					HasMethod:   false,
					HeaderCount: 2,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostname:             "shared.example.com",
					RouteCreateTimestamp: earlier,
					RouteNamespacedName:  "ns/grpc-rule",
				},
				PrecedenceFactor: &RulePrecedenceFactor{
					PathType:    3,
					PathLength:  10, // GRPC service length 10, no method — same specificity as HTTP path length 10
					HeaderCount: 2,
				},
			},
			want:   false,
			reason: "all specificity factors equal, falls to namespaced name: 'ns/grpc-rule' < 'ns/http-rule' alphabetically so GRPC wins",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareRulePrecedenceUnified(tt.ruleOne, tt.ruleTwo)
			assert.Equal(t, tt.want, got, tt.reason)
		})
	}
}

func Test_SortAllRulesByPrecedence_CrossKind(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// --- Routes sharing hostname "api.example.com" ---

	// HTTP catch-all: PathPrefix "/" (newest route)
	httpCatchAll := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{
				Name:              "http-catchall",
				Namespace:         "team-a",
				CreationTimestamp: v1.Time{Time: t3},
			},
			Spec: gwv1.HTTPRouteSpec{
				Hostnames: []gwv1.Hostname{"api.example.com"},
			},
		},
		rules: []RouteRule{
			&convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  (*gwv1.PathMatchType)(awssdk.String("PathPrefix")),
								Value: awssdk.String("/"),
							},
						},
					},
				},
			},
		},
	}

	// HTTP specific path: Exact "/health"
	httpHealth := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{
				Name:              "http-health",
				Namespace:         "team-a",
				CreationTimestamp: v1.Time{Time: t2},
			},
			Spec: gwv1.HTTPRouteSpec{
				Hostnames: []gwv1.Hostname{"api.example.com"},
			},
		},
		rules: []RouteRule{
			&convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  (*gwv1.PathMatchType)(awssdk.String("Exact")),
								Value: awssdk.String("/health"),
							},
						},
					},
				},
			},
		},
	}

	// HTTP with prefix "/api/v1" (medium specificity)
	httpAPIv1 := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{
				Name:              "http-api-v1",
				Namespace:         "team-b",
				CreationTimestamp: v1.Time{Time: t1},
			},
			Spec: gwv1.HTTPRouteSpec{
				Hostnames: []gwv1.Hostname{"api.example.com"},
			},
		},
		rules: []RouteRule{
			&convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  (*gwv1.PathMatchType)(awssdk.String("PathPrefix")),
								Value: awssdk.String("/api/v1"),
							},
						},
					},
				},
			},
		},
	}

	// GRPC exact method: "com.example.UserService/GetUser" (very specific)
	grpcGetUser := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{
				Name:              "grpc-user-svc",
				Namespace:         "team-c",
				CreationTimestamp: v1.Time{Time: t2},
			},
			Spec: gwv1.GRPCRouteSpec{
				Hostnames: []gwv1.Hostname{"api.example.com"},
			},
		},
		rules: []RouteRule{
			&convertedGRPCRouteRule{
				rule: &gwv1.GRPCRouteRule{
					Matches: []gwv1.GRPCRouteMatch{
						{
							Method: &gwv1.GRPCMethodMatch{
								Type:    (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")),
								Service: awssdk.String("com.example.UserService"),
								Method:  awssdk.String("GetUser"),
							},
						},
					},
				},
			},
		},
	}

	// GRPC service-only match: "com.example.OrderService" (no method — less specific than full match)
	grpcOrderSvc := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{
				Name:              "grpc-order-svc",
				Namespace:         "team-c",
				CreationTimestamp: v1.Time{Time: t1},
			},
			Spec: gwv1.GRPCRouteSpec{
				Hostnames: []gwv1.Hostname{"api.example.com"},
			},
		},
		rules: []RouteRule{
			&convertedGRPCRouteRule{
				rule: &gwv1.GRPCRouteRule{
					Matches: []gwv1.GRPCRouteMatch{
						{
							Method: &gwv1.GRPCMethodMatch{
								Type:    (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact")),
								Service: awssdk.String("com.example.OrderService"),
								Method:  awssdk.String("PlaceOrder"),
							},
						},
					},
				},
			},
		},
	}

	// GRPC catch-all (no method specified — matches all gRPC)
	grpcCatchAll := &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: v1.ObjectMeta{
				Name:              "grpc-catchall",
				Namespace:         "team-d",
				CreationTimestamp: v1.Time{Time: t3},
			},
			Spec: gwv1.GRPCRouteSpec{
				Hostnames: []gwv1.Hostname{"api.example.com"},
			},
		},
		rules: []RouteRule{
			&convertedGRPCRouteRule{
				rule: &gwv1.GRPCRouteRule{},
			},
		},
	}

	// Input: deliberately shuffled order to test sorting robustness
	input := []RouteDescriptor{
		httpCatchAll,
		grpcCatchAll,
		grpcGetUser,
		httpAPIv1,
		grpcOrderSvc,
		httpHealth,
	}

	result := SortAllRulesByPrecedence(input, 0)

	// Expected ordering by specificity (highest priority first):
	// 1. grpcGetUser     - exact, effective path len = 24+7+2 = 33
	// 2. grpcOrderSvc    - exact, effective path len = 24+10+2 = 36 (longer, but same type)
	//    Wait - OrderService (24) + PlaceOrder (10) = 36 vs UserService (23) + GetUser (7) = 32
	//    So OrderService is actually longer → higher priority
	// 3. httpHealth      - exact, path len = 7
	// 4. httpAPIv1       - prefix, path len = 7
	// 5. httpCatchAll    - prefix, path len = 1
	// 6. grpcCatchAll    - no match (pathType=0), path len = 0

	assert.Equal(t, 6, len(result), "should have 6 rules total")

	// Verify the ordering
	names := make([]string, len(result))
	for i, r := range result {
		names[i] = r.CommonRulePrecedence.RouteNamespacedName
	}

	// Rule 1: grpcOrderSvc (exact, longest effective path: /com.example.OrderService/PlaceOrder = 36)
	assert.Equal(t, "team-c/grpc-order-svc", names[0],
		"GRPC exact with longest path should be first")

	// Rule 2: grpcGetUser (exact, effective path: /com.example.UserService/GetUser = 32)
	assert.Equal(t, "team-c/grpc-user-svc", names[1],
		"GRPC exact with second longest path should be second")

	// Rule 3: httpHealth (exact, path len 7)
	assert.Equal(t, "team-a/http-health", names[2],
		"HTTP exact /health should come after longer GRPC exact paths")

	// Rule 4: httpAPIv1 (prefix, path len 7)
	assert.Equal(t, "team-b/http-api-v1", names[3],
		"HTTP prefix /api/v1 should come after exact matches")

	// Rule 5: httpCatchAll (prefix, path len 1)
	assert.Equal(t, "team-a/http-catchall", names[4],
		"HTTP prefix / catch-all should be near last")

	// Rule 6: grpcCatchAll (no method, pathType=0)
	assert.Equal(t, "team-d/grpc-catchall", names[5],
		"GRPC with no method match (catch-all) should be last")
}

// Test_SortAllRulesByPrecedence_MultiHostnameSplit exercises the per-hostname
// rule split: a route with N hostnames and M matches produces N×M rule units,
// one per (hostname, match), each carrying a SINGLE hostname. Hostname
// precedence is evaluated per hostname (scalar), and equally-specific hostnames
// (e.g. api.example.com vs web.example.com) are ordered deterministically by the
// hostname-string tiebreaker. The rest of the precedence file uses only
// single-hostname routes, so these cases are the multi-hostname coverage.
func Test_SortAllRulesByPrecedence_MultiHostnameSplit(t *testing.T) {
	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	prefix := (*gwv1.PathMatchType)(awssdk.String("PathPrefix"))
	exact := (*gwv1.PathMatchType)(awssdk.String("Exact"))
	rootPrefix := gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: prefix, Value: awssdk.String("/")}}
	healthExact := gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/health")}}

	mkRoute := func(hostnames []string, matches []gwv1.HTTPRouteMatch) *httpRouteDescription {
		hn := make([]gwv1.Hostname, len(hostnames))
		for i, h := range hostnames {
			hn[i] = gwv1.Hostname(h)
		}
		return &httpRouteDescription{
			route: &gwv1.HTTPRoute{
				ObjectMeta: v1.ObjectMeta{Name: "route", Namespace: "ns", CreationTimestamp: v1.Time{Time: created}},
				Spec:       gwv1.HTTPRouteSpec{Hostnames: hn},
			},
			rules: []RouteRule{&convertedHTTPRouteRule{rule: &gwv1.HTTPRouteRule{Matches: matches}}},
		}
	}

	type unit struct {
		hostname string
		matchIdx int
	}

	tests := []struct {
		name      string
		hostnames []string
		matches   []gwv1.HTTPRouteMatch
		want      []unit
	}{
		{
			name:      "2 hostnames - split into 2 units ordered by specificity (non-wildcard before wildcard)",
			hostnames: []string{"*.example.com", "api.example.com"},
			matches:   []gwv1.HTTPRouteMatch{rootPrefix},
			want: []unit{
				{"api.example.com", 0},
				{"*.example.com", 0},
			},
		},
		{
			name:      "3 hostnames x 2 matches - 6 units; hostname precedence dominates, path breaks ties within a hostname",
			hostnames: []string{"*.example.com", "example.com", "a.b.example.com"},
			matches:   []gwv1.HTTPRouteMatch{healthExact, rootPrefix}, // m0=/health (exact), m1=/ (prefix)
			want: []unit{
				{"a.b.example.com", 0}, // most-specific hostname (3 dots), exact path first
				{"a.b.example.com", 1}, // most-specific hostname, prefix path
				{"example.com", 0},     // 1 dot
				{"example.com", 1},
				{"*.example.com", 0}, // wildcard hostname last
				{"*.example.com", 1},
			},
		},
		{
			name: "7 hostnames - 7 units; mixed specificity plus equal-specificity string tiebreak (api < web)",
			hostnames: []string{
				"web.example.com", "*.example.com", "example.com", "a.b.c.example.com",
				"*.sub.example.com", "api.example.com", "a.b.example.com",
			},
			matches: []gwv1.HTTPRouteMatch{rootPrefix},
			want: []unit{
				{"a.b.c.example.com", 0}, // non-wildcard, 4 dots
				{"a.b.example.com", 0},   // non-wildcard, 3 dots
				{"api.example.com", 0},   // 2 dots, len 15 — ties web on specificity; api < web
				{"web.example.com", 0},   // 2 dots, len 15
				{"example.com", 0},       // 1 dot
				{"*.sub.example.com", 0}, // wildcard, 3 dots
				{"*.example.com", 0},     // wildcard, 2 dots
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := mkRoute(tt.hostnames, tt.matches)
			result := SortAllRulesByPrecedence([]RouteDescriptor{route}, 0)

			assert.Equalf(t, len(tt.hostnames)*len(tt.matches), len(result),
				"expected N hostnames (%d) × M matches (%d) units", len(tt.hostnames), len(tt.matches))

			for i, w := range tt.want {
				if i >= len(result) {
					break
				}
				assert.Equalf(t, w.hostname, result[i].CommonRulePrecedence.Hostname,
					"priority %d hostname", i+1)
				assert.Equalf(t, w.matchIdx, result[i].CommonRulePrecedence.MatchIndexInRule,
					"priority %d matchIndex", i+1)
			}
		})
	}
}

// Test_SortAllRulesByPrecedence_NoHostnameCatchAll verifies that a route with no
// hostnames (a catch-all, e.g. attached to a listener with no hostname) still
// produces one rule per match after the per-hostname split. The empty hostname
// list collapses to a SINGLE catch-all hostname unit, so N(=1) × M matches = M
// rules — each with an empty hostname list (no host-header condition -> matches
// all hosts), preserving the pre-split behavior. (0 hostnames × 2 matches = 2
// rules, not 1.)
func Test_SortAllRulesByPrecedence_NoHostnameCatchAll(t *testing.T) {
	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	prefix := (*gwv1.PathMatchType)(awssdk.String("PathPrefix"))
	exact := (*gwv1.PathMatchType)(awssdk.String("Exact"))

	route := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: v1.ObjectMeta{Name: "catchall", Namespace: "ns", CreationTimestamp: v1.Time{Time: created}},
			Spec:       gwv1.HTTPRouteSpec{}, // no hostnames
		},
		rules: []RouteRule{&convertedHTTPRouteRule{rule: &gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{
				{Path: &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/health")}}, // m0
				{Path: &gwv1.HTTPPathMatch{Type: prefix, Value: awssdk.String("/")}},      // m1
			},
		}}},
	}

	result := SortAllRulesByPrecedence([]RouteDescriptor{route}, 0)

	// 1 catch-all hostname unit × 2 matches = 2 rules (NOT 1).
	assert.Equal(t, 2, len(result), "no-hostname route with 2 matches must still produce 2 rules (one per match)")

	// Both units are catch-all: empty hostname list -> no host-header condition downstream.
	for i := range result {
		assert.Emptyf(t, result[i].CommonRulePrecedence.Hostname, "unit %d must carry no hostname (catch-all)", i+1)
	}

	// Hostname precedence ties (both empty), so ordering falls to path: exact
	// /health (m0) outranks prefix / (m1).
	assert.Equal(t, 0, result[0].CommonRulePrecedence.MatchIndexInRule, "exact /health match should be first")
	assert.Equal(t, 3, result[0].PrecedenceFactor.PathType, "priority-1 should be the exact match (pathType 3)")
	assert.Equal(t, 1, result[1].CommonRulePrecedence.MatchIndexInRule, "prefix / match should be second")
}

// Test_SortAllRulesByPrecedence_InheritsListenerHostname covers the case where a
// route declares NO hostnames but attaches to a listener that HAS one. Per the
// Gateway API, the route inherits the listener's hostname; the loader records
// that in CompatibleHostnamesByPort, and getCommonRouteInfo must prefer it over
// the (empty) route hostnames. The resulting split units must carry the
// listener's hostname — i.e. produce a host-scoped rule, NOT a catch-all that
// matches every host. This is the only precedence test that exercises the
// CompatibleHostnamesByPort path (the others use the route-hostname fallback).
func Test_SortAllRulesByPrecedence_InheritsListenerHostname(t *testing.T) {
	prefix := (*gwv1.PathMatchType)(awssdk.String("PathPrefix"))
	const port int32 = 80

	route := &MockRoute{
		Kind:         HTTPRouteKind,
		Name:         "no-hostname-route",
		Namespace:    "ns",
		Hostnames:    nil, // route declares no hostnames
		CreationTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		// Loader-computed intersection: the route inherits the listener's hostname.
		CompatibleHostnamesByPort: map[int32][]gwv1.Hostname{port: {"listener.example.com"}},
		Rules: []RouteRule{&MockRule{RawRule: &gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{
				{Path: &gwv1.HTTPPathMatch{Type: prefix, Value: awssdk.String("/")}},    // m0
				{Path: &gwv1.HTTPPathMatch{Type: prefix, Value: awssdk.String("/api")}}, // m1
			},
		}}},
	}

	result := SortAllRulesByPrecedence([]RouteDescriptor{route}, port)

	// 1 inherited hostname × 2 matches = 2 rules.
	assert.Equal(t, 2, len(result), "inherited-hostname route with 2 matches must produce 2 rules")

	// Every unit must carry the LISTENER's hostname (not be a catch-all). An empty
	// hostname list here would mean the rule matches every host — the bug this guards.
	for i := range result {
		assert.Equalf(t, "listener.example.com", result[i].CommonRulePrecedence.Hostname,
			"unit %d must inherit the listener hostname, not become a catch-all", i+1)
	}

	// Ordering falls to path (hostname is identical): /api (len 4) beats / (len 1).
	assert.Equal(t, 4, result[0].PrecedenceFactor.PathLength, "priority-1 should be the longer /api prefix")
	assert.Equal(t, 1, result[0].CommonRulePrecedence.MatchIndexInRule, "/api is match index 1")
	assert.Equal(t, 1, result[1].PrecedenceFactor.PathLength, "priority-2 should be the / prefix")
	assert.Equal(t, 0, result[1].CommonRulePrecedence.MatchIndexInRule, "/ is match index 0")
}

// Test_SortAllRulesByPrecedence_TwoRoutesDistinctHostnames guards against a
// (mis)understanding that the split iterates the listener-wide set of hostnames.
// CompatibleHostnamesByPort is keyed per route, so each route sees ONLY its own
// hostname: two single-hostname routes R1(A) and R2(B) must produce exactly two
// units — R1×A and R2×B — NOT the cross product R1×A, R1×B, R2×A, R2×B.
func Test_SortAllRulesByPrecedence_TwoRoutesDistinctHostnames(t *testing.T) {
	prefix := (*gwv1.PathMatchType)(awssdk.String("PathPrefix"))
	const port int32 = 80

	mk := func(name, hostname string) *MockRoute {
		return &MockRoute{
			Kind:         HTTPRouteKind,
			Name:         name,
			Namespace:    "ns",
			CreationTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			// Per-route compatible hostnames, exactly as the loader stores them.
			CompatibleHostnamesByPort: map[int32][]gwv1.Hostname{port: {gwv1.Hostname(hostname)}},
			Rules: []RouteRule{&MockRule{RawRule: &gwv1.HTTPRouteRule{
				Matches: []gwv1.HTTPRouteMatch{{Path: &gwv1.HTTPPathMatch{Type: prefix, Value: awssdk.String("/")}}},
			}}},
		}
	}

	r1 := mk("r1", "a.example.com")
	r2 := mk("r2", "b.example.com")

	result := SortAllRulesByPrecedence([]RouteDescriptor{r1, r2}, port)

	// Exactly 2 units (R1×A, R2×B), NOT the 4-unit cross product.
	assert.Equal(t, 2, len(result), "each route sees only its own hostname: expect R1×A and R2×B, not the cross product")

	routeToHost := map[string]string{}
	for _, u := range result {
		routeToHost[u.CommonRulePrecedence.RouteNamespacedName] = u.CommonRulePrecedence.Hostname
	}
	assert.Equal(t, map[string]string{
		"ns/r1": "a.example.com",
		"ns/r2": "b.example.com",
	}, routeToHost, "R1 must map only to A and R2 only to B (no cross-contamination)")
}
