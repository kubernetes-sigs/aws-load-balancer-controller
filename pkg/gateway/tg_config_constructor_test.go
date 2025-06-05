package gateway

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"testing"
)

// Just basic tests here, we'll validate the logic in the method specific unit tests.
func Test_ConstructTargetGroupConfigForRoute(t *testing.T) {
	testCases := []struct {
		name           string
		tgConfig       *elbv2gw.TargetGroupConfiguration
		routeName      string
		routeNamespace string
		routeKind      string
		expected       *elbv2gw.TargetGroupProps
	}{
		{
			name:           "null config",
			routeKind:      "HTTPRoute",
			routeName:      "httproute",
			routeNamespace: "routens",
		},
		{
			name: "with just default props",
			tgConfig: &elbv2gw.TargetGroupConfiguration{
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					DefaultConfiguration: elbv2gw.TargetGroupProps{
						TargetType: (*elbv2gw.TargetType)(awssdk.String("ip")),
					},
				},
			},
			routeKind:      "HTTPRoute",
			routeName:      "httproute",
			routeNamespace: "routens",
			expected: &elbv2gw.TargetGroupProps{
				TargetType: (*elbv2gw.TargetType)(awssdk.String("ip")),
			},
		},
		{
			name:           "basic merge",
			routeKind:      "HTTPRoute",
			routeName:      "httproute",
			routeNamespace: "routens",
			tgConfig: &elbv2gw.TargetGroupConfiguration{
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					DefaultConfiguration: elbv2gw.TargetGroupProps{
						TargetType: (*elbv2gw.TargetType)(awssdk.String("ip")),
					},
					RouteConfigurations: []elbv2gw.RouteConfiguration{
						{
							RouteIdentifier: elbv2gw.RouteIdentifier{
								RouteKind: "HTTPRoute",
							},
							TargetGroupProps: elbv2gw.TargetGroupProps{
								TargetGroupName: awssdk.String("my-tg-name"),
							},
						},
					},
				},
			},
			expected: &elbv2gw.TargetGroupProps{
				TargetType:            (*elbv2gw.TargetType)(awssdk.String("ip")),
				TargetGroupName:       awssdk.String("my-tg-name"),
				Tags:                  &map[string]string{},
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			constructor := NewTargetGroupConfigConstructor()
			res := constructor.ConstructTargetGroupConfigForRoute(tc.tgConfig, tc.routeName, tc.routeNamespace, tc.routeKind)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func Test_mergeWithLongestMatch(t *testing.T) {
	testCases := []struct {
		name           string
		defaultConfig  *elbv2gw.TargetGroupProps
		routeConfigs   []elbv2gw.RouteConfiguration
		routeName      string
		routeNamespace string
		routeKind      string
		expected       *elbv2gw.TargetGroupProps
	}{
		{
			name: "only default provided",
			defaultConfig: &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("tg name"),
				IPAddressType:   (*elbv2gw.TargetGroupIPAddressType)(awssdk.String("ipv4")),
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthyThresholdCount:   awssdk.Int32(1),
					HealthCheckInterval:     awssdk.Int32(2),
					HealthCheckPath:         awssdk.String("/foo"),
					HealthCheckPort:         awssdk.String("3"),
					HealthCheckProtocol:     (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.TargetGroupHealthCheckProtocolTCP))),
					HealthCheckTimeout:      awssdk.Int32(4),
					UnhealthyThresholdCount: awssdk.Int32(5),
					Matcher: &elbv2gw.HealthCheckMatcher{
						HTTPCode: awssdk.String("1"),
					},
				},
				NodeSelector: &metav1.LabelSelector{},
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "foo",
						Value: "bar",
					},
				},
				TargetType:         (*elbv2gw.TargetType)(awssdk.String("ip")),
				Protocol:           (*elbv2gw.Protocol)(awssdk.String(string(elbv2gw.ProtocolTCP))),
				ProtocolVersion:    (*elbv2gw.ProtocolVersion)(awssdk.String(string(elbv2gw.ProtocolVersionHTTP1))),
				EnableMultiCluster: awssdk.Bool(true),
				Tags: &map[string]string{
					"t1": "t2",
					"t3": "t4",
				},
			},
			routeKind:      "HTTPRoute",
			routeName:      "httproute",
			routeNamespace: "routens",
			expected: &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("tg name"),
				IPAddressType:   (*elbv2gw.TargetGroupIPAddressType)(awssdk.String("ipv4")),
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthyThresholdCount:   awssdk.Int32(1),
					HealthCheckInterval:     awssdk.Int32(2),
					HealthCheckPath:         awssdk.String("/foo"),
					HealthCheckPort:         awssdk.String("3"),
					HealthCheckProtocol:     (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.TargetGroupHealthCheckProtocolTCP))),
					HealthCheckTimeout:      awssdk.Int32(4),
					UnhealthyThresholdCount: awssdk.Int32(5),
					Matcher: &elbv2gw.HealthCheckMatcher{
						HTTPCode: awssdk.String("1"),
					},
				},
				NodeSelector: &metav1.LabelSelector{},
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "foo",
						Value: "bar",
					},
				},
				TargetType:         (*elbv2gw.TargetType)(awssdk.String("ip")),
				Protocol:           (*elbv2gw.Protocol)(awssdk.String(string(elbv2gw.ProtocolTCP))),
				ProtocolVersion:    (*elbv2gw.ProtocolVersion)(awssdk.String(string(elbv2gw.ProtocolVersionHTTP1))),
				EnableMultiCluster: awssdk.Bool(true),
				Tags: &map[string]string{
					"t1": "t2",
					"t3": "t4",
				},
			},
		},
		{
			name: "no route config match, use default values",
			defaultConfig: &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("tg name"),
				IPAddressType:   (*elbv2gw.TargetGroupIPAddressType)(awssdk.String("ipv4")),
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthyThresholdCount:   awssdk.Int32(1),
					HealthCheckInterval:     awssdk.Int32(2),
					HealthCheckPath:         awssdk.String("/foo"),
					HealthCheckPort:         awssdk.String("3"),
					HealthCheckProtocol:     (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.TargetGroupHealthCheckProtocolTCP))),
					HealthCheckTimeout:      awssdk.Int32(4),
					UnhealthyThresholdCount: awssdk.Int32(5),
					Matcher: &elbv2gw.HealthCheckMatcher{
						HTTPCode: awssdk.String("1"),
					},
				},
				NodeSelector: &metav1.LabelSelector{},
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "foo",
						Value: "bar",
					},
				},
				TargetType:         (*elbv2gw.TargetType)(awssdk.String("ip")),
				Protocol:           (*elbv2gw.Protocol)(awssdk.String(string(elbv2gw.ProtocolTCP))),
				ProtocolVersion:    (*elbv2gw.ProtocolVersion)(awssdk.String(string(elbv2gw.ProtocolVersionHTTP1))),
				EnableMultiCluster: awssdk.Bool(true),
				Tags: &map[string]string{
					"t1": "t2",
					"t3": "t4",
				},
			},
			routeKind:      "HTTPRoute",
			routeName:      "httproute",
			routeNamespace: "routens",
			expected: &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("tg name"),
				IPAddressType:   (*elbv2gw.TargetGroupIPAddressType)(awssdk.String("ipv4")),
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthyThresholdCount:   awssdk.Int32(1),
					HealthCheckInterval:     awssdk.Int32(2),
					HealthCheckPath:         awssdk.String("/foo"),
					HealthCheckPort:         awssdk.String("3"),
					HealthCheckProtocol:     (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.TargetGroupHealthCheckProtocolTCP))),
					HealthCheckTimeout:      awssdk.Int32(4),
					UnhealthyThresholdCount: awssdk.Int32(5),
					Matcher: &elbv2gw.HealthCheckMatcher{
						HTTPCode: awssdk.String("1"),
					},
				},
				NodeSelector: &metav1.LabelSelector{},
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "foo",
						Value: "bar",
					},
				},
				TargetType:         (*elbv2gw.TargetType)(awssdk.String("ip")),
				Protocol:           (*elbv2gw.Protocol)(awssdk.String(string(elbv2gw.ProtocolTCP))),
				ProtocolVersion:    (*elbv2gw.ProtocolVersion)(awssdk.String(string(elbv2gw.ProtocolVersionHTTP1))),
				EnableMultiCluster: awssdk.Bool(true),
				Tags: &map[string]string{
					"t1": "t2",
					"t3": "t4",
				},
			},
			routeConfigs: []elbv2gw.RouteConfiguration{
				{
					RouteIdentifier: elbv2gw.RouteIdentifier{
						RouteKind: "UDPRoute",
					},
					TargetGroupProps: elbv2gw.TargetGroupProps{
						TargetGroupName: awssdk.String("my-udp-tg-name"),
					},
				},
				{
					RouteIdentifier: elbv2gw.RouteIdentifier{
						RouteKind: "TLSRoute",
					},
					TargetGroupProps: elbv2gw.TargetGroupProps{
						TargetGroupName: awssdk.String("my-tls-tg-name"),
					},
				},
			},
		},
		{
			name: "exact route match, use only route specific values",
			defaultConfig: &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("tg name"),
				IPAddressType:   (*elbv2gw.TargetGroupIPAddressType)(awssdk.String("ipv4")),
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthyThresholdCount:   awssdk.Int32(1),
					HealthCheckInterval:     awssdk.Int32(2),
					HealthCheckPath:         awssdk.String("/foo"),
					HealthCheckPort:         awssdk.String("3"),
					HealthCheckProtocol:     (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.TargetGroupHealthCheckProtocolTCP))),
					HealthCheckTimeout:      awssdk.Int32(4),
					UnhealthyThresholdCount: awssdk.Int32(5),
					Matcher: &elbv2gw.HealthCheckMatcher{
						HTTPCode: awssdk.String("1"),
					},
				},
				NodeSelector: &metav1.LabelSelector{},
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "foo",
						Value: "bar",
					},
				},
				TargetType:         (*elbv2gw.TargetType)(awssdk.String("ip")),
				Protocol:           (*elbv2gw.Protocol)(awssdk.String(string(elbv2gw.ProtocolTCP))),
				ProtocolVersion:    (*elbv2gw.ProtocolVersion)(awssdk.String(string(elbv2gw.ProtocolVersionHTTP1))),
				EnableMultiCluster: awssdk.Bool(true),
				Tags: &map[string]string{
					"t1": "t2",
					"t3": "t4",
				},
			},
			routeKind:      "HTTPRoute",
			routeName:      "httproute",
			routeNamespace: "routens",
			expected: &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("tg name route"),
				IPAddressType:   (*elbv2gw.TargetGroupIPAddressType)(awssdk.String("ipv6")),
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthyThresholdCount:   awssdk.Int32(10),
					HealthCheckInterval:     awssdk.Int32(20),
					HealthCheckPath:         awssdk.String("/fooroute"),
					HealthCheckPort:         awssdk.String("30"),
					HealthCheckProtocol:     (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.TargetGroupHealthCheckProtocolHTTP))),
					HealthCheckTimeout:      awssdk.Int32(40),
					UnhealthyThresholdCount: awssdk.Int32(50),
					Matcher: &elbv2gw.HealthCheckMatcher{
						HTTPCode: awssdk.String("10"),
					},
				},
				NodeSelector: &metav1.LabelSelector{},
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "foo",
						Value: "barroute",
					},
				},
				TargetType:         (*elbv2gw.TargetType)(awssdk.String("instance")),
				Protocol:           (*elbv2gw.Protocol)(awssdk.String(string(elbv2gw.ProtocolUDP))),
				ProtocolVersion:    (*elbv2gw.ProtocolVersion)(awssdk.String(string(elbv2gw.ProtocolVersionHTTP2))),
				EnableMultiCluster: awssdk.Bool(false),
				Tags: &map[string]string{
					"t1": "t2route",
					"t3": "t4route",
				},
			},
			routeConfigs: []elbv2gw.RouteConfiguration{
				{
					RouteIdentifier: elbv2gw.RouteIdentifier{
						RouteKind: "HTTPRoute",
					},
					TargetGroupProps: elbv2gw.TargetGroupProps{
						TargetGroupName: awssdk.String("other http route that shouldnt considered"),
					},
				},
				{
					RouteIdentifier: elbv2gw.RouteIdentifier{
						RouteKind:      "HTTPRoute",
						RouteName:      "httproute",
						RouteNamespace: "routens",
					},
					TargetGroupProps: elbv2gw.TargetGroupProps{
						TargetGroupName: awssdk.String("tg name route"),
						IPAddressType:   (*elbv2gw.TargetGroupIPAddressType)(awssdk.String("ipv6")),
						HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
							HealthyThresholdCount:   awssdk.Int32(10),
							HealthCheckInterval:     awssdk.Int32(20),
							HealthCheckPath:         awssdk.String("/fooroute"),
							HealthCheckPort:         awssdk.String("30"),
							HealthCheckProtocol:     (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.TargetGroupHealthCheckProtocolHTTP))),
							HealthCheckTimeout:      awssdk.Int32(40),
							UnhealthyThresholdCount: awssdk.Int32(50),
							Matcher: &elbv2gw.HealthCheckMatcher{
								HTTPCode: awssdk.String("10"),
							},
						},
						NodeSelector: &metav1.LabelSelector{},
						TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
							{
								Key:   "foo",
								Value: "barroute",
							},
						},
						TargetType:         (*elbv2gw.TargetType)(awssdk.String("instance")),
						Protocol:           (*elbv2gw.Protocol)(awssdk.String(string(elbv2gw.ProtocolUDP))),
						ProtocolVersion:    (*elbv2gw.ProtocolVersion)(awssdk.String(string(elbv2gw.ProtocolVersionHTTP2))),
						EnableMultiCluster: awssdk.Bool(false),
						Tags: &map[string]string{
							"t1": "t2route",
							"t3": "t4route",
						},
					},
				},
				{
					RouteIdentifier: elbv2gw.RouteIdentifier{
						RouteKind: "TLSRoute",
					},
					TargetGroupProps: elbv2gw.TargetGroupProps{
						TargetGroupName: awssdk.String("my-tls-tg-name"),
					},
				},
			},
		},
		{
			name: "kind + ns match takes precedence over just kind match",
			defaultConfig: &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("tg name"),
				IPAddressType:   (*elbv2gw.TargetGroupIPAddressType)(awssdk.String("ipv4")),
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthyThresholdCount:   awssdk.Int32(1),
					HealthCheckInterval:     awssdk.Int32(2),
					HealthCheckPath:         awssdk.String("/foo"),
					HealthCheckPort:         awssdk.String("3"),
					HealthCheckProtocol:     (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.TargetGroupHealthCheckProtocolTCP))),
					HealthCheckTimeout:      awssdk.Int32(4),
					UnhealthyThresholdCount: awssdk.Int32(5),
					Matcher: &elbv2gw.HealthCheckMatcher{
						HTTPCode: awssdk.String("1"),
					},
				},
				NodeSelector: &metav1.LabelSelector{},
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "foo",
						Value: "bar",
					},
				},
				TargetType:         (*elbv2gw.TargetType)(awssdk.String("ip")),
				Protocol:           (*elbv2gw.Protocol)(awssdk.String(string(elbv2gw.ProtocolTCP))),
				ProtocolVersion:    (*elbv2gw.ProtocolVersion)(awssdk.String(string(elbv2gw.ProtocolVersionHTTP1))),
				EnableMultiCluster: awssdk.Bool(true),
				Tags: &map[string]string{
					"t1": "t2",
					"t3": "t4",
				},
			},
			routeKind:      "HTTPRoute",
			routeName:      "httproute",
			routeNamespace: "routens",
			expected: &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("expected tg name"),
				IPAddressType:   (*elbv2gw.TargetGroupIPAddressType)(awssdk.String("ipv4")),
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthyThresholdCount:   awssdk.Int32(1),
					HealthCheckInterval:     awssdk.Int32(2),
					HealthCheckPath:         awssdk.String("/foo"),
					HealthCheckPort:         awssdk.String("3"),
					HealthCheckProtocol:     (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.TargetGroupHealthCheckProtocolTCP))),
					HealthCheckTimeout:      awssdk.Int32(4),
					UnhealthyThresholdCount: awssdk.Int32(5),
					Matcher: &elbv2gw.HealthCheckMatcher{
						HTTPCode: awssdk.String("1"),
					},
				},
				NodeSelector: &metav1.LabelSelector{},
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "foo",
						Value: "bar",
					},
				},
				TargetType:         (*elbv2gw.TargetType)(awssdk.String("ip")),
				Protocol:           (*elbv2gw.Protocol)(awssdk.String(string(elbv2gw.ProtocolTCP))),
				ProtocolVersion:    (*elbv2gw.ProtocolVersion)(awssdk.String(string(elbv2gw.ProtocolVersionHTTP1))),
				EnableMultiCluster: awssdk.Bool(true),
				Tags: &map[string]string{
					"t1": "t2",
					"t3": "t4",
				},
			},
			routeConfigs: []elbv2gw.RouteConfiguration{
				{
					RouteIdentifier: elbv2gw.RouteIdentifier{
						RouteKind:      "HTTPRoute",
						RouteNamespace: "routens",
					},
					TargetGroupProps: elbv2gw.TargetGroupProps{
						TargetGroupName: awssdk.String("expected tg name"),
					},
				},
				{
					RouteIdentifier: elbv2gw.RouteIdentifier{
						RouteKind: "HTTPRoute",
					},
					TargetGroupProps: elbv2gw.TargetGroupProps{
						TargetGroupName: awssdk.String("not expected tg name"),
					},
				},
			},
		},
		{
			name: "kind is used as last resort",
			defaultConfig: &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("tg name"),
				IPAddressType:   (*elbv2gw.TargetGroupIPAddressType)(awssdk.String("ipv4")),
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthyThresholdCount:   awssdk.Int32(1),
					HealthCheckInterval:     awssdk.Int32(2),
					HealthCheckPath:         awssdk.String("/foo"),
					HealthCheckPort:         awssdk.String("3"),
					HealthCheckProtocol:     (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.TargetGroupHealthCheckProtocolTCP))),
					HealthCheckTimeout:      awssdk.Int32(4),
					UnhealthyThresholdCount: awssdk.Int32(5),
					Matcher: &elbv2gw.HealthCheckMatcher{
						HTTPCode: awssdk.String("1"),
					},
				},
				NodeSelector: &metav1.LabelSelector{},
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "foo",
						Value: "bar",
					},
				},
				TargetType:         (*elbv2gw.TargetType)(awssdk.String("ip")),
				Protocol:           (*elbv2gw.Protocol)(awssdk.String(string(elbv2gw.ProtocolTCP))),
				ProtocolVersion:    (*elbv2gw.ProtocolVersion)(awssdk.String(string(elbv2gw.ProtocolVersionHTTP1))),
				EnableMultiCluster: awssdk.Bool(true),
				Tags: &map[string]string{
					"t1": "t2",
					"t3": "t4",
				},
			},
			routeKind:      "HTTPRoute",
			routeName:      "httproute",
			routeNamespace: "routens",
			expected: &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("expected tg name"),
				IPAddressType:   (*elbv2gw.TargetGroupIPAddressType)(awssdk.String("ipv4")),
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthyThresholdCount:   awssdk.Int32(1),
					HealthCheckInterval:     awssdk.Int32(2),
					HealthCheckPath:         awssdk.String("/foo"),
					HealthCheckPort:         awssdk.String("3"),
					HealthCheckProtocol:     (*elbv2gw.TargetGroupHealthCheckProtocol)(awssdk.String(string(elbv2gw.TargetGroupHealthCheckProtocolTCP))),
					HealthCheckTimeout:      awssdk.Int32(4),
					UnhealthyThresholdCount: awssdk.Int32(5),
					Matcher: &elbv2gw.HealthCheckMatcher{
						HTTPCode: awssdk.String("1"),
					},
				},
				NodeSelector: &metav1.LabelSelector{},
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "foo",
						Value: "bar",
					},
				},
				TargetType:         (*elbv2gw.TargetType)(awssdk.String("ip")),
				Protocol:           (*elbv2gw.Protocol)(awssdk.String(string(elbv2gw.ProtocolTCP))),
				ProtocolVersion:    (*elbv2gw.ProtocolVersion)(awssdk.String(string(elbv2gw.ProtocolVersionHTTP1))),
				EnableMultiCluster: awssdk.Bool(true),
				Tags: &map[string]string{
					"t1": "t2",
					"t3": "t4",
				},
			},
			routeConfigs: []elbv2gw.RouteConfiguration{
				{
					RouteIdentifier: elbv2gw.RouteIdentifier{
						RouteKind:      "HTTPRoute",
						RouteNamespace: "routens",
					},
					TargetGroupProps: elbv2gw.TargetGroupProps{
						TargetGroupName: awssdk.String("expected tg name"),
					},
				},
				{
					RouteIdentifier: elbv2gw.RouteIdentifier{
						RouteKind: "HTTPRoute",
					},
					TargetGroupProps: elbv2gw.TargetGroupProps{
						TargetGroupName: awssdk.String("not expected tg name"),
					},
				},
			},
		},
		{
			name: "no match because more specific route identifiers are used",
			defaultConfig: &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("tg name"),
			},
			routeKind:      "HTTPRoute",
			routeName:      "httproute",
			routeNamespace: "routens",
			routeConfigs: []elbv2gw.RouteConfiguration{
				{
					RouteIdentifier: elbv2gw.RouteIdentifier{
						RouteKind:      "HTTPRoute",
						RouteNamespace: "namespace",
						RouteName:      "name",
					},
					TargetGroupProps: elbv2gw.TargetGroupProps{
						TargetGroupName: awssdk.String("not expected tg name"),
					},
				},
				{
					RouteIdentifier: elbv2gw.RouteIdentifier{
						RouteKind:      "HTTPRoute",
						RouteNamespace: "namespace",
					},
					TargetGroupProps: elbv2gw.TargetGroupProps{
						TargetGroupName: awssdk.String("not expected tg name2"),
					},
				},
			},
			expected: &elbv2gw.TargetGroupProps{
				TargetGroupName: awssdk.String("tg name"),
			},
		},
		{
			name: "attribute / tag merging logic",
			defaultConfig: &elbv2gw.TargetGroupProps{
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "not-shared-default",
						Value: "default-not-shared-valued",
					},
					{
						Key:   "shared",
						Value: "shared-default-value",
					},
				},
				Tags: &map[string]string{
					"not-shared-default": "not-shared-default-value",
					"shared-tag":         "shared-tag-default-value",
				},
			},
			routeKind:      "HTTPRoute",
			routeName:      "httproute",
			routeNamespace: "routens",
			routeConfigs: []elbv2gw.RouteConfiguration{
				{
					RouteIdentifier: elbv2gw.RouteIdentifier{
						RouteKind: "HTTPRoute",
					},
					TargetGroupProps: elbv2gw.TargetGroupProps{
						TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
							{
								Key:   "not-shared-route",
								Value: "route-not-shared-valued",
							},
							{
								Key:   "shared",
								Value: "shared-route-value",
							},
						},
						Tags: &map[string]string{
							"not-shared-route": "not-shared-route-value",
							"shared-tag":       "shared-tag-route-value",
						},
					},
				},
			},
			expected: &elbv2gw.TargetGroupProps{
				TargetGroupAttributes: []elbv2gw.TargetGroupAttribute{
					{
						Key:   "not-shared-default",
						Value: "default-not-shared-valued",
					},
					{
						Key:   "not-shared-route",
						Value: "route-not-shared-valued",
					},
					{
						Key:   "shared",
						Value: "shared-route-value",
					},
				},
				Tags: &map[string]string{
					"not-shared-default": "not-shared-default-value",
					"not-shared-route":   "not-shared-route-value",
					"shared-tag":         "shared-tag-route-value",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			constructor := &targetGroupConfigConstructorImpl{}
			res := constructor.mergeWithLongestMatch(tc.defaultConfig, tc.routeConfigs, tc.routeName, tc.routeNamespace, tc.routeKind)
			assert.Equal(t, tc.expected, res)
		})
	}
}
