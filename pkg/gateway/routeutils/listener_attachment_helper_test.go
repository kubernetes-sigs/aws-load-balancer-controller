package routeutils

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

type mockNamespaceSelector struct {
	nss sets.Set[string]
	err error
}

func (mnss *mockNamespaceSelector) getNamespacesFromSelector(_ context.Context, _ *metav1.LabelSelector) (sets.Set[string], error) {
	return mnss.nss, mnss.err
}

func Test_listenerAllowsAttachment(t *testing.T) {
	testCases := []struct {
		name             string
		gwNamespace      string
		routeNamespace   string
		listenerProtocol gwv1.ProtocolType
		expected         bool
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
		},
		{
			name:             "kind is not ok",
			gwNamespace:      "ns1",
			routeNamespace:   "ns1",
			listenerProtocol: gwv1.TLSProtocolType,
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
			mockReconciler := NewMockRouteReconciler()
			result, err := attachmentHelper.listenerAllowsAttachment(context.Background(), gw, gwv1.Listener{
				Protocol: tc.listenerProtocol,
			}, route, mockReconciler)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
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
