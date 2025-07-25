package routeutils

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"testing"

	"github.com/stretchr/testify/assert"
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

func Test_buildHttpRedirectAction(t *testing.T) {

	scheme := "https"
	expectedScheme := "HTTPS"
	invalidScheme := "invalid"

	port := int32(80)
	portString := "80"
	statusCode := 301
	replaceFullPath := "/new-path"
	replacePrefixPath := "/new-prefix-path"
	replacePrefixPathAfterProcessing := "/new-prefix-path/*"
	invalidPath := "/invalid-path*"

	tests := []struct {
		name    string
		filter  *gwv1.HTTPRequestRedirectFilter
		want    []elbv2model.Action
		wantErr bool
	}{
		{
			name: "redirect with all fields provided",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme:     &scheme,
				Hostname:   (*gwv1.PreciseHostname)(&hostname),
				Port:       (*gwv1.PortNumber)(&port),
				StatusCode: &statusCode,
				Path: &gwv1.HTTPPathModifier{
					Type:            gwv1.FullPathHTTPPathModifier,
					ReplaceFullPath: &replaceFullPath,
				},
			},
			want: []elbv2model.Action{
				{
					Type: elbv2model.ActionTypeRedirect,
					RedirectConfig: &elbv2model.RedirectActionConfig{
						Host:       &hostname,
						Path:       &replaceFullPath,
						Port:       &portString,
						Protocol:   &expectedScheme,
						StatusCode: "HTTP_301",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "redirect with prefix match",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Path: &gwv1.HTTPPathModifier{
					Type:               gwv1.PrefixMatchHTTPPathModifier,
					ReplacePrefixMatch: &replacePrefixPath,
				},
			},
			want: []elbv2model.Action{
				{
					Type: elbv2model.ActionTypeRedirect,
					RedirectConfig: &elbv2model.RedirectActionConfig{
						Path: &replacePrefixPathAfterProcessing,
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "redirect with no component provided",
			filter:  &gwv1.HTTPRequestRedirectFilter{},
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid scheme provided",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: &invalidScheme,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "path with wildcards in ReplaceFullPath",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Path: &gwv1.HTTPPathModifier{
					Type:            gwv1.FullPathHTTPPathModifier,
					ReplaceFullPath: &invalidPath,
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "path with wildcards in ReplacePrefixMatch",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Path: &gwv1.HTTPPathModifier{
					Type:               gwv1.PrefixMatchHTTPPathModifier,
					ReplacePrefixMatch: &invalidPath,
				},
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildHttpRedirectAction(tt.filter)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_BuildHttpRuleActionsBasedOnFilter(t *testing.T) {
	tests := []struct {
		name        string
		filters     []gwv1.HTTPRouteFilter
		wantErr     bool
		errContains string
	}{
		{
			name: "request redirect filter",
			filters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterRequestRedirect,
					RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
						Port: (*gwv1.PortNumber)(awssdk.Int32(80)),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "unsupported filter type",
			filters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterRequestHeaderModifier,
				},
			},
			wantErr:     true,
			errContains: "Unsupported filter type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions, err := BuildHttpRuleActionsBasedOnFilter(tt.filters)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, actions)
			} else {
				assert.NoError(t, err)
			}
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
