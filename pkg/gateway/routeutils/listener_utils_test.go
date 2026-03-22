package routeutils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type mockListenerSetLoader struct {
	result   listenerSetLoadResult
	rejected []*gwv1.ListenerSet
	error    error
}

func (l *mockListenerSetLoader) retrieveListenersFromListenerSets(ctx context.Context, gateway gwv1.Gateway) (listenerSetLoadResult, []*gwv1.ListenerSet, error) {
	return l.result, l.rejected, l.error
}

func TestValidateListeners(t *testing.T) {
	tests := []struct {
		name            string
		listeners       []gwv1.Listener
		controllerName  string
		expectedErrors  bool
		expectedCount   int
		expectedReasons []gwv1.ListenerConditionReason
	}{
		{
			name:           "empty listeners",
			listeners:      []gwv1.Listener{},
			controllerName: gateway_constants.ALBGatewayController,
			expectedErrors: false,
			expectedCount:  0,
		},
		{
			name: "valid HTTP listener",
			listeners: []gwv1.Listener{
				{
					Name:          "http",
					Port:          80,
					Protocol:      gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  false,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonAccepted},
		},
		{
			name: "invalid port - too low",
			listeners: []gwv1.Listener{
				{
					Name:          "invalid-low",
					Port:          0,
					Protocol:      gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonPortUnavailable},
		},
		{
			name: "invalid port - too high",
			listeners: []gwv1.Listener{
				{
					Name:          "invalid-high",
					Port:          65536,
					Protocol:      gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonPortUnavailable},
		},
		{
			name: "ALB unsupported TCP protocol",
			listeners: []gwv1.Listener{
				{
					Name:          "tcp",
					Port:          80,
					Protocol:      gwv1.TCPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonUnsupportedProtocol},
		},
		{
			name: "ALB unsupported UDP protocol",
			listeners: []gwv1.Listener{
				{
					Name:          "udp",
					Port:          80,
					Protocol:      gwv1.UDPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonUnsupportedProtocol},
		},
		{
			name: "ALB unsupported TLS protocol",
			listeners: []gwv1.Listener{
				{
					Name:          "tls",
					Port:          80,
					Protocol:      gwv1.TLSProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonUnsupportedProtocol},
		},
		{
			name: "NLB unsupported HTTP protocol",
			listeners: []gwv1.Listener{
				{
					Name:          "http",
					Port:          80,
					Protocol:      gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
			},
			controllerName:  gateway_constants.NLBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonUnsupportedProtocol},
		},
		{
			name: "NLB unsupported HTTPS protocol",
			listeners: []gwv1.Listener{
				{
					Name:          "https",
					Port:          443,
					Protocol:      gwv1.HTTPSProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
			},
			controllerName:  gateway_constants.NLBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonUnsupportedProtocol},
		},
		{
			name: "protocol conflict - HTTP vs HTTPS",
			listeners: []gwv1.Listener{
				{
					Name:          "http",
					Port:          80,
					Protocol:      gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
				{
					Name:          "https",
					Port:          80,
					Protocol:      gwv1.HTTPSProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   2,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonAccepted, gwv1.ListenerReasonProtocolConflict},
		},
		{
			name: "TCP+UDP allowed on same port",
			listeners: []gwv1.Listener{
				{
					Name:          "tcp",
					Port:          80,
					Protocol:      gwv1.TCPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
				{
					Name:          "udp",
					Port:          80,
					Protocol:      gwv1.UDPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
			},
			controllerName:  gateway_constants.NLBGatewayController,
			expectedErrors:  false,
			expectedCount:   2,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonAccepted, gwv1.ListenerReasonAccepted},
		},
		{
			name: "hostname conflict",
			listeners: []gwv1.Listener{
				{
					Name:          "http1",
					Port:          80,
					Protocol:      gwv1.HTTPProtocolType,
					Hostname:      (*gwv1.Hostname)(&[]string{"example.com"}[0]),
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
				{
					Name:          "http2",
					Port:          80,
					Protocol:      gwv1.HTTPProtocolType,
					Hostname:      (*gwv1.Hostname)(&[]string{"example.com"}[0]),
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   2,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonAccepted, gwv1.ListenerReasonHostnameConflict},
		},
		{
			name: "different hostnames on same port - valid",
			listeners: []gwv1.Listener{
				{
					Name:          "http1",
					Port:          80,
					Protocol:      gwv1.HTTPProtocolType,
					Hostname:      (*gwv1.Hostname)(&[]string{"example.com"}[0]),
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
				{
					Name:          "http2",
					Port:          80,
					Protocol:      gwv1.HTTPProtocolType,
					Hostname:      (*gwv1.Hostname)(&[]string{"test.com"}[0]),
					AllowedRoutes: &gwv1.AllowedRoutes{},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  false,
			expectedCount:   2,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonAccepted, gwv1.ListenerReasonAccepted},
		},
		{
			name: "invalid route kinds",
			listeners: []gwv1.Listener{
				{
					Name:     "invalid-kinds",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{Kind: gwv1.Kind(TCPRouteKind)},
						},
					},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonInvalidRouteKinds},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateListeners(allListeners{GatewayListeners: tt.listeners}, tt.controllerName)

			assert.Equal(t, tt.expectedErrors, result.GatewayListenerValidation.HasErrors)
			assert.Equal(t, tt.expectedCount, len(result.GatewayListenerValidation.Results))

			if len(tt.expectedReasons) > 0 {
				reasons := make([]gwv1.ListenerConditionReason, 0, len(result.GatewayListenerValidation.Results))
				for _, res := range result.GatewayListenerValidation.Results {
					reasons = append(reasons, res.Reason)
				}
				assert.ElementsMatch(t, tt.expectedReasons, reasons)
			}
		})
	}
}

func TestGetSupportedKinds(t *testing.T) {
	tests := []struct {
		name              string
		controllerName    string
		listener          gwv1.Listener
		expectedSupported bool
		expectedCount     int
	}{
		{
			name:           "ALB HTTP listener default kinds",
			controllerName: gateway_constants.ALBGatewayController,
			listener: gwv1.Listener{
				Protocol:      gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{},
			},
			expectedSupported: true,
			expectedCount:     1,
		},
		{
			name:           "ALB HTTPS listener default kinds",
			controllerName: gateway_constants.ALBGatewayController,
			listener: gwv1.Listener{
				Protocol:      gwv1.HTTPSProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{},
			},
			expectedSupported: true,
			expectedCount:     2,
		},
		{
			name:           "NLB TCP listener default kinds",
			controllerName: gateway_constants.NLBGatewayController,
			listener: gwv1.Listener{
				Protocol:      gwv1.TCPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{},
			},
			expectedSupported: true,
			expectedCount:     1,
		},
		{
			name:           "ALB with valid explicit kinds",
			controllerName: gateway_constants.ALBGatewayController,
			listener: gwv1.Listener{
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{
						{Kind: gwv1.Kind(HTTPRouteKind)},
					},
				},
			},
			expectedSupported: true,
			expectedCount:     1,
		},
		{
			name:           "ALB with invalid explicit kinds",
			controllerName: gateway_constants.ALBGatewayController,
			listener: gwv1.Listener{
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{
						{Kind: gwv1.Kind(TCPRouteKind)},
					},
				},
			},
			expectedSupported: false,
			expectedCount:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kinds, supported := getSupportedKinds(tt.controllerName, tt.listener)

			assert.Equal(t, tt.expectedSupported, supported)
			assert.Equal(t, tt.expectedCount, len(kinds))
		})
	}
}

func TestValidateListeners_ListenerSets(t *testing.T) {
	lsNN := func(ns, name string) types.NamespacedName {
		return types.NamespacedName{Namespace: ns, Name: name}
	}

	makeLS := func(ns, name string, creationTime time.Time) gwv1.ListenerSet {
		return gwv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         ns,
				CreationTimestamp: metav1.NewTime(creationTime),
			},
		}
	}

	makeLSSources := func(ls gwv1.ListenerSet, listeners ...gwv1.Listener) []listenerSetListenerSource {
		sources := make([]listenerSetListenerSource, 0, len(listeners))
		for _, l := range listeners {
			sources = append(sources, listenerSetListenerSource{parentRef: ls, listener: l})
		}
		return sources
	}

	tests := []struct {
		name                     string
		gatewayListeners         []gwv1.Listener
		listenerSetLoadResult    listenerSetLoadResult
		controllerName           string
		expectedGatewayHasErrors bool
		expectedHasErrors        bool
		expectedLSValidationKeys []types.NamespacedName
		expectedLSReasons        map[types.NamespacedName]map[gwv1.SectionName]gwv1.ListenerConditionReason
		expectedGatewayReasons   map[gwv1.SectionName]gwv1.ListenerConditionReason
	}{
		{
			name:             "valid listener set listener - no conflicts",
			gatewayListeners: []gwv1.Listener{},
			listenerSetLoadResult: func() listenerSetLoadResult {
				ls := makeLS("ns1", "ls1", time.Now())
				return listenerSetLoadResult{
					listenersPerListenerSet: map[types.NamespacedName][]listenerSetListenerSource{
						lsNN("ns1", "ls1"): makeLSSources(ls,
							gwv1.Listener{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
						),
					},
					acceptedListenerSets: map[types.NamespacedName]gwv1.ListenerSet{
						lsNN(ls.Name, ls.Namespace): ls,
					},
				}
			}(),
			controllerName:           gateway_constants.ALBGatewayController,
			expectedGatewayHasErrors: false,
			expectedHasErrors:        false,
			expectedLSValidationKeys: []types.NamespacedName{lsNN("ns1", "ls1")},
			expectedLSReasons: map[types.NamespacedName]map[gwv1.SectionName]gwv1.ListenerConditionReason{
				lsNN("ns1", "ls1"): {"http": gwv1.ListenerReasonAccepted},
			},
		},
		{
			name: "listener set conflicts with gateway listener - LS gets protocol conflict",
			gatewayListeners: []gwv1.Listener{
				{Name: "gw-http", Port: 80, Protocol: gwv1.HTTPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			listenerSetLoadResult: func() listenerSetLoadResult {
				ls := makeLS("ns1", "ls1", time.Now())
				return listenerSetLoadResult{
					listenersPerListenerSet: map[types.NamespacedName][]listenerSetListenerSource{
						lsNN("ns1", "ls1"): makeLSSources(ls,
							gwv1.Listener{Name: "ls-https", Port: 80, Protocol: gwv1.HTTPSProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
						),
					},
					acceptedListenerSets: map[types.NamespacedName]gwv1.ListenerSet{
						lsNN(ls.Name, ls.Namespace): ls,
					},
				}
			}(),
			controllerName:           gateway_constants.ALBGatewayController,
			expectedGatewayHasErrors: false,
			expectedHasErrors:        true,
			expectedGatewayReasons:   map[gwv1.SectionName]gwv1.ListenerConditionReason{"gw-http": gwv1.ListenerReasonAccepted},
			expectedLSValidationKeys: []types.NamespacedName{lsNN("ns1", "ls1")},
			expectedLSReasons: map[types.NamespacedName]map[gwv1.SectionName]gwv1.ListenerConditionReason{
				lsNN("ns1", "ls1"): {"ls-https": gwv1.ListenerReasonProtocolConflict},
			},
		},
		{
			name: "listener set hostname conflict with gateway listener",
			gatewayListeners: []gwv1.Listener{
				{Name: "gw-http", Port: 80, Protocol: gwv1.HTTPProtocolType, Hostname: (*gwv1.Hostname)(&[]string{"example.com"}[0]), AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			listenerSetLoadResult: func() listenerSetLoadResult {
				ls := makeLS("ns1", "ls1", time.Now())
				return listenerSetLoadResult{
					listenersPerListenerSet: map[types.NamespacedName][]listenerSetListenerSource{
						lsNN("ns1", "ls1"): makeLSSources(ls,
							gwv1.Listener{Name: "ls-http", Port: 80, Protocol: gwv1.HTTPProtocolType, Hostname: (*gwv1.Hostname)(&[]string{"example.com"}[0]), AllowedRoutes: &gwv1.AllowedRoutes{}},
						),
					},
					acceptedListenerSets: map[types.NamespacedName]gwv1.ListenerSet{
						lsNN(ls.Name, ls.Namespace): ls,
					},
				}
			}(),
			controllerName:           gateway_constants.ALBGatewayController,
			expectedGatewayHasErrors: false,
			expectedHasErrors:        true,
			expectedGatewayReasons:   map[gwv1.SectionName]gwv1.ListenerConditionReason{"gw-http": gwv1.ListenerReasonAccepted},
			expectedLSReasons: map[types.NamespacedName]map[gwv1.SectionName]gwv1.ListenerConditionReason{
				lsNN("ns1", "ls1"): {"ls-http": gwv1.ListenerReasonHostnameConflict},
			},
		},
		{
			name:             "listener set with invalid protocol for ALB",
			gatewayListeners: []gwv1.Listener{},
			listenerSetLoadResult: func() listenerSetLoadResult {
				ls := makeLS("ns1", "ls1", time.Now())
				return listenerSetLoadResult{
					listenersPerListenerSet: map[types.NamespacedName][]listenerSetListenerSource{
						lsNN("ns1", "ls1"): makeLSSources(ls,
							gwv1.Listener{Name: "tcp", Port: 80, Protocol: gwv1.TCPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
						),
					},
					acceptedListenerSets: map[types.NamespacedName]gwv1.ListenerSet{
						lsNN(ls.Name, ls.Namespace): ls,
					},
				}
			}(),
			controllerName:           gateway_constants.ALBGatewayController,
			expectedGatewayHasErrors: false,
			expectedHasErrors:        true,
			expectedLSReasons: map[types.NamespacedName]map[gwv1.SectionName]gwv1.ListenerConditionReason{
				lsNN("ns1", "ls1"): {"tcp": gwv1.ListenerReasonUnsupportedProtocol},
			},
		},
		{
			name:             "multiple listener sets - older LS takes priority over newer LS on conflict",
			gatewayListeners: []gwv1.Listener{},
			listenerSetLoadResult: func() listenerSetLoadResult {
				ls1 := makeLS("ns1", "ls-older", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
				ls2 := makeLS("ns1", "ls-newer", time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))
				return listenerSetLoadResult{
					listenersPerListenerSet: map[types.NamespacedName][]listenerSetListenerSource{
						lsNN("ns1", "ls-older"): makeLSSources(ls1,
							gwv1.Listener{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
						),
						lsNN("ns1", "ls-newer"): makeLSSources(ls2,
							gwv1.Listener{Name: "https", Port: 80, Protocol: gwv1.HTTPSProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
						),
					},
					acceptedListenerSets: map[types.NamespacedName]gwv1.ListenerSet{
						lsNN(ls1.Name, ls1.Namespace): ls1,
						lsNN(ls2.Name, ls2.Namespace): ls2,
					},
				}
			}(),
			controllerName:           gateway_constants.ALBGatewayController,
			expectedGatewayHasErrors: false,
			expectedHasErrors:        true,
			expectedLSReasons: map[types.NamespacedName]map[gwv1.SectionName]gwv1.ListenerConditionReason{
				lsNN("ns1", "ls-older"): {"http": gwv1.ListenerReasonAccepted},
				lsNN("ns1", "ls-newer"): {"https": gwv1.ListenerReasonProtocolConflict},
			},
		},
		{
			name:             "empty listener set load result",
			gatewayListeners: []gwv1.Listener{},
			listenerSetLoadResult: listenerSetLoadResult{
				listenersPerListenerSet: map[types.NamespacedName][]listenerSetListenerSource{},
				acceptedListenerSets:    map[types.NamespacedName]gwv1.ListenerSet{},
			},
			controllerName:           gateway_constants.ALBGatewayController,
			expectedGatewayHasErrors: false,
			expectedHasErrors:        false,
		},
		{
			name: "gateway and listener set both valid on different ports",
			gatewayListeners: []gwv1.Listener{
				{Name: "gw-http", Port: 80, Protocol: gwv1.HTTPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			listenerSetLoadResult: func() listenerSetLoadResult {
				ls := makeLS("ns1", "ls1", time.Now())
				return listenerSetLoadResult{
					listenersPerListenerSet: map[types.NamespacedName][]listenerSetListenerSource{
						lsNN("ns1", "ls1"): makeLSSources(ls,
							gwv1.Listener{Name: "ls-https", Port: 443, Protocol: gwv1.HTTPSProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
						),
					},
					acceptedListenerSets: map[types.NamespacedName]gwv1.ListenerSet{
						lsNN(ls.Name, ls.Namespace): ls,
					},
				}
			}(),
			controllerName:           gateway_constants.ALBGatewayController,
			expectedGatewayHasErrors: false,
			expectedHasErrors:        false,
			expectedGatewayReasons:   map[gwv1.SectionName]gwv1.ListenerConditionReason{"gw-http": gwv1.ListenerReasonAccepted},
			expectedLSReasons: map[types.NamespacedName]map[gwv1.SectionName]gwv1.ListenerConditionReason{
				lsNN("ns1", "ls1"): {"ls-https": gwv1.ListenerReasonAccepted},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := allListeners{
				GatewayListeners:     tt.gatewayListeners,
				ListenerSetListeners: tt.listenerSetLoadResult,
			}
			result := validateListeners(input, tt.controllerName)

			assert.Equal(t, tt.expectedGatewayHasErrors, result.GatewayListenerValidation.HasErrors)
			assert.Equal(t, tt.expectedHasErrors, result.HasErrors())

			if tt.expectedGatewayReasons != nil {
				for name, expectedReason := range tt.expectedGatewayReasons {
					actual, ok := result.GatewayListenerValidation.Results[name]
					assert.True(t, ok, "expected gateway listener result for %s", name)
					assert.Equal(t, expectedReason, actual.Reason)
				}
			}

			if tt.expectedLSValidationKeys != nil {
				assert.Len(t, result.ListenerSetListenerValidation, len(tt.expectedLSValidationKeys))
			}

			if tt.expectedLSReasons != nil {
				for lsKey, listenerReasons := range tt.expectedLSReasons {
					lsResult, ok := result.ListenerSetListenerValidation[lsKey]
					assert.True(t, ok, "expected listener set validation for %s", lsKey)
					for listenerName, expectedReason := range listenerReasons {
						actual, ok := lsResult.Results[listenerName]
						assert.True(t, ok, "expected listener result for %s in %s", listenerName, lsKey)
						assert.Equal(t, expectedReason, actual.Reason)
					}
				}
			}
		})
	}
}
