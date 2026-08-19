package routeutils

import (
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_SortAllRulesByPrecedence_ConsolidatesMethodOnlyMatches(t *testing.T) {
	exact := (*gwv1.PathMatchType)(awssdk.String("Exact"))
	prefix := (*gwv1.PathMatchType)(awssdk.String("PathPrefix"))
	regex := (*gwv1.HeaderMatchType)(awssdk.String("RegularExpression"))
	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	methodMatch := func(pathType *gwv1.PathMatchType, path string, method gwv1.HTTPMethod) gwv1.HTTPRouteMatch {
		return gwv1.HTTPRouteMatch{
			Path:   &gwv1.HTTPPathMatch{Type: pathType, Value: awssdk.String(path)},
			Method: &method,
		}
	}

	type wantRule struct {
		matchIndex int
		methods    []string
	}

	tests := []struct {
		name               string
		hostnames          []string
		matches            []gwv1.HTTPRouteMatch
		listenerRuleConfig *elbv2gw.ListenerRuleConfiguration
		want               []wantRule
	}{
		{
			name: "six method-only-differing matches collapse into method-list rules",
			matches: []gwv1.HTTPRouteMatch{
				methodMatch(exact, "/foo", gwv1.HTTPMethodGet),
				methodMatch(exact, "/foo", gwv1.HTTPMethodHead),
				methodMatch(exact, "/foo", gwv1.HTTPMethodPost),
				methodMatch(exact, "/foo", gwv1.HTTPMethodPut),
				methodMatch(exact, "/foo", gwv1.HTTPMethodPatch),
				methodMatch(exact, "/foo", gwv1.HTTPMethodDelete),
			},
			// The exact path takes one of the five condition values a rule may
			// carry, so the six methods are chunked into 4 + 2 instead of six rules.
			want: []wantRule{
				{matchIndex: 0, methods: []string{"GET", "HEAD", "POST", "PUT"}},
				{matchIndex: 4, methods: []string{"PATCH", "DELETE"}},
			},
		},
		{
			name: "methods fitting the value budget collapse into a single rule",
			matches: []gwv1.HTTPRouteMatch{
				methodMatch(exact, "/foo", gwv1.HTTPMethodGet),
				methodMatch(exact, "/foo", gwv1.HTTPMethodHead),
				methodMatch(exact, "/foo", gwv1.HTTPMethodPost),
			},
			want: []wantRule{
				{matchIndex: 0, methods: []string{"GET", "HEAD", "POST"}},
			},
		},
		{
			name:      "hostname takes a value from the budget, shrinking the chunks",
			hostnames: []string{"example.com"},
			matches: []gwv1.HTTPRouteMatch{
				methodMatch(exact, "/foo", gwv1.HTTPMethodGet),
				methodMatch(exact, "/foo", gwv1.HTTPMethodHead),
				methodMatch(exact, "/foo", gwv1.HTTPMethodPost),
				methodMatch(exact, "/foo", gwv1.HTTPMethodPut),
			},
			want: []wantRule{
				{matchIndex: 0, methods: []string{"GET", "HEAD", "POST"}},
				{matchIndex: 3, methods: []string{"PUT"}},
			},
		},
		{
			name: "no value budget left for a method list leaves the matches untouched",
			matches: []gwv1.HTTPRouteMatch{
				// prefix path (2 values) + 2 header values leaves room for a
				// single method value, i.e. today's one rule per match.
				{
					Path:   &gwv1.HTTPPathMatch{Type: prefix, Value: awssdk.String("/api")},
					Method: (*gwv1.HTTPMethod)(awssdk.String("GET")),
					Headers: []gwv1.HTTPHeaderMatch{
						{Name: "x-one", Value: "a"},
						{Name: "x-two", Value: "b"},
					},
				},
				{
					Path:   &gwv1.HTTPPathMatch{Type: prefix, Value: awssdk.String("/api")},
					Method: (*gwv1.HTTPMethod)(awssdk.String("POST")),
					Headers: []gwv1.HTTPHeaderMatch{
						{Name: "x-one", Value: "a"},
						{Name: "x-two", Value: "b"},
					},
				},
			},
			want: []wantRule{
				{matchIndex: 0, methods: nil},
				{matchIndex: 1, methods: nil},
			},
		},
		{
			name: "matches differing in path are not merged",
			matches: []gwv1.HTTPRouteMatch{
				methodMatch(exact, "/foo", gwv1.HTTPMethodGet),
				methodMatch(exact, "/bar", gwv1.HTTPMethodPost),
			},
			want: []wantRule{
				// /bar and /foo have the same path type and length, so the match
				// index breaks the tie, exactly as before consolidation.
				{matchIndex: 0, methods: nil},
				{matchIndex: 1, methods: nil},
			},
		},
		{
			name: "matches differing in path type are not merged",
			matches: []gwv1.HTTPRouteMatch{
				methodMatch(exact, "/foo", gwv1.HTTPMethodGet),
				methodMatch(prefix, "/foo", gwv1.HTTPMethodPost),
			},
			want: []wantRule{
				{matchIndex: 0, methods: nil},
				{matchIndex: 1, methods: nil},
			},
		},
		{
			name: "matches differing in headers are not merged",
			matches: []gwv1.HTTPRouteMatch{
				{
					Path:    &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")},
					Method:  (*gwv1.HTTPMethod)(awssdk.String("GET")),
					Headers: []gwv1.HTTPHeaderMatch{{Name: "x-one", Value: "a"}},
				},
				{
					Path:    &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")},
					Method:  (*gwv1.HTTPMethod)(awssdk.String("POST")),
					Headers: []gwv1.HTTPHeaderMatch{{Name: "x-one", Value: "b"}},
				},
			},
			want: []wantRule{
				{matchIndex: 0, methods: nil},
				{matchIndex: 1, methods: nil},
			},
		},
		{
			name: "matches differing only in header match type are not merged",
			matches: []gwv1.HTTPRouteMatch{
				{
					Path:    &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")},
					Method:  (*gwv1.HTTPMethod)(awssdk.String("GET")),
					Headers: []gwv1.HTTPHeaderMatch{{Name: "x-one", Value: "a"}},
				},
				{
					Path:    &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")},
					Method:  (*gwv1.HTTPMethod)(awssdk.String("POST")),
					Headers: []gwv1.HTTPHeaderMatch{{Name: "x-one", Type: regex, Value: "a"}},
				},
			},
			want: []wantRule{
				{matchIndex: 0, methods: nil},
				{matchIndex: 1, methods: nil},
			},
		},
		{
			name: "matches differing in query params are not merged",
			matches: []gwv1.HTTPRouteMatch{
				{
					Path:        &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")},
					Method:      (*gwv1.HTTPMethod)(awssdk.String("GET")),
					QueryParams: []gwv1.HTTPQueryParamMatch{{Name: "version", Value: "1"}},
				},
				{
					Path:        &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")},
					Method:      (*gwv1.HTTPMethod)(awssdk.String("POST")),
					QueryParams: []gwv1.HTTPQueryParamMatch{{Name: "version", Value: "2"}},
				},
			},
			want: []wantRule{
				{matchIndex: 0, methods: nil},
				{matchIndex: 1, methods: nil},
			},
		},
		{
			name: "identical headers and query params still merge",
			matches: []gwv1.HTTPRouteMatch{
				{
					Path:        &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")},
					Method:      (*gwv1.HTTPMethod)(awssdk.String("GET")),
					Headers:     []gwv1.HTTPHeaderMatch{{Name: "x-one", Value: "a"}},
					QueryParams: []gwv1.HTTPQueryParamMatch{{Name: "version", Value: "1"}},
				},
				{
					Path:        &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")},
					Method:      (*gwv1.HTTPMethod)(awssdk.String("POST")),
					Headers:     []gwv1.HTTPHeaderMatch{{Name: "x-one", Value: "a"}},
					QueryParams: []gwv1.HTTPQueryParamMatch{{Name: "version", Value: "1"}},
				},
			},
			want: []wantRule{
				{matchIndex: 0, methods: []string{"GET", "POST"}},
			},
		},
		{
			name: "a match without a method is not merged with method-bearing matches",
			matches: []gwv1.HTTPRouteMatch{
				{Path: &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")}},
				methodMatch(exact, "/foo", gwv1.HTTPMethodGet),
				methodMatch(exact, "/foo", gwv1.HTTPMethodPost),
			},
			want: []wantRule{
				// method-bearing rules are more specific, so they keep sorting first
				{matchIndex: 1, methods: []string{"GET", "POST"}},
				{matchIndex: 0, methods: nil},
			},
		},
		{
			name: "duplicate methods collapse to a single condition value",
			matches: []gwv1.HTTPRouteMatch{
				methodMatch(exact, "/foo", gwv1.HTTPMethodGet),
				methodMatch(exact, "/foo", gwv1.HTTPMethodGet),
			},
			want: []wantRule{
				{matchIndex: 0, methods: []string{"GET"}},
			},
		},
		{
			name: "matches scoped to different source IP conditions are not merged",
			matches: []gwv1.HTTPRouteMatch{
				methodMatch(exact, "/foo", gwv1.HTTPMethodGet),
				methodMatch(exact, "/foo", gwv1.HTTPMethodPost),
			},
			listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
				Spec: elbv2gw.ListenerRuleConfigurationSpec{
					Conditions: []elbv2gw.ListenerRuleCondition{
						{
							Field:          elbv2gw.ListenerRuleConditionFieldSourceIP,
							SourceIPConfig: &elbv2gw.SourceIPConditionConfig{Values: []string{"10.0.0.0/8"}},
							MatchIndexes:   &[]int{0},
						},
					},
				},
			},
			want: []wantRule{
				{matchIndex: 0, methods: nil},
				{matchIndex: 1, methods: nil},
			},
		},
		{
			name: "matches sharing an unscoped source IP condition are merged",
			matches: []gwv1.HTTPRouteMatch{
				methodMatch(exact, "/foo", gwv1.HTTPMethodGet),
				methodMatch(exact, "/foo", gwv1.HTTPMethodPost),
			},
			listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
				Spec: elbv2gw.ListenerRuleConfigurationSpec{
					Conditions: []elbv2gw.ListenerRuleCondition{
						{
							Field:          elbv2gw.ListenerRuleConditionFieldSourceIP,
							SourceIPConfig: &elbv2gw.SourceIPConditionConfig{Values: []string{"10.0.0.0/8"}},
						},
					},
				},
			},
			want: []wantRule{
				{matchIndex: 0, methods: []string{"GET", "POST"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := &MockRoute{
				Kind:         HTTPRouteKind,
				Name:         "route",
				Namespace:    "ns",
				Hostnames:    tt.hostnames,
				CreationTime: created,
				Rules: []RouteRule{&MockRule{
					RawRule:            &gwv1.HTTPRouteRule{Matches: tt.matches},
					ListenerRuleConfig: tt.listenerRuleConfig,
				}},
			}

			result := SortAllRulesByPrecedence([]RouteDescriptor{route}, 80)

			assert.Equal(t, len(tt.want), len(result), "unexpected number of ALB rules")
			assert.LessOrEqual(t, len(result), len(tt.matches), "consolidation must never increase the rule count")
			for i, want := range tt.want {
				if i >= len(result) {
					break
				}
				assert.Equalf(t, want.matchIndex, result[i].CommonRulePrecedence.MatchIndexInRule,
					"rule %d must keep the lowest match index of its group", i+1)
				assert.Equalf(t, want.methods, result[i].HTTPMethods, "rule %d methods", i+1)
			}
		})
	}
}

// Test_SortAllRulesByPrecedence_ConsolidationKeepsOrderingAcrossRoutes verifies
// that a consolidated group sorts exactly where its members sorted: the merged
// entry keeps the precedence factors of its members, so more specific rules of
// other routes stay ahead of it and less specific ones stay behind it.
func Test_SortAllRulesByPrecedence_ConsolidationKeepsOrderingAcrossRoutes(t *testing.T) {
	exact := (*gwv1.PathMatchType)(awssdk.String("Exact"))
	prefix := (*gwv1.PathMatchType)(awssdk.String("PathPrefix"))
	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	mkRoute := func(name string, matches []gwv1.HTTPRouteMatch) *MockRoute {
		return &MockRoute{
			Kind:         HTTPRouteKind,
			Name:         name,
			Namespace:    "ns",
			Hostnames:    []string{"example.com"},
			CreationTime: created,
			Rules:        []RouteRule{&MockRule{RawRule: &gwv1.HTTPRouteRule{Matches: matches}}},
		}
	}

	method := func(m string) *gwv1.HTTPMethod {
		return (*gwv1.HTTPMethod)(awssdk.String(m))
	}

	// exact /longer-path outranks the consolidated exact /foo rules, which in
	// turn outrank the prefix /foo rule.
	moreSpecific := mkRoute("a-more-specific", []gwv1.HTTPRouteMatch{
		{Path: &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/longer-path")}},
	})
	lessSpecific := mkRoute("b-less-specific", []gwv1.HTTPRouteMatch{
		{Path: &gwv1.HTTPPathMatch{Type: prefix, Value: awssdk.String("/foo")}},
	})
	consolidated := mkRoute("c-consolidated", []gwv1.HTTPRouteMatch{
		{Path: &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")}, Method: method("GET")},
		{Path: &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")}, Method: method("HEAD")},
		{Path: &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")}, Method: method("POST")},
		{Path: &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")}, Method: method("PUT")},
	})

	routes := []RouteDescriptor{lessSpecific, consolidated, moreSpecific}
	result := SortAllRulesByPrecedence(routes, 80)

	// 1 + 1 + (4 methods chunked by the hostname-reduced budget into 3 + 1) = 4
	assert.Equal(t, 4, len(result))

	assert.Equal(t, "ns/a-more-specific", result[0].CommonRulePrecedence.RouteNamespacedName)
	assert.Equal(t, "ns/c-consolidated", result[1].CommonRulePrecedence.RouteNamespacedName)
	assert.Equal(t, []string{"GET", "HEAD", "POST"}, result[1].HTTPMethods)
	assert.Equal(t, "ns/c-consolidated", result[2].CommonRulePrecedence.RouteNamespacedName)
	assert.Equal(t, []string{"PUT"}, result[2].HTTPMethods)
	assert.Equal(t, "ns/b-less-specific", result[3].CommonRulePrecedence.RouteNamespacedName)
}

// Test_SortAllRulesByPrecedence_ConsolidationIsPerRuleAndHostname verifies that
// matches of different route rules, or of different hostname units of the same
// rule, never merge: each rule has its own backends and filters, and each
// hostname unit is its own ALB rule.
func Test_SortAllRulesByPrecedence_ConsolidationIsPerRuleAndHostname(t *testing.T) {
	exact := (*gwv1.PathMatchType)(awssdk.String("Exact"))
	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	method := func(m string) *gwv1.HTTPMethod {
		return (*gwv1.HTTPMethod)(awssdk.String(m))
	}
	match := func(m string) gwv1.HTTPRouteMatch {
		return gwv1.HTTPRouteMatch{
			Path:   &gwv1.HTTPPathMatch{Type: exact, Value: awssdk.String("/foo")},
			Method: method(m),
		}
	}

	route := &MockRoute{
		Kind:         HTTPRouteKind,
		Name:         "route",
		Namespace:    "ns",
		Hostnames:    []string{"example.com", "other.example.com"},
		CreationTime: created,
		Rules: []RouteRule{
			&MockRule{RawRule: &gwv1.HTTPRouteRule{Matches: []gwv1.HTTPRouteMatch{match("GET"), match("POST")}}},
			&MockRule{RawRule: &gwv1.HTTPRouteRule{Matches: []gwv1.HTTPRouteMatch{match("PUT"), match("PATCH")}}},
		},
	}

	result := SortAllRulesByPrecedence([]RouteDescriptor{route}, 80)

	// 2 rules × 2 hostnames = 4 consolidated entries (down from 8).
	assert.Equal(t, 4, len(result))
	for i := range result {
		assert.Lenf(t, result[i].HTTPMethods, 2, "entry %d must carry both methods of its own rule and hostname", i+1)
	}

	byUnit := make(map[string][]string, len(result))
	for _, entry := range result {
		key := entry.CommonRulePrecedence.Hostname
		if entry.CommonRulePrecedence.RuleIndexInRoute == 1 {
			key += "#rule1"
		}
		byUnit[key] = entry.HTTPMethods
	}
	assert.Equal(t, []string{"GET", "POST"}, byUnit["other.example.com"])
	assert.Equal(t, []string{"PUT", "PATCH"}, byUnit["other.example.com#rule1"])
	assert.Equal(t, []string{"GET", "POST"}, byUnit["example.com"])
	assert.Equal(t, []string{"PUT", "PATCH"}, byUnit["example.com#rule1"])
}

// Test_SortAllRulesByPrecedence_GRPCMatchesAreNotConsolidated documents that the
// consolidation is HTTP only: GRPC method matches become path-pattern conditions
// whose service and method lengths feed the precedence factors, so merging them
// would change ordering.
func Test_SortAllRulesByPrecedence_GRPCMatchesAreNotConsolidated(t *testing.T) {
	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	grpcExact := (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact"))

	route := &MockRoute{
		Kind:         GRPCRouteKind,
		Name:         "grpc",
		Namespace:    "ns",
		CreationTime: created,
		Rules: []RouteRule{&MockRule{RawRule: &gwv1.GRPCRouteRule{
			Matches: []gwv1.GRPCRouteMatch{
				{Method: &gwv1.GRPCMethodMatch{Type: grpcExact, Service: awssdk.String("svc"), Method: awssdk.String("Get")}},
				{Method: &gwv1.GRPCMethodMatch{Type: grpcExact, Service: awssdk.String("svc"), Method: awssdk.String("Put")}},
			},
		}}},
	}

	result := SortAllRulesByPrecedence([]RouteDescriptor{route}, 80)

	assert.Equal(t, 2, len(result))
	for i := range result {
		assert.Emptyf(t, result[i].HTTPMethods, "GRPC entry %d must not carry consolidated methods", i+1)
	}
}
