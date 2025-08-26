package routeutils

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"math"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
	"time"
)

var (
	defaultHostname = []string{"example.com"}
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
	testCases := []struct {
		name   string
		input  []RouteDescriptor
		output []RulePrecedence
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
						Hostnames:            make([]string, 0),
						RouteDescriptor:      httpOneRuleNoMatch,
						Rule:                 httpOneRuleNoMatch.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     math.MaxInt,
						RouteCreateTimestamp: httpOneRuleNoMatch.GetRouteCreateTimestamp(),
					},
					HttpSpecificRulePrecedenceFactor: &HttpSpecificRulePrecedenceFactor{},
					HTTPMatch:                        &gwv1.HTTPRouteMatch{},
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
						Hostnames:            make([]string, 0),
						RouteDescriptor:      httpOneRuleOneMatch,
						Rule:                 httpOneRuleOneMatch.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpOneRuleOneMatch.GetRouteCreateTimestamp(),
					},
					HttpSpecificRulePrecedenceFactor: &HttpSpecificRulePrecedenceFactor{
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
						Hostnames:            make([]string, 0),
						RouteDescriptor:      httpOneRuleMultipleMatches,
						Rule:                 httpOneRuleMultipleMatches.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: httpOneRuleMultipleMatches.GetRouteCreateTimestamp(),
					},
					HttpSpecificRulePrecedenceFactor: &HttpSpecificRulePrecedenceFactor{
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
						Hostnames:            make([]string, 0),
						RouteDescriptor:      httpOneRuleMultipleMatches,
						Rule:                 httpOneRuleMultipleMatches.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     1,
						RouteCreateTimestamp: httpOneRuleMultipleMatches.GetRouteCreateTimestamp(),
					},
					HttpSpecificRulePrecedenceFactor: &HttpSpecificRulePrecedenceFactor{
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
						Hostnames:            make([]string, 0),
						RouteDescriptor:      grpcOneRuleNoMatch,
						Rule:                 grpcOneRuleNoMatch.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     math.MaxInt,
						RouteCreateTimestamp: grpcOneRuleNoMatch.GetRouteCreateTimestamp(),
					},
					GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{},
					GRPCMatch:                        &gwv1.GRPCRouteMatch{},
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
						Hostnames:            make([]string, 0),
						RouteDescriptor:      grpcOneRuleOneMatch,
						Rule:                 grpcOneRuleOneMatch.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: grpcOneRuleOneMatch.GetRouteCreateTimestamp(),
					},
					GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
						PathType:      3,
						ServiceLength: 16,
						MethodLength:  4,
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
						Hostnames:            make([]string, 0),
						RouteDescriptor:      grpcOneRuleMultipleMatches,
						Rule:                 grpcOneRuleMultipleMatches.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     1,
						RouteCreateTimestamp: grpcOneRuleMultipleMatches.GetRouteCreateTimestamp(),
					},
					GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
						PathType:      3,
						ServiceLength: 17,
						MethodLength:  11,
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
						Hostnames:            make([]string, 0),
						RouteDescriptor:      grpcOneRuleMultipleMatches,
						Rule:                 grpcOneRuleMultipleMatches.rules[0],
						RuleIndexInRoute:     0,
						MatchIndexInRule:     0,
						RouteCreateTimestamp: grpcOneRuleMultipleMatches.GetRouteCreateTimestamp(),
					},
					GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
						PathType:      3,
						ServiceLength: 16,
						MethodLength:  4,
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			result := SortAllRulesByPrecedence(tc.input)
			assert.Equal(t, tc.output, result)
		})
	}
}

func Test_compareHttpRulePrecedence(t *testing.T) {
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
					Hostnames: []string{"api.example.com"},
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: []string{"*.example.com"},
				},
			},
			want:   true,
			reason: "exact hostname has higher precedence than wildcard",
		},
		{
			name: "path type - exact vs prefix",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: defaultHostname,
				},
				HttpSpecificRulePrecedenceFactor: &HttpSpecificRulePrecedenceFactor{
					PathType: 3,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: defaultHostname,
				},
				HttpSpecificRulePrecedenceFactor: &HttpSpecificRulePrecedenceFactor{
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
					Hostnames: defaultHostname,
				},
				HttpSpecificRulePrecedenceFactor: &HttpSpecificRulePrecedenceFactor{
					PathType:   1,
					PathLength: 10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: defaultHostname,
				},
				HttpSpecificRulePrecedenceFactor: &HttpSpecificRulePrecedenceFactor{
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
					Hostnames: defaultHostname,
				},
				HttpSpecificRulePrecedenceFactor: &HttpSpecificRulePrecedenceFactor{
					PathType:   1,
					PathLength: 5,
					HasMethod:  true,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: defaultHostname,
				},
				HttpSpecificRulePrecedenceFactor: &HttpSpecificRulePrecedenceFactor{
					PathType:   1,
					PathLength: 5,
					HasMethod:  false,
				},
			},
			want:   true,
			reason: "rule with method has higher precedence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareHttpRulePrecedence(tt.ruleOne, tt.ruleTwo)
			assert.Equal(t, tt.want, got, tt.reason)
		})
	}
}

func Test_compareGrpcRulePrecedence(t *testing.T) {
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
					Hostnames: []string{"api.example.com"},
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: []string{"*.example.com"},
				},
			},
			want:   true,
			reason: "exact hostname has higher precedence than wildcard",
		},
		{
			name: "grpc route service precedence",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: defaultHostname,
				},
				GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
					PathType:      1,
					ServiceLength: 10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: defaultHostname,
				},
				GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
					PathType:      1,
					ServiceLength: 5,
				},
			},
			want:   true,
			reason: "rule with longer service length has higher precedence",
		},
		{
			name: "grpc header count precedence",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: defaultHostname,
				},
				GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
					PathType:    1,
					HeaderCount: 10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: defaultHostname,
				},
				GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
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
					Hostnames: defaultHostname,
				},
				GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
					PathType:     1,
					MethodLength: 10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: defaultHostname,
				},
				GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
					PathType:     1,
					MethodLength: 5,
				},
			},
			want:   true,
			reason: "rules with longer method length has higher precedence",
		},
		{
			name: "grpc service precedence over method",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: defaultHostname,
				},
				GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
					PathType:      1,
					ServiceLength: 5,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: defaultHostname,
				},
				GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
					PathType:     1,
					MethodLength: 10,
				},
			},
			want:   true,
			reason: "rules with service has higher precedence than method",
		},
		{
			name: "creation timestamp precedence",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames:            defaultHostname,
					RouteCreateTimestamp: earlier,
				},
				GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
					PathType:      1,
					ServiceLength: 10,
					MethodLength:  10,
					HeaderCount:   10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames:            defaultHostname,
					RouteCreateTimestamp: now,
				},
				GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
					PathType:      1,
					ServiceLength: 10,
					MethodLength:  10,
					HeaderCount:   10,
				},
			},
			want:   true,
			reason: "earlier creation time has higher precedence",
		},
		{
			name: "rule index precedence",
			ruleOne: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames:        defaultHostname,
					RuleIndexInRoute: 1,
				},
				GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
					PathType:      1,
					ServiceLength: 10,
					MethodLength:  10,
					HeaderCount:   10,
				},
			},
			ruleTwo: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames:        defaultHostname,
					RuleIndexInRoute: 3,
				},
				GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{
					PathType:      1,
					ServiceLength: 10,
					MethodLength:  10,
					HeaderCount:   10,
				},
			},
			want:   true,
			reason: "lower rule index has higher precedence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareGrpcRulePrecedence(tt.ruleOne, tt.ruleTwo)
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
