package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func ptrPathType(t gwv1.PathMatchType) *gwv1.PathMatchType       { return &t }
func ptrHeaderType(t gwv1.HeaderMatchType) *gwv1.HeaderMatchType { return &t }
func ptrString(s string) *string                                 { return &s }

func TestTranslateConditions(t *testing.T) {
	exactPath := func(p string) []gwv1.HTTPRouteMatch {
		return []gwv1.HTTPRouteMatch{{
			Path: &gwv1.HTTPPathMatch{Type: ptrPathType(gwv1.PathMatchExact), Value: ptrString(p)},
		}}
	}
	emptyMatch := []gwv1.HTTPRouteMatch{{}}
	paramA := "paramA"
	paramB := "paramB"
	getMethod := gwv1.HTTPMethod("GET")
	headMethod := gwv1.HTTPMethod("HEAD")

	tests := []struct {
		name           string
		conditions     []ingress.RuleCondition
		matches        []gwv1.HTTPRouteMatch
		wantNil        bool
		wantMatchCount int
		wantLRCCount   int
		check          func(t *testing.T, result *conditionResult)
	}{
		{
			name:       "nil conditions returns nil",
			conditions: nil,
			matches:    emptyMatch,
			wantNil:    true,
		},
		{
			name:       "empty conditions returns nil",
			conditions: []ingress.RuleCondition{},
			matches:    emptyMatch,
			wantNil:    true,
		},
		{
			name: "host-header values go to LRC",
			conditions: []ingress.RuleCondition{{
				Field:            ingress.RuleConditionFieldHostHeader,
				HostHeaderConfig: &ingress.HostHeaderConditionConfig{Values: []string{"anno.example.com", "*.example.com"}},
			}},
			matches:        emptyMatch,
			wantMatchCount: 1,
			wantLRCCount:   1,
			check: func(t *testing.T, result *conditionResult) {
				assert.Equal(t, gatewayv1beta1.ListenerRuleConditionFieldHostHeader, result.ListenerRuleConditions[0].Field)
				assert.Equal(t, []string{"anno.example.com", "*.example.com"}, result.ListenerRuleConditions[0].HostHeaderConfig.Values)
			},
		},
		{
			name: "host-header complex wildcard values go to LRC",
			conditions: []ingress.RuleCondition{{
				Field:            ingress.RuleConditionFieldHostHeader,
				HostHeaderConfig: &ingress.HostHeaderConditionConfig{Values: []string{"www.*.example.com", "host?name.com"}},
			}},
			matches:        emptyMatch,
			wantMatchCount: 1,
			wantLRCCount:   1,
			check: func(t *testing.T, result *conditionResult) {
				assert.Equal(t, gatewayv1beta1.ListenerRuleConditionFieldHostHeader, result.ListenerRuleConditions[0].Field)
				assert.Equal(t, []string{"www.*.example.com", "host?name.com"}, result.ListenerRuleConditions[0].HostHeaderConfig.Values)
			},
		},
		{
			name: "host-header regexValues goes to LRC",
			conditions: []ingress.RuleCondition{{
				Field:            ingress.RuleConditionFieldHostHeader,
				HostHeaderConfig: &ingress.HostHeaderConditionConfig{RegexValues: []string{"^(.+)\\.example\\.com$"}},
			}},
			matches:        emptyMatch,
			wantMatchCount: 1,
			wantLRCCount:   1,
			check: func(t *testing.T, result *conditionResult) {
				assert.Equal(t, []string{"^(.+)\\.example\\.com$"}, result.ListenerRuleConditions[0].HostHeaderConfig.RegexValues)
			},
		},
		{
			name: "path-pattern values add additional matches (OR with base path)",
			conditions: []ingress.RuleCondition{{
				Field:             ingress.RuleConditionFieldPathPattern,
				PathPatternConfig: &ingress.PathPatternConditionConfig{Values: []string{"/anno/path2"}},
			}},
			matches:        exactPath("/path2"),
			wantMatchCount: 2,
			check: func(t *testing.T, result *conditionResult) {
				// Original path preserved
				assert.Equal(t, "/path2", *result.Matches[0].Path.Value)
				// Condition path added
				assert.Equal(t, ptrPathType(gwv1.PathMatchExact), result.Matches[1].Path.Type)
				assert.Equal(t, "/anno/path2", *result.Matches[1].Path.Value)
			},
		},
		{
			name: "path-pattern regexValues add RegularExpression match",
			conditions: []ingress.RuleCondition{{
				Field:             ingress.RuleConditionFieldPathPattern,
				PathPatternConfig: &ingress.PathPatternConditionConfig{RegexValues: []string{"^/api/?(.*)$"}},
			}},
			matches:        emptyMatch,
			wantMatchCount: 2,
			check: func(t *testing.T, result *conditionResult) {
				// Original empty match preserved
				// Condition regex path added
				assert.Equal(t, ptrPathType(gwv1.PathMatchRegularExpression), result.Matches[1].Path.Type)
				assert.Equal(t, "^/api/?(.*)$", *result.Matches[1].Path.Value)
			},
		},
		{
			name: "http-header values joined with comma in single match",
			conditions: []ingress.RuleCondition{{
				Field:            ingress.RuleConditionFieldHTTPHeader,
				HTTPHeaderConfig: &ingress.HTTPHeaderConditionConfig{HTTPHeaderName: "HeaderName", Values: []string{"Val1", "Val2"}},
			}},
			matches:        exactPath("/path3"),
			wantMatchCount: 1,
			check: func(t *testing.T, result *conditionResult) {
				m := result.Matches[0]
				assert.Equal(t, "/path3", *m.Path.Value)
				require.Len(t, m.Headers, 1)
				assert.Equal(t, gwv1.HTTPHeaderName("HeaderName"), m.Headers[0].Name)
				assert.Equal(t, ptrHeaderType(gwv1.HeaderMatchExact), m.Headers[0].Type)
				assert.Equal(t, "Val1,Val2", m.Headers[0].Value)
			},
		},
		{
			name: "http-header regexValues uses RegularExpression type",
			conditions: []ingress.RuleCondition{{
				Field:            ingress.RuleConditionFieldHTTPHeader,
				HTTPHeaderConfig: &ingress.HTTPHeaderConditionConfig{HTTPHeaderName: "User-Agent", RegexValues: []string{".+Chrome.+"}},
			}},
			matches:        emptyMatch,
			wantMatchCount: 1,
			check: func(t *testing.T, result *conditionResult) {
				assert.Equal(t, ptrHeaderType(gwv1.HeaderMatchRegularExpression), result.Matches[0].Headers[0].Type)
				assert.Equal(t, ".+Chrome.+", result.Matches[0].Headers[0].Value)
			},
		},
		{
			name: "http-request-method expands matches (OR)",
			conditions: []ingress.RuleCondition{{
				Field:                   ingress.RuleConditionFieldHTTPRequestMethod,
				HTTPRequestMethodConfig: &ingress.HTTPRequestMethodConditionConfig{Values: []string{"GET", "HEAD"}},
			}},
			matches:        exactPath("/path4"),
			wantMatchCount: 2,
			check: func(t *testing.T, result *conditionResult) {
				assert.Equal(t, &getMethod, result.Matches[0].Method)
				assert.Equal(t, &headMethod, result.Matches[1].Method)
				assert.Equal(t, "/path4", *result.Matches[0].Path.Value)
				assert.Equal(t, "/path4", *result.Matches[1].Path.Value)
			},
		},
		{
			name: "query-string values expand matches (OR within single condition)",
			conditions: []ingress.RuleCondition{{
				Field: ingress.RuleConditionFieldQueryString,
				QueryStringConfig: &ingress.QueryStringConditionConfig{
					Values: []ingress.QueryStringKeyValuePair{
						{Key: &paramA, Value: "valueA1"},
						{Key: &paramA, Value: "valueA2"},
					},
				},
			}},
			matches:        emptyMatch,
			wantMatchCount: 2,
			check: func(t *testing.T, result *conditionResult) {
				assert.Equal(t, "valueA1", result.Matches[0].QueryParams[0].Value)
				assert.Equal(t, "valueA2", result.Matches[1].QueryParams[0].Value)
			},
		},
		{
			name: "source-ip goes to LRC",
			conditions: []ingress.RuleCondition{{
				Field:          ingress.RuleConditionFieldSourceIP,
				SourceIPConfig: &ingress.SourceIPConditionConfig{Values: []string{"192.168.0.0/16", "172.16.0.0/16"}},
			}},
			matches:        emptyMatch,
			wantMatchCount: 1,
			wantLRCCount:   1,
			check: func(t *testing.T, result *conditionResult) {
				assert.Equal(t, gatewayv1beta1.ListenerRuleConditionFieldSourceIP, result.ListenerRuleConditions[0].Field)
				assert.Equal(t, []string{"192.168.0.0/16", "172.16.0.0/16"}, result.ListenerRuleConditions[0].SourceIPConfig.Values)
			},
		},
		{
			name: "multiple conditions — http-header AND query-string AND query-string (rule-path7)",
			conditions: []ingress.RuleCondition{
				{
					Field:            ingress.RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &ingress.HTTPHeaderConditionConfig{HTTPHeaderName: "HeaderName", Values: []string{"HeaderValue"}},
				},
				{
					Field:             ingress.RuleConditionFieldQueryString,
					QueryStringConfig: &ingress.QueryStringConditionConfig{Values: []ingress.QueryStringKeyValuePair{{Key: &paramA, Value: "valueA"}}},
				},
				{
					Field:             ingress.RuleConditionFieldQueryString,
					QueryStringConfig: &ingress.QueryStringConditionConfig{Values: []ingress.QueryStringKeyValuePair{{Key: &paramB, Value: "valueB"}}},
				},
			},
			matches:        exactPath("/path7"),
			wantMatchCount: 1,
			check: func(t *testing.T, result *conditionResult) {
				m := result.Matches[0]
				assert.Equal(t, "/path7", *m.Path.Value)
				require.Len(t, m.Headers, 1)
				assert.Equal(t, "HeaderValue", m.Headers[0].Value)
				require.Len(t, m.QueryParams, 2)
				assert.Equal(t, "valueA", m.QueryParams[0].Value)
				assert.Equal(t, "valueB", m.QueryParams[1].Value)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := translateConditions(tt.conditions, tt.matches)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			if tt.wantMatchCount > 0 {
				assert.Len(t, result.Matches, tt.wantMatchCount)
			}
			assert.Len(t, result.ListenerRuleConditions, tt.wantLRCCount)
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}
