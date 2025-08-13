package gateway

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	appContainerPort        = 80
	udpContainerPort        = 8080
	defaultNumReplicas      = 3
	defaultName             = "gateway-e2e"
	udpDefaultName          = defaultName + "-udp"
	defaultGatewayClassName = "gwclass-e2e"
	defaultLbConfigName     = "lbconfig-e2e"
	defaultTgConfigName     = "tgconfig-e2e"
	udpDefaultTgConfigName  = defaultTgConfigName + "-udp"
	testHostname            = "*.elb.us-west-2.amazonaws.com"
	// constants used in ALB http route matches and filters tests
	headerModificationServerEnabled = "routing.http.response.server.enabled"
	headerModificationMaxAge        = "routing.http.response.access_control_max_age.header_value"
	headerModificationMaxAgeValue   = "30"
	testPathString                  = "/test-path"
	testHttpHeaderNameOne           = "X-Test-Header-One"
	testHttpHeaderValueOne          = "test-header-value-One"
	testHttpHeaderNameTwo           = "X-Test-Header-Two"
	testHttpHeaderValueTwo          = "test-header-value-Two"
	testTargetGroupArn              = "arn:randomArn"
	testQueryStringKeyOne           = "queryKeyOne"
	testQueryStringValueOne         = "queryValueOne"
	testQueryStringKeyTwo           = "queryKeyTwo"
	testQueryStringValueTwo         = "queryValueTwo"
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

var listenerConfigurationForHeaderModification = &[]elbv2gw.ListenerConfiguration{
	{
		ProtocolPort: "HTTP:80",
		ListenerAttributes: []elbv2gw.ListenerAttribute{
			{
				Key:   headerModificationServerEnabled,
				Value: "true",
			},
			{
				Key:   headerModificationMaxAge,
				Value: headerModificationMaxAgeValue,
			},
		},
	},
}

var defaultPort = gwalpha2.PortNumber(80)
var DefaultHttpRouteRuleBackendRefs = []gwv1.HTTPBackendRef{
	{
		BackendRef: gwv1.BackendRef{
			BackendObjectReference: gwv1.BackendObjectReference{
				Name: defaultName,
				Port: &defaultPort,
			},
		},
	},
}

var portNew = gwalpha2.PortNumber(8443)
var httpRouteRuleWithMatchesAndFilters = []gwv1.HTTPRouteRule{
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

var httpRouteRuleWithMatchesAndTargetGroupWeights = []gwv1.HTTPRouteRule{
	// rule 1
	{
		Matches: []gwv1.HTTPRouteMatch{
			{
				Path: &gwv1.HTTPPathMatch{
					Type:  &[]gwv1.PathMatchType{gwv1.PathMatchExact}[0],
					Value: awssdk.String(testPathString),
				},
			},
		},
		BackendRefs: []gwv1.HTTPBackendRef{
			{
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: defaultName,
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
					Value: awssdk.String(testPathString),
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Name:  testQueryStringKeyOne,
						Value: testQueryStringValueOne,
					},
					{
						Name:  testQueryStringKeyTwo,
						Value: testQueryStringValueTwo,
					},
				},
			},
		},
		BackendRefs: []gwv1.HTTPBackendRef{
			{
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: defaultName,
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
					Value: awssdk.String(testPathString),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Name:  testHttpHeaderNameOne,
						Value: testHttpHeaderValueOne,
					},
					{
						Name:  testHttpHeaderNameTwo,
						Value: testHttpHeaderValueTwo,
					},
				},
				Method: &[]gwv1.HTTPMethod{gwv1.HTTPMethodGet}[0],
			},
		},
		BackendRefs: []gwv1.HTTPBackendRef{
			{
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: defaultName,
						Port: &defaultPort,
					},
					Weight: awssdk.Int32(30),
				},
			},
		},
	},
}
