package test_resources

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	AppContainerPort        = 80
	UDPContainerPort        = 8080
	GRPCContainerPort       = 50051
	DefaultNumReplicas      = 3
	DefaultName             = "gateway-e2e"
	UDPDefaultName          = DefaultName + "-udp"
	GRPCDefaultName         = DefaultName + "-grpc"
	DefaultGatewayClassName = "gwclass-e2e"
	DefaultLbConfigName     = "lbconfig-e2e"
	DefaultTgConfigName     = "tgconfig-e2e"
	DefaultLRConfigName     = "lrconfig-e2e"
	UDPDefaultTgConfigName  = DefaultTgConfigName + "-udp"
	TestHostname            = "*.elb.us-west-2.amazonaws.com"
	// constants used in ALB http route matches and filters tests
	HeaderModificationServerEnabled = "routing.http.response.server.enabled"
	HeaderModificationMaxAge        = "routing.http.response.access_control_max_age.header_value"
	HeaderModificationMaxAgeValue   = "30"
	TestPathString                  = "/test-path"
	TestHttpHeaderNameOne           = "X-Test-Header-One"
	TestHttpHeaderValueOne          = "test-header-value-One"
	TestHttpHeaderNameTwo           = "X-Test-Header-Two"
	TestHttpHeaderValueTwo          = "test-header-value-Two"
	TestTargetGroupArn              = "arn:randomArn"
	TestQueryStringKeyOne           = "queryKeyOne"
	TestQueryStringValueOne         = "queryValueOne"
	TestQueryStringKeyTwo           = "queryKeyTwo"
	TestQueryStringValueTwo         = "queryValueTwo"
)

// Common settings for ALB target group health checks
var DEFAULT_ALB_TARGET_GROUP_HC = &verifier.TargetGroupHC{
	Protocol:           "HTTP",
	Port:               "traffic-port",
	Path:               "/",
	Interval:           15,
	Timeout:            5,
	HealthyThreshold:   3,
	UnhealthyThreshold: 3,
}

// Common settings for ALB target group health checks using GRPC
var DEFAULT_ALB_TARGET_GROUP_HC_GRPC = &verifier.TargetGroupHC{
	Protocol:           "HTTP",
	Port:               "traffic-port",
	Path:               "/AWS.ALB/healthcheck",
	Interval:           15,
	Timeout:            5,
	HealthyThreshold:   3,
	UnhealthyThreshold: 3,
}

var ListenerConfigurationForHeaderModification = &[]elbv2gw.ListenerConfiguration{
	{
		ProtocolPort: "HTTP:80",
		ListenerAttributes: []elbv2gw.ListenerAttribute{
			{
				Key:   HeaderModificationServerEnabled,
				Value: "true",
			},
			{
				Key:   HeaderModificationMaxAge,
				Value: HeaderModificationMaxAgeValue,
			},
		},
	},
}

var defaultPort = gwalpha2.PortNumber(80)
var DefaultHttpRouteRuleBackendRefs = []gwv1.HTTPBackendRef{
	{
		BackendRef: gwv1.BackendRef{
			BackendObjectReference: gwv1.BackendObjectReference{
				Name: DefaultName,
				Port: &defaultPort,
			},
		},
	},
}

var DefaultGrpcPort = gwalpha2.PortNumber(50051)
var DefaultGrpcRouteRuleBackendRefs = []gwv1.GRPCBackendRef{
	{
		BackendRef: gwv1.BackendRef{
			BackendObjectReference: gwv1.BackendObjectReference{
				Name: GRPCDefaultName,
				Port: &DefaultGrpcPort,
			},
		},
	},
}

var portNew = gwalpha2.PortNumber(8443)
var HTTPRouteRuleWithMatchesAndFilters = []gwv1.HTTPRouteRule{
	{
		Matches: []gwv1.HTTPRouteMatch{
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchExact}[0],
					Value: awssdk.String("/old-path"),
				},
			},
		},
		Filters: []gwv1.HTTPRouteFilter{
			{
				Type: gwv1.HTTPRouteFilterRequestRedirect,
				RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
					Scheme:   awssdk.String("https"),
					Hostname: (*gwv1.PreciseHostname)(awssdk.String("example.com")),
					Path: &gwv1.HTTPPathModifier{
						Type:            gwv1.FullPathHTTPPathModifier,
						ReplaceFullPath: awssdk.String("/new-path"),
					},
					StatusCode: awssdk.Int(301),
					Port:       &defaultPort,
				},
			},
		},
	},
	{
		Matches: []gwv1.HTTPRouteMatch{
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchPathPrefix}[0],
					Value: awssdk.String("/api"),
				},
			},
		},
		Filters: []gwv1.HTTPRouteFilter{
			{
				Type: gwv1.HTTPRouteFilterRequestRedirect,
				RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
					Scheme:   awssdk.String("https"),
					Hostname: (*gwv1.PreciseHostname)(awssdk.String("api.example.com")),
					Path: &gwv1.HTTPPathModifier{
						Type:               gwv1.PrefixMatchHTTPPathModifier,
						ReplacePrefixMatch: awssdk.String("/v2"),
					},
					StatusCode: awssdk.Int(302),
					Port:       &defaultPort,
				},
			},
		},
	},
	{
		Matches: []gwv1.HTTPRouteMatch{
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchExact}[0],
					Value: awssdk.String("/secure"),
				},
			},
		},
		Filters: []gwv1.HTTPRouteFilter{
			{
				Type: gwv1.HTTPRouteFilterRequestRedirect,
				RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
					Scheme:     awssdk.String("https"),
					Hostname:   (*gwv1.PreciseHostname)(awssdk.String("secure.example.com")),
					Port:       &portNew,
					StatusCode: awssdk.Int(302),
				},
			},
		},
	},
}

var HttpRouteRuleWithMatchesAndTargetGroupWeights = []gwv1.HTTPRouteRule{
	// rule 1
	{
		Matches: []gwv1.HTTPRouteMatch{
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchExact}[0],
					Value: awssdk.String(TestPathString),
				},
			},
		},
		BackendRefs: []gwv1.HTTPBackendRef{
			{
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: DefaultName,
						Port: &defaultPort,
					},
					Weight: awssdk.Int32(50),
				},
			},
		},
	},
	// rule 2
	{
		Matches: []gwv1.HTTPRouteMatch{
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchPathPrefix}[0],
					Value: awssdk.String(TestPathString),
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Name:  TestQueryStringKeyOne,
						Value: TestQueryStringValueOne,
					},
					{
						Name:  TestQueryStringKeyTwo,
						Value: TestQueryStringValueTwo,
					},
				},
			},
		},
		BackendRefs: []gwv1.HTTPBackendRef{
			{
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: DefaultName,
						Port: &defaultPort,
					},
					Weight: awssdk.Int32(30),
				},
			},
		},
	},
	// rule 3
	{
		Matches: []gwv1.HTTPRouteMatch{
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchPathPrefix}[0],
					Value: awssdk.String(TestPathString),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Name:  TestHttpHeaderNameOne,
						Value: TestHttpHeaderValueOne,
					},
					{
						Name:  TestHttpHeaderNameTwo,
						Value: TestHttpHeaderValueTwo,
					},
				},
				Method: &[]gwv1.HTTPMethod{gwv1.HTTPMethodGet}[0],
			},
		},
		BackendRefs: []gwv1.HTTPBackendRef{
			{
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: DefaultName,
						Port: &defaultPort,
					},
					Weight: awssdk.Int32(30),
				},
			},
		},
	},
}

var HTTPRouteRuleWithMultiMatchesInSingleRule = []gwv1.HTTPRouteRule{
	{
		BackendRefs: DefaultHttpRouteRuleBackendRefs,
		Matches: []gwv1.HTTPRouteMatch{
			// matchIndex = 0
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchExact}[0],
					Value: awssdk.String(TestPathString),
				},
			},
			// matchIndex = 1
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchPathPrefix}[0],
					Value: awssdk.String(TestPathString),
				},
				Method: &[]gwv1.HTTPMethod{gwv1.HTTPMethodGet}[0],
			},
			// matchIndex = 2
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchPathPrefix}[0],
					Value: awssdk.String(TestPathString),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Name:  TestHttpHeaderNameOne,
						Value: TestHttpHeaderValueOne,
					},
				},
			},
		},
		Filters: []gwv1.HTTPRouteFilter{
			{
				Type: gwv1.HTTPRouteFilterExtensionRef,
				ExtensionRef: &gwv1.LocalObjectReference{
					Name:  DefaultLRConfigName,
					Kind:  constants.ListenerRuleConfiguration,
					Group: constants.ControllerCRDGroupVersion,
				},
			},
		},
	},
}

const (
	// Mock OIDC provider endpoints for testing
	TestOidcIssuer                = "https://test-oidc-provider.example.com"
	TestOidcAuthorizationEndpoint = "https://test-oidc-provider.example.com/oauth2/authorize"
	TestOidcTokenEndpoint         = "https://test-oidc-provider.example.com/oauth2/token"
	TestOidcUserInfoEndpoint      = "https://test-oidc-provider.example.com/oauth2/userinfo"
	// constants used in path matcher tests
	TestExactPath  = "/exact-match-path"
	TestPrefixPath = "/prefix-match"
	TestRegexPath  = "/regex/[a-z]+/items"
)

// HTTPRouteRulesWithPathMatchers defines HTTPRoute rules exercising Exact, Prefix, and RegularExpression path types.
var HTTPRouteRulesWithPathMatchers = []gwv1.HTTPRouteRule{
	{
		Matches: []gwv1.HTTPRouteMatch{
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchExact}[0],
					Value: awssdk.String(TestExactPath),
				},
			},
		},
		BackendRefs: DefaultHttpRouteRuleBackendRefs,
	},
	{
		Matches: []gwv1.HTTPRouteMatch{
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchPathPrefix}[0],
					Value: awssdk.String(TestPrefixPath),
				},
			},
		},
		BackendRefs: DefaultHttpRouteRuleBackendRefs,
	},
	{
		Matches: []gwv1.HTTPRouteMatch{
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchRegularExpression}[0],
					Value: awssdk.String(TestRegexPath),
				},
			},
		},
		BackendRefs: DefaultHttpRouteRuleBackendRefs,
	},
}
