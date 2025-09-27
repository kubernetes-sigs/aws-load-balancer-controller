package routeutils

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	headerName    = "testHeader"
	headerValue   = "testValue"
	queryName     = "testQuery"
	queryValue    = "testValue"
	hostname      = "example.com"
	service       = "testService"
	method        = "testMethod"
	testKey       = "testKey"
	testValue     = "testValue"
	testKeyTwo    = "testKeyTwo"
	testValueTwo  = "testValueTwo"
	prefixType    = gwv1.PathMatchPathPrefix
	exactType     = gwv1.PathMatchExact
	regexType     = gwv1.PathMatchRegularExpression
	grpcExactType = gwv1.GRPCMethodMatchExact
	grpcRegexType = gwv1.GRPCMethodMatchRegularExpression
	sourceIP1     = "10.0.0.0/8"
	sourceIP2     = "192.168.1.0/24"
	sourceIP3     = "172.16.0.0/12"
)

func Test_BuildHttpRuleConditions(t *testing.T) {
	pathValue := "/test"
	defaultPathValue := "/"
	methodValue := gwv1.HTTPMethodGet

	tests := []struct {
		name    string
		rule    RulePrecedence
		want    []elbv2model.RuleCondition
		wantErr bool
	}{
		{
			name: "match with all fields provided",
			rule: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: []string{hostname},
				},
				HTTPMatch: &gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  &exactType,
						Value: &pathValue,
					},
					Method: &methodValue,
					Headers: []gwv1.HTTPHeaderMatch{
						{
							Name:  gwv1.HTTPHeaderName(headerName),
							Value: headerValue,
						},
					},
					QueryParams: []gwv1.HTTPQueryParamMatch{
						{
							Name:  gwv1.HTTPHeaderName(queryName),
							Value: queryValue,
						},
					},
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHostHeader,
					HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
						Values: []string{hostname},
					},
				},
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{pathValue},
					},
				},
				{
					Field: elbv2model.RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &elbv2model.HTTPHeaderConditionConfig{
						HTTPHeaderName: headerName,
						Values:         []string{headerValue},
					},
				},
				{
					Field: elbv2model.RuleConditionFieldQueryString,
					QueryStringConfig: &elbv2model.QueryStringConditionConfig{
						Values: []elbv2model.QueryStringKeyValuePair{
							{
								Key:   &queryName,
								Value: queryValue,
							},
						},
					},
				},
				{
					Field: elbv2model.RuleConditionFieldHTTPRequestMethod,
					HTTPRequestMethodConfig: &elbv2model.HTTPRequestMethodConditionConfig{
						Values: []string{string(methodValue)},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "with default Type Prefix and default value /",
			rule: RulePrecedence{
				HTTPMatch: &gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  &prefixType,
						Value: &defaultPathValue,
					},
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{"/*"},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildHttpRuleConditions(tt.rule)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_buildHttpPathCondition(t *testing.T) {
	pathValue := "/prefix"
	pathValueWithWildcard := "/prefix*"
	regexPathValue := "/v+?/*"

	tests := []struct {
		name    string
		path    gwv1.HTTPPathMatch
		want    []elbv2model.RuleCondition
		wantErr bool
	}{
		{
			name: "prefix path type",
			path: gwv1.HTTPPathMatch{
				Type:  &prefixType,
				Value: &pathValue,
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{"/prefix", "/prefix/*"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "prefix path type with wildcard",
			path: gwv1.HTTPPathMatch{
				Type:  &prefixType,
				Value: &pathValueWithWildcard,
			},
			wantErr: true,
		},
		{
			name: "exact path type",
			path: gwv1.HTTPPathMatch{
				Type:  &exactType,
				Value: &pathValue,
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{"/prefix"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "exact path type with wildcard",
			path: gwv1.HTTPPathMatch{
				Type:  &exactType,
				Value: &pathValueWithWildcard,
			},
			wantErr: true,
		},
		{
			name: "regular expression path type",
			path: gwv1.HTTPPathMatch{
				Type:  &regexType,
				Value: &regexPathValue,
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{regexPathValue},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildHttpPathCondition(&tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "shouldn't contain wildcards")
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_buildHttpHeaderCondition(t *testing.T) {
	tests := []struct {
		name        string
		headerMatch []gwv1.HTTPHeaderMatch
		want        []elbv2model.RuleCondition
	}{
		{
			name: "single header match",
			headerMatch: []gwv1.HTTPHeaderMatch{
				{
					Name:  gwv1.HTTPHeaderName(testKey),
					Value: testValue,
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &elbv2model.HTTPHeaderConditionConfig{
						HTTPHeaderName: testKey,
						Values:         []string{testValue},
					},
				},
			},
		},
		{
			name: "multiple header match",
			headerMatch: []gwv1.HTTPHeaderMatch{
				{
					Name:  gwv1.HTTPHeaderName(testKey),
					Value: testValue,
				},
				{
					Name:  gwv1.HTTPHeaderName(testKeyTwo),
					Value: testValueTwo,
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &elbv2model.HTTPHeaderConditionConfig{
						HTTPHeaderName: testKey,
						Values:         []string{testValue},
					},
				},
				{
					Field: elbv2model.RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &elbv2model.HTTPHeaderConditionConfig{
						HTTPHeaderName: testKeyTwo,
						Values:         []string{testValueTwo},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildHttpHeaderCondition(tt.headerMatch)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildHttpQueryParamCondition(t *testing.T) {
	tests := []struct {
		name        string
		queryParams []gwv1.HTTPQueryParamMatch
		want        []elbv2model.RuleCondition
	}{
		{
			name: "single query param",
			queryParams: []gwv1.HTTPQueryParamMatch{
				{
					Name:  gwv1.HTTPHeaderName(testKey),
					Value: testValue,
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldQueryString,
					QueryStringConfig: &elbv2model.QueryStringConditionConfig{
						Values: []elbv2model.QueryStringKeyValuePair{
							{
								Key:   &testKey,
								Value: testValue,
							},
						},
					},
				},
			},
		},
		{
			name: "multiple query params",
			queryParams: []gwv1.HTTPQueryParamMatch{
				{
					Name:  gwv1.HTTPHeaderName(testKey),
					Value: testValue,
				},
				{
					Name:  gwv1.HTTPHeaderName(testKeyTwo),
					Value: testValueTwo,
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldQueryString,
					QueryStringConfig: &elbv2model.QueryStringConditionConfig{
						Values: []elbv2model.QueryStringKeyValuePair{
							{
								Key:   &testKey,
								Value: testValue,
							},
						},
					},
				},
				{
					Field: elbv2model.RuleConditionFieldQueryString,
					QueryStringConfig: &elbv2model.QueryStringConditionConfig{
						Values: []elbv2model.QueryStringKeyValuePair{
							{
								Key:   &testKeyTwo,
								Value: testValueTwo,
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildHttpQueryParamCondition(tt.queryParams)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildHttpMethodCondition(t *testing.T) {
	methodValue := "GET"

	tests := []struct {
		name   string
		method gwv1.HTTPMethod
		want   []elbv2model.RuleCondition
	}{
		{
			name:   "simple method",
			method: gwv1.HTTPMethod(methodValue),
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHTTPRequestMethod,
					HTTPRequestMethodConfig: &elbv2model.HTTPRequestMethodConditionConfig{
						Values: []string{methodValue},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildHttpMethodCondition(&tt.method)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_BuildGrpcRuleConditions(t *testing.T) {
	tests := []struct {
		name    string
		rule    RulePrecedence
		want    []elbv2model.RuleCondition
		wantErr bool
	}{
		{
			name: "input has both method and headers",
			rule: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: []string{hostname},
				},
				GRPCMatch: &gwv1.GRPCRouteMatch{
					Method: &gwv1.GRPCMethodMatch{
						Type:    &grpcExactType,
						Service: &service,
						Method:  &method,
					},
					Headers: []gwv1.GRPCHeaderMatch{
						{
							Name:  gwv1.GRPCHeaderName(headerName),
							Value: headerValue,
						},
					},
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHostHeader,
					HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
						Values: []string{hostname},
					},
				},
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{"/" + service + "/" + method},
					},
				},
				{
					Field: elbv2model.RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &elbv2model.HTTPHeaderConditionConfig{
						HTTPHeaderName: headerName,
						Values:         []string{headerValue},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "input with method is nil",
			rule: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: []string{hostname},
				},
				GRPCMatch: &gwv1.GRPCRouteMatch{},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHostHeader,
					HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
						Values: []string{hostname},
					},
				},
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{"/*"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "input with match is nil",
			rule: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Hostnames: []string{hostname},
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHostHeader,
					HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
						Values: []string{hostname},
					},
				},
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{"/*"},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildGrpcRuleConditions(tt.rule)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_buildGrpcHeaderCondition(t *testing.T) {
	tests := []struct {
		name        string
		headerMatch []gwv1.GRPCHeaderMatch
		want        []elbv2model.RuleCondition
	}{
		{
			name: "single header match",
			headerMatch: []gwv1.GRPCHeaderMatch{
				{
					Name:  gwv1.GRPCHeaderName(testKey),
					Value: testValue,
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &elbv2model.HTTPHeaderConditionConfig{
						HTTPHeaderName: testKey,
						Values:         []string{testValue},
					},
				},
			},
		},
		{
			name: "multiple header match",
			headerMatch: []gwv1.GRPCHeaderMatch{
				{
					Name:  gwv1.GRPCHeaderName(testKey),
					Value: testValue,
				},
				{
					Name:  gwv1.GRPCHeaderName(testKeyTwo),
					Value: testValueTwo,
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &elbv2model.HTTPHeaderConditionConfig{
						HTTPHeaderName: testKey,
						Values:         []string{testValue},
					},
				},
				{
					Field: elbv2model.RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &elbv2model.HTTPHeaderConditionConfig{
						HTTPHeaderName: testKeyTwo,
						Values:         []string{testValueTwo},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGrpcHeaderCondition(tt.headerMatch)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildGrpcMethodCondition(t *testing.T) {

	pathWithBoth := "/" + service + "/" + method
	pathWithService := "/" + service + "/*"
	pathWithMethod := "/*/" + method

	regexService := "testService*"
	regexMethod := "testMethod?"
	regexPathWithBoth := "/" + regexService + "/" + regexMethod
	regexPathWithService := "/" + regexService + "/*"
	regexPathWithMethod := "/*/" + regexMethod

	tests := []struct {
		name    string
		method  *gwv1.GRPCMethodMatch
		want    []elbv2model.RuleCondition
		wantErr bool
	}{
		{
			name: "exact match with both service and method",
			method: &gwv1.GRPCMethodMatch{
				Type:    &grpcExactType,
				Service: &service,
				Method:  &method,
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{pathWithBoth},
					},
				},
			},
		},
		{
			name: "exact match with only service",
			method: &gwv1.GRPCMethodMatch{
				Type:    &grpcExactType,
				Service: &service,
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{pathWithService},
					},
				},
			},
		},
		{
			name: "exact match with only method",
			method: &gwv1.GRPCMethodMatch{
				Type:   &grpcExactType,
				Method: &method,
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{pathWithMethod},
					},
				},
			},
		},
		{
			name: "regex match with both service and method",
			method: &gwv1.GRPCMethodMatch{
				Type:    &grpcRegexType,
				Service: &regexService,
				Method:  &regexMethod,
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{regexPathWithBoth},
					},
				},
			},
		},
		{
			name: "regex match with only service",
			method: &gwv1.GRPCMethodMatch{
				Type:    &grpcRegexType,
				Service: &regexService,
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{regexPathWithService},
					},
				},
			},
		},
		{
			name: "regex match with only method",
			method: &gwv1.GRPCMethodMatch{
				Type:   &grpcRegexType,
				Method: &regexMethod,
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{regexPathWithMethod},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildGrpcMethodCondition(tt.method)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestGenerateValuesFromMatchHeaderValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple comma separation",
			input:    "a,b,c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "escaped comma",
			input:    "a\\,b,c",
			expected: []string{"a,b", "c"},
		},
		{
			name:     "escaped backslash",
			input:    "a\\\\,b",
			expected: []string{"a\\", "b"},
		},
		{
			name:     "multiple escaped commas",
			input:    "a\\,b\\,c",
			expected: []string{"a,b,c"},
		},
		{
			name:     "mixed escapes",
			input:    "a\\\\,b\\,c,d",
			expected: []string{"a\\", "b,c", "d"},
		},
		{
			name:     "no commas",
			input:    "single-value",
			expected: []string{"single-value"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{""},
		},
		{
			name:     "only commas",
			input:    ",,",
			expected: []string{"", "", ""},
		},
		{
			name:     "escaped other characters",
			input:    "a\\n,b\\t",
			expected: []string{"an", "bt"},
		},
		{
			name:     "backslash at end",
			input:    "a\\",
			expected: []string{"a\\"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateValuesFromMatchHeaderValue(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func Test_BuildSourceIpInCondition(t *testing.T) {
	matchIndex0 := 0
	matchIndex1 := 1

	tests := []struct {
		name               string
		ruleWithPrecedence RulePrecedence
		conditionsList     []elbv2model.RuleCondition
		expected           []elbv2model.RuleCondition
	}{
		{
			name: "rule without listener rule config",
			ruleWithPrecedence: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: &mockRouteRule{
						listenerRuleConfig: nil,
					},
					MatchIndexInRule: matchIndex0,
				},
			},
			conditionsList: []elbv2model.RuleCondition{},
			expected:       []elbv2model.RuleCondition{},
		},
		{
			name: "source IP condition without MatchIndexes applies to all",
			ruleWithPrecedence: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: &mockRouteRule{
						listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
							Spec: elbv2gw.ListenerRuleConfigurationSpec{
								Conditions: []elbv2gw.ListenerRuleCondition{
									{
										Field: elbv2gw.ListenerRuleConditionFieldSourceIP,
										SourceIPConfig: &elbv2gw.SourceIPConditionConfig{
											Values: []string{sourceIP1},
										},
										MatchIndexes: nil,
									},
								},
							},
						},
					},
					MatchIndexInRule: matchIndex0,
				},
			},
			conditionsList: []elbv2model.RuleCondition{},
			expected: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionField(elbv2gw.ListenerRuleConditionFieldSourceIP),
					SourceIPConfig: &elbv2model.SourceIPConditionConfig{
						Values: []string{sourceIP1},
					},
				},
			},
		},
		{
			name: "source IP condition with matching MatchIndex",
			ruleWithPrecedence: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: &mockRouteRule{
						listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
							Spec: elbv2gw.ListenerRuleConfigurationSpec{
								Conditions: []elbv2gw.ListenerRuleCondition{
									{
										Field: elbv2gw.ListenerRuleConditionFieldSourceIP,
										SourceIPConfig: &elbv2gw.SourceIPConditionConfig{
											Values: []string{sourceIP1},
										},
										MatchIndexes: &[]int{matchIndex0},
									},
								},
							},
						},
					},
					MatchIndexInRule: matchIndex0,
				},
			},
			conditionsList: []elbv2model.RuleCondition{},
			expected: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionField(elbv2gw.ListenerRuleConditionFieldSourceIP),
					SourceIPConfig: &elbv2model.SourceIPConditionConfig{
						Values: []string{sourceIP1},
					},
				},
			},
		},
		{
			name: "source IP condition with non-matching MatchIndex",
			ruleWithPrecedence: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: &mockRouteRule{
						listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
							Spec: elbv2gw.ListenerRuleConfigurationSpec{
								Conditions: []elbv2gw.ListenerRuleCondition{
									{
										Field: elbv2gw.ListenerRuleConditionFieldSourceIP,
										SourceIPConfig: &elbv2gw.SourceIPConditionConfig{
											Values: []string{sourceIP1},
										},
										MatchIndexes: &[]int{matchIndex1},
									},
								},
							},
						},
					},
					MatchIndexInRule: matchIndex0,
				},
			},
			conditionsList: []elbv2model.RuleCondition{},
			expected:       []elbv2model.RuleCondition{},
		},
		{
			name: "multiple source IP conditions with different MatchIndexes",
			ruleWithPrecedence: RulePrecedence{
				CommonRulePrecedence: CommonRulePrecedence{
					Rule: &mockRouteRule{
						listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
							Spec: elbv2gw.ListenerRuleConfigurationSpec{
								Conditions: []elbv2gw.ListenerRuleCondition{
									{
										Field: elbv2gw.ListenerRuleConditionFieldSourceIP,
										SourceIPConfig: &elbv2gw.SourceIPConditionConfig{
											Values: []string{sourceIP1},
										},
										MatchIndexes: &[]int{matchIndex0},
									},
									{
										Field: elbv2gw.ListenerRuleConditionFieldSourceIP,
										SourceIPConfig: &elbv2gw.SourceIPConditionConfig{
											Values: []string{sourceIP2},
										},
										MatchIndexes: &[]int{matchIndex1},
									},
									{
										Field: elbv2gw.ListenerRuleConditionFieldSourceIP,
										SourceIPConfig: &elbv2gw.SourceIPConditionConfig{
											Values: []string{sourceIP3},
										},
										MatchIndexes: nil, // Apply to all
									},
								},
							},
						},
					},
					MatchIndexInRule: matchIndex0,
				},
			},
			conditionsList: []elbv2model.RuleCondition{},
			expected: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionField(elbv2gw.ListenerRuleConditionFieldSourceIP),
					SourceIPConfig: &elbv2model.SourceIPConditionConfig{
						Values: []string{sourceIP1},
					},
				},
				{
					Field: elbv2model.RuleConditionField(elbv2gw.ListenerRuleConditionFieldSourceIP),
					SourceIPConfig: &elbv2model.SourceIPConditionConfig{
						Values: []string{sourceIP3},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildSourceIpInCondition(tt.ruleWithPrecedence, tt.conditionsList)
			assert.Equal(t, tt.expected, result)
		})
	}
}

type mockRouteRule struct {
	listenerRuleConfig *elbv2gw.ListenerRuleConfiguration
}

func (m *mockRouteRule) GetRawRouteRule() interface{} {
	//TODO implement me
	panic("implement me")
}

func (m *mockRouteRule) GetSectionName() *gwv1.SectionName {
	//TODO implement me
	panic("implement me")
}

func (m *mockRouteRule) GetBackends() []Backend {
	//TODO implement me
	panic("implement me")
}

func (m *mockRouteRule) GetListenerRuleConfig() *elbv2gw.ListenerRuleConfiguration {
	return m.listenerRuleConfig
}
