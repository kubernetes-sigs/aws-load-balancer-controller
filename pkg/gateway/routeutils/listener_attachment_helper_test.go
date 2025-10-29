package routeutils

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type mockNamespaceSelector struct {
	nss sets.Set[string]
	err error
}

func (mnss *mockNamespaceSelector) getNamespacesFromSelector(_ context.Context, _ *metav1.LabelSelector) (sets.Set[string], error) {
	return mnss.nss, mnss.err
}

func Test_listenerAllowsAttachment(t *testing.T) {

	type expectedRouteStatus struct {
		reason  string
		message string
	}

	testCases := []struct {
		name                 string
		gwNamespace          string
		routeNamespace       string
		listenerProtocol     gwv1.ProtocolType
		expectedStatusUpdate *expectedRouteStatus
		expected             bool
	}{
		{
			name:             "namespace and kind are ok",
			gwNamespace:      "ns1",
			routeNamespace:   "ns1",
			listenerProtocol: gwv1.HTTPProtocolType,
			expected:         true,
		},
		{
			name:             "namespace is not ok",
			gwNamespace:      "ns1",
			routeNamespace:   "ns2",
			listenerProtocol: gwv1.HTTPProtocolType,
			expectedStatusUpdate: &expectedRouteStatus{
				reason:  string(gwv1.RouteReasonNotAllowedByListeners),
				message: RouteStatusInfoRejectedMessageNamespaceNotMatch,
			},
		},
		{
			name:             "kind is not ok",
			gwNamespace:      "ns1",
			routeNamespace:   "ns1",
			listenerProtocol: gwv1.TLSProtocolType,
			expectedStatusUpdate: &expectedRouteStatus{
				reason:  string(gwv1.RouteReasonNotAllowedByListeners),
				message: RouteStatusInfoRejectedMessageKindNotMatch,
			},
		},
	}

	// Just using default ns behavior (route ns has to equal gw ns)
	// Using an HTTP route always
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gw := gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw1",
					Namespace: tc.gwNamespace,
				},
			}

			route := &httpRouteDescription{route: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route1",
					Namespace: tc.routeNamespace,
				},
			}}
			attachmentHelper := listenerAttachmentHelperImpl{
				logger: logr.Discard(),
			}
			hostnameFromHttpRoute := map[types.NamespacedName][]gwv1.Hostname{}
			hostnameFromGrpcRoute := map[types.NamespacedName][]gwv1.Hostname{}
			_, result, statusUpdate, err := attachmentHelper.listenerAllowsAttachment(context.Background(), gw, gwv1.Listener{
				Protocol: tc.listenerProtocol,
			}, route, hostnameFromHttpRoute, hostnameFromGrpcRoute)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
			if tc.expectedStatusUpdate == nil {
				assert.Nil(t, statusUpdate)
			} else {
				assert.NotNil(t, statusUpdate)
				assert.Equal(t, gw.Name, statusUpdate.ParentRefGateway.Name)
				assert.Equal(t, gw.Namespace, statusUpdate.ParentRefGateway.Namespace)
				assert.Equal(t, route.GetRouteNamespacedName().Name, statusUpdate.RouteMetadata.RouteName)
				assert.Equal(t, route.GetRouteNamespacedName().Namespace, statusUpdate.RouteMetadata.RouteNamespace)
				assert.Equal(t, tc.expectedStatusUpdate.message, statusUpdate.RouteStatusInfo.Message)
				assert.Equal(t, tc.expectedStatusUpdate.reason, statusUpdate.RouteStatusInfo.Reason)
			}
		})
	}
}

func Test_namespaceCheck(t *testing.T) {

	type namespaceScenario struct {
		scenarioName   string
		gwNamespace    string
		routeNamespace string
		expected       bool
	}

	nsSame := gwv1.NamespacesFromSame
	nsAll := gwv1.NamespacesFromAll
	nsSelector := gwv1.NamespacesFromSelector
	testCases := []struct {
		namespaceSelectorResult sets.Set[string]
		namespaceSelectorError  error
		listener                gwv1.Listener
		name                    string

		scenarios []namespaceScenario
		expectErr bool
	}{
		{
			name:                   "no listener.allowedroutes defaults to same namespace",
			namespaceSelectorError: errors.New("this shouldnt get called"),
			scenarios: []namespaceScenario{
				{
					scenarioName:   "same ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns1",
					expected:       true,
				},
				{
					scenarioName:   "different ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns2",
				},
			},
		},
		{
			name:                   "no listener.allowedroutes.namespaces defaults to same namespace",
			namespaceSelectorError: errors.New("this shouldnt get called"),
			scenarios: []namespaceScenario{
				{
					scenarioName:   "same ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns1",
					expected:       true,
				},
				{
					scenarioName:   "different ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns2",
				},
			},
		},
		{
			name:                   "no listener.allowedroutes.namespaces.from defaults to same namespace",
			namespaceSelectorError: errors.New("this shouldnt get called"),
			scenarios: []namespaceScenario{
				{
					scenarioName:   "same ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns1",
					expected:       true,
				},
				{
					scenarioName:   "different ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns2",
				},
			},
		},
		{
			name:                   "listener.allowedroutes.namespaces.from set to same",
			namespaceSelectorError: errors.New("this shouldnt get called"),
			scenarios: []namespaceScenario{
				{
					scenarioName:   "same ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns1",
					expected:       true,
				},
				{
					scenarioName:   "different ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns2",
				},
			},
			listener: gwv1.Listener{
				AllowedRoutes: &gwv1.AllowedRoutes{
					Namespaces: &gwv1.RouteNamespaces{
						From: &nsSame,
					},
				},
			},
		},
		{
			name:                   "listener.allowedroutes.namespaces.from set to all",
			namespaceSelectorError: errors.New("this shouldnt get called"),
			scenarios: []namespaceScenario{
				{
					scenarioName:   "same ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns1",
					expected:       true,
				},
				{
					scenarioName:   "different ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns2",
					expected:       true,
				},
			},
			listener: gwv1.Listener{
				AllowedRoutes: &gwv1.AllowedRoutes{
					Namespaces: &gwv1.RouteNamespaces{
						From: &nsAll,
					},
				},
			},
		},
		{
			name: "listener.allowedroutes.namespaces.from set to selector with no selector specified",
			scenarios: []namespaceScenario{
				{
					scenarioName:   "same ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns1",
					expected:       false,
				},
				{
					scenarioName:   "different ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns2",
					expected:       false,
				},
			},
			listener: gwv1.Listener{
				AllowedRoutes: &gwv1.AllowedRoutes{
					Namespaces: &gwv1.RouteNamespaces{
						From: &nsSelector,
					},
				},
			},
		},
		{
			name: "listener.allowedroutes.namespaces.from set to selector",
			scenarios: []namespaceScenario{
				{
					scenarioName:   "same ns but not in selector",
					gwNamespace:    "ns1",
					routeNamespace: "ns1",
					expected:       false,
				},
				{
					scenarioName:   "different ns",
					gwNamespace:    "ns1",
					routeNamespace: "ns2",
					expected:       false,
				},
				{
					scenarioName:   "different ns but in selector results",
					gwNamespace:    "ns1",
					routeNamespace: "ns3",
					expected:       true,
				},
			},
			listener: gwv1.Listener{
				AllowedRoutes: &gwv1.AllowedRoutes{
					Namespaces: &gwv1.RouteNamespaces{
						From:     &nsSelector,
						Selector: &metav1.LabelSelector{},
					},
				},
			},
			namespaceSelectorResult: sets.New("ns3", "ns5"),
		},
	}

	for _, tc := range testCases {
		for _, scenario := range tc.scenarios {
			t.Run(tc.name+"-"+scenario.scenarioName, func(t *testing.T) {
				attachmentHelper := listenerAttachmentHelperImpl{
					namespaceSelector: &mockNamespaceSelector{
						err: tc.namespaceSelectorError,
						nss: tc.namespaceSelectorResult,
					},
					logger: logr.Discard(),
				}

				gw := gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw1",
						Namespace: scenario.gwNamespace,
					},
				}

				route := &httpRouteDescription{route: &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "route1",
						Namespace: scenario.routeNamespace,
					},
				}}

				result, err := attachmentHelper.namespaceCheck(context.Background(), gw, tc.listener, route)

				if tc.expectErr {
					assert.Error(t, err)
					return
				}
				assert.NoError(t, err)
				assert.Equal(t, scenario.expected, result)
			})
		}
	}
}

func Test_kindCheck(t *testing.T) {
	term := gwv1.TLSModeTerminate
	pt := gwv1.TLSModePassthrough
	testCases := []struct {
		name           string
		route          preLoadRouteDescriptor
		listener       gwv1.Listener
		expectedResult bool
	}{
		{
			name:  "use fallback - https protocol, http route",
			route: &httpRouteDescription{},
			listener: gwv1.Listener{
				Protocol: gwv1.HTTPSProtocolType,
			},
			expectedResult: true,
		},
		{
			name:  "use fallback - http protocol, http route",
			route: &httpRouteDescription{},
			listener: gwv1.Listener{
				Protocol: gwv1.HTTPSProtocolType,
			},
			expectedResult: true,
		},
		{
			name:  "use fallback - udp protocol, http route",
			route: &httpRouteDescription{},
			listener: gwv1.Listener{
				Protocol: gwv1.UDPProtocolType,
			},
			expectedResult: false,
		},
		{
			name:  "use allowed kinds list - no route kinds specified",
			route: &httpRouteDescription{},
			listener: gwv1.Listener{
				Protocol:      gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{}},
			},
			expectedResult: true,
		},
		{
			name:  "use allowed kinds list - override protocol specific allowed kinds",
			route: &httpRouteDescription{},
			listener: gwv1.Listener{
				Protocol: gwv1.UDPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{
					{Kind: gwv1.Kind(HTTPRouteKind)},
				}},
			},
			expectedResult: true,
		},
		{
			name:  "tls listener, tcp route, terminate by default",
			route: &tcpRouteDescription{},
			listener: gwv1.Listener{
				Protocol: gwv1.TCPProtocolType,
			},
			expectedResult: true,
		},
		{
			name:  "tls listener, tls route, terminate by default",
			route: &tlsRouteDescription{},
			listener: gwv1.Listener{
				Protocol: gwv1.TCPProtocolType,
			},
			expectedResult: false,
		},
		{
			name:  "tls listener, tcp route, terminate specified",
			route: &tcpRouteDescription{},
			listener: gwv1.Listener{
				Protocol: gwv1.TCPProtocolType,
				TLS: &gwv1.GatewayTLSConfig{
					Mode: &term,
				},
			},
			expectedResult: true,
		},
		{
			name:  "tls listener, tcp route, passthrough specified",
			route: &tcpRouteDescription{},
			listener: gwv1.Listener{
				Protocol: gwv1.TLSProtocolType,
				TLS: &gwv1.GatewayTLSConfig{
					Mode: &pt,
				},
			},
			expectedResult: false,
		},
		{
			name:  "tls listener, tls route, passthrough specified",
			route: &tlsRouteDescription{},
			listener: gwv1.Listener{
				Protocol: gwv1.TLSProtocolType,
				TLS: &gwv1.GatewayTLSConfig{
					Mode: &pt,
				},
			},
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			attachmentHelper := listenerAttachmentHelperImpl{
				logger: logr.Discard(),
			}
			assert.Equal(t, tc.expectedResult, attachmentHelper.kindCheck(tc.listener, tc.route))
		})
	}
}

func Test_hostnameCheck(t *testing.T) {
	validHostname := gwv1.Hostname("example.com")
	invalidHostname := gwv1.Hostname("invalid..hostname")
	invalidHostnameTwo := gwv1.Hostname("another..invalid")

	tests := []struct {
		name           string
		listener       gwv1.Listener
		route          preLoadRouteDescriptor
		expectedResult bool
		expectedError  bool
	}{
		{
			name: "listener has no hostname - should pass",
			listener: gwv1.Listener{
				Hostname: nil,
			},
			route: &mockRoute{
				hostnames: []gwv1.Hostname{"example.com"},
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "route has no hostnames - should pass",
			listener: gwv1.Listener{
				Hostname: &validHostname,
			},
			route: &mockRoute{
				hostnames: []gwv1.Hostname{},
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "listener hostname invalid - should fail",
			listener: gwv1.Listener{
				Hostname: &invalidHostname,
			},
			route: &mockRoute{
				hostnames: []gwv1.Hostname{"example.com"},
			},
			expectedResult: false,
			expectedError:  true,
		},
		{
			name: "compatible hostnames - should pass",
			listener: gwv1.Listener{
				Hostname: &validHostname,
			},
			route: &mockRoute{
				hostnames: []gwv1.Hostname{"example.com"},
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "incompatible hostnames - should fail",
			listener: gwv1.Listener{
				Hostname: &validHostname,
			},
			route: &mockRoute{
				hostnames: []gwv1.Hostname{"example.test.com"},
			},
			expectedResult: false,
			expectedError:  false,
		},
		{
			name: "route has invalid hostname but valid one matches - should pass",
			listener: gwv1.Listener{
				Hostname: &validHostname,
			},
			route: &mockRoute{
				hostnames: []gwv1.Hostname{invalidHostname, "example.com"},
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "route has only invalid hostnames - should fail",
			listener: gwv1.Listener{
				Hostname: &validHostname,
			},
			route: &mockRoute{
				hostnames: []gwv1.Hostname{invalidHostname, invalidHostnameTwo},
			},
			expectedResult: false,
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := &listenerAttachmentHelperImpl{
				logger: logr.Discard(),
			}

			_, result, err := helper.hostnameCheck(tt.listener, tt.route)

			assert.Equal(t, tt.expectedResult, result)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_hostnameIntersection(t *testing.T) {
	tests := []struct {
		name                        string
		listenerHostname            *gwv1.Hostname
		routeHostnames              []gwv1.Hostname
		expectedAttachment          bool
		expectedCompatibleHostnames []gwv1.Hostname
		expectEmpty                 bool
	}{
		{
			name:                        "Route has nil hostnames - inherits listener hostname",
			listenerHostname:            ptr(gwv1.Hostname("bar.com")),
			routeHostnames:              nil,
			expectedAttachment:          true,
			expectedCompatibleHostnames: []gwv1.Hostname{"bar.com"},
		},
		{
			name:                        "Route has NO hostnames - inherits listener hostname",
			listenerHostname:            ptr(gwv1.Hostname("bar.com")),
			routeHostnames:              []gwv1.Hostname{},
			expectedAttachment:          true,
			expectedCompatibleHostnames: []gwv1.Hostname{"bar.com"},
		},
		{
			name:               "Listener has NO hostname",
			listenerHostname:   nil,
			routeHostnames:     []gwv1.Hostname{"foo.com"},
			expectedAttachment: true,
			expectEmpty:        true,
		},
		{
			name:               "Both have NO hostnames",
			listenerHostname:   nil,
			routeHostnames:     []gwv1.Hostname{},
			expectedAttachment: true,
			expectEmpty:        true,
		},
		{
			name:                        "Exact match",
			listenerHostname:            ptr(gwv1.Hostname("bar.com")),
			routeHostnames:              []gwv1.Hostname{"bar.com"},
			expectedAttachment:          true,
			expectedCompatibleHostnames: []gwv1.Hostname{"bar.com"},
		},
		{
			name:                        "Listener wildcard matches route",
			listenerHostname:            ptr(gwv1.Hostname("*.bar.com")),
			routeHostnames:              []gwv1.Hostname{"foo.bar.com"},
			expectedAttachment:          true,
			expectedCompatibleHostnames: []gwv1.Hostname{"foo.bar.com"},
		},
		{
			name:                        "Route wildcard matches listener",
			listenerHostname:            ptr(gwv1.Hostname("foo.bar.com")),
			routeHostnames:              []gwv1.Hostname{"*.bar.com"},
			expectedAttachment:          true,
			expectedCompatibleHostnames: []gwv1.Hostname{"foo.bar.com"},
		},
		{
			name:                        "Both wildcards, compatible",
			listenerHostname:            ptr(gwv1.Hostname("*.bar.com")),
			routeHostnames:              []gwv1.Hostname{"*.bar.com"},
			expectedAttachment:          true,
			expectedCompatibleHostnames: []gwv1.Hostname{"*.bar.com"},
		},
		{
			name:               "No overlap - rejected",
			listenerHostname:   ptr(gwv1.Hostname("bar.com")),
			routeHostnames:     []gwv1.Hostname{"foo.com"},
			expectedAttachment: false,
			expectEmpty:        true,
		},
		{
			name:                        "Multiple route hostnames, partial match",
			listenerHostname:            ptr(gwv1.Hostname("*.bar.com")),
			routeHostnames:              []gwv1.Hostname{"foo.bar.com", "baz.bar.com", "unrelated.com"},
			expectedAttachment:          true,
			expectedCompatibleHostnames: []gwv1.Hostname{"foo.bar.com", "baz.bar.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := &listenerAttachmentHelperImpl{
				logger: logr.Discard(),
			}

			listener := gwv1.Listener{
				Hostname: tt.listenerHostname,
			}

			route := &mockRoute{
				hostnames: tt.routeHostnames,
			}

			compatibleHostnames, result, err := helper.hostnameCheck(listener, route)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedAttachment, result)

			if tt.expectEmpty {
				assert.Empty(t, compatibleHostnames)
			} else {
				assert.Equal(t, tt.expectedCompatibleHostnames, compatibleHostnames)
			}
		})
	}
}

func ptr[T any](v T) *T {
	return &v
}

func Test_crossServingHostnameUniquenessCheck(t *testing.T) {
	hostnames := []gwv1.Hostname{"example.com"}
	namespace := "test-namespace"
	httpRouteName := "http-route-name"
	grpcRouteName := "grpc-route-name"
	tests := []struct {
		name                    string
		route                   preLoadRouteDescriptor
		hostnamesFromHttpRoutes map[types.NamespacedName][]gwv1.Hostname
		hostnamesFromGrpcRoutes map[types.NamespacedName][]gwv1.Hostname
		expected                bool
	}{
		{
			name: "GRPC route only - should pass",
			route: &mockRoute{
				routeKind:      GRPCRouteKind,
				hostnames:      hostnames,
				namespacedName: types.NamespacedName{Name: grpcRouteName, Namespace: namespace},
			},
			hostnamesFromHttpRoutes: map[types.NamespacedName][]gwv1.Hostname{},
			hostnamesFromGrpcRoutes: map[types.NamespacedName][]gwv1.Hostname{},
			expected:                true,
		},
		{
			name: "HTTP route only - should pass",
			route: &mockRoute{
				routeKind:      HTTPRouteKind,
				hostnames:      hostnames,
				namespacedName: types.NamespacedName{Name: httpRouteName, Namespace: namespace},
			},
			hostnamesFromHttpRoutes: map[types.NamespacedName][]gwv1.Hostname{},
			hostnamesFromGrpcRoutes: map[types.NamespacedName][]gwv1.Hostname{},
			expected:                true,
		},
		{
			name: "GRPC route with overlapping HTTP route hostname - should fail",
			route: &mockRoute{
				routeKind:      GRPCRouteKind,
				hostnames:      hostnames,
				namespacedName: types.NamespacedName{Name: grpcRouteName, Namespace: namespace},
			},
			hostnamesFromHttpRoutes: map[types.NamespacedName][]gwv1.Hostname{
				{Name: httpRouteName, Namespace: namespace}: hostnames,
			},
			hostnamesFromGrpcRoutes: map[types.NamespacedName][]gwv1.Hostname{},
			expected:                false,
		},
		{
			name: "HTTP route with overlapping GRPC route hostname - should fail",
			route: &mockRoute{
				routeKind:      HTTPRouteKind,
				hostnames:      hostnames,
				namespacedName: types.NamespacedName{Name: httpRouteName, Namespace: namespace},
			},
			hostnamesFromHttpRoutes: map[types.NamespacedName][]gwv1.Hostname{},
			hostnamesFromGrpcRoutes: map[types.NamespacedName][]gwv1.Hostname{
				{Name: grpcRouteName, Namespace: namespace}: hostnames,
			},
			expected: false,
		},
		{
			name: "GRPC route with non-overlapping HTTP route hostname - should pass",
			route: &mockRoute{
				routeKind:      GRPCRouteKind,
				hostnames:      []gwv1.Hostname{"grpc.example.com"},
				namespacedName: types.NamespacedName{Name: grpcRouteName, Namespace: namespace},
			},
			hostnamesFromHttpRoutes: map[types.NamespacedName][]gwv1.Hostname{
				{Name: httpRouteName, Namespace: namespace}: {"http.example.com"},
			},
			hostnamesFromGrpcRoutes: map[types.NamespacedName][]gwv1.Hostname{},
			expected:                true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := &listenerAttachmentHelperImpl{
				logger: logr.Discard(),
			}

			result, _ := helper.crossServingHostnameUniquenessCheck(tt.route, tt.hostnamesFromHttpRoutes, tt.hostnamesFromGrpcRoutes)
			assert.Equal(t, tt.expected, result)
		})
	}
}
