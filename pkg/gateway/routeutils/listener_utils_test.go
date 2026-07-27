package routeutils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/gateway/constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type mockListenerSetLoader struct {
	result   listenerSetLoadResult
	rejected []gwv1.ListenerSet
	error    error
}

func (l *mockListenerSetLoader) retrieveListenersFromListenerSets(ctx context.Context, gateway gwv1.Gateway) (listenerSetLoadResult, []gwv1.ListenerSet, error) {
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
			result := validateListeners(allListeners{GatewayListeners: tt.listeners}, 0, tt.controllerName)

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
			result := validateListeners(input, 0, tt.controllerName)

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

func TestCalculateAttachedListenerSets(t *testing.T) {
	tests := []struct {
		name     string
		input    map[types.NamespacedName]ListenerValidationResults
		expected int32
	}{
		{
			name:     "nil map returns zero",
			input:    nil,
			expected: 0,
		},
		{
			name:     "empty map returns zero",
			input:    map[types.NamespacedName]ListenerValidationResults{},
			expected: 0,
		},
		{
			name: "single listener set with one valid listener",
			input: map[types.NamespacedName]ListenerValidationResults{
				{Namespace: "ns1", Name: "ls1"}: {
					Results: map[gwv1.SectionName]ListenerValidationResult{
						"listener-1": {ListenerName: "listener-1", IsValid: true},
					},
				},
			},
			expected: 1,
		},
		{
			name: "single listener set with one invalid listener",
			input: map[types.NamespacedName]ListenerValidationResults{
				{Namespace: "ns1", Name: "ls1"}: {
					Results: map[gwv1.SectionName]ListenerValidationResult{
						"listener-1": {ListenerName: "listener-1", IsValid: false},
					},
				},
			},
			expected: 0,
		},
		{
			name: "single listener set with mixed valid and invalid listeners counts as one",
			input: map[types.NamespacedName]ListenerValidationResults{
				{Namespace: "ns1", Name: "ls1"}: {
					Results: map[gwv1.SectionName]ListenerValidationResult{
						"listener-1": {ListenerName: "listener-1", IsValid: false},
						"listener-2": {ListenerName: "listener-2", IsValid: true},
						"listener-3": {ListenerName: "listener-3", IsValid: false},
					},
				},
			},
			expected: 1,
		},
		{
			name: "multiple listener sets all valid",
			input: map[types.NamespacedName]ListenerValidationResults{
				{Namespace: "ns1", Name: "ls1"}: {
					Results: map[gwv1.SectionName]ListenerValidationResult{
						"listener-1": {ListenerName: "listener-1", IsValid: true},
					},
				},
				{Namespace: "ns1", Name: "ls2"}: {
					Results: map[gwv1.SectionName]ListenerValidationResult{
						"listener-2": {ListenerName: "listener-2", IsValid: true},
					},
				},
				{Namespace: "ns2", Name: "ls3"}: {
					Results: map[gwv1.SectionName]ListenerValidationResult{
						"listener-3": {ListenerName: "listener-3", IsValid: true},
					},
				},
			},
			expected: 3,
		},
		{
			name: "multiple listener sets some with no valid listeners",
			input: map[types.NamespacedName]ListenerValidationResults{
				{Namespace: "ns1", Name: "ls1"}: {
					Results: map[gwv1.SectionName]ListenerValidationResult{
						"listener-1": {ListenerName: "listener-1", IsValid: true},
					},
				},
				{Namespace: "ns1", Name: "ls2"}: {
					Results: map[gwv1.SectionName]ListenerValidationResult{
						"listener-2": {ListenerName: "listener-2", IsValid: false},
						"listener-3": {ListenerName: "listener-3", IsValid: false},
					},
				},
				{Namespace: "ns2", Name: "ls3"}: {
					Results: map[gwv1.SectionName]ListenerValidationResult{
						"listener-4": {ListenerName: "listener-4", IsValid: true},
						"listener-5": {ListenerName: "listener-5", IsValid: false},
					},
				},
			},
			expected: 2,
		},
		{
			name: "listener set with empty results map",
			input: map[types.NamespacedName]ListenerValidationResults{
				{Namespace: "ns1", Name: "ls1"}: {
					Results: map[gwv1.SectionName]ListenerValidationResult{},
				},
			},
			expected: 0,
		},
		{
			name: "all listener sets have only invalid listeners",
			input: map[types.NamespacedName]ListenerValidationResults{
				{Namespace: "ns1", Name: "ls1"}: {
					Results: map[gwv1.SectionName]ListenerValidationResult{
						"listener-1": {ListenerName: "listener-1", IsValid: false},
					},
				},
				{Namespace: "ns2", Name: "ls2"}: {
					Results: map[gwv1.SectionName]ListenerValidationResult{
						"listener-2": {ListenerName: "listener-2", IsValid: false},
					},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateAttachedListenerSets(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateListenerList(t *testing.T) {
	hostname := func(h string) *gwv1.Hostname {
		hn := gwv1.Hostname(h)
		return &hn
	}

	type expectedListener struct {
		isValid bool
		reason  gwv1.ListenerConditionReason
	}

	tests := []struct {
		name             string
		listenerList     []gwv1.Listener
		portHostnameMap  map[string]bool
		portProtocolMap  map[gwv1.PortNumber]gwv1.ProtocolType
		controllerName   string
		generation       int64
		expectedHasError bool
		expected         map[gwv1.SectionName]expectedListener
	}{
		{
			name:             "empty listener list produces no results",
			listenerList:     []gwv1.Listener{},
			controllerName:   gateway_constants.ALBGatewayController,
			generation:       7,
			expectedHasError: false,
			expected:         map[gwv1.SectionName]expectedListener{},
		},
		{
			name: "valid ALB HTTP listener is accepted",
			listenerList: []gwv1.Listener{
				{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: false,
			expected: map[gwv1.SectionName]expectedListener{
				"http": {isValid: true, reason: gwv1.ListenerReasonAccepted},
			},
		},
		{
			name: "unsupported route kind is rejected",
			listenerList: []gwv1.Listener{
				{
					Name:     "bad-kind",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{{Kind: gwv1.Kind(TCPRouteKind)}},
					},
				},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"bad-kind": {isValid: false, reason: gwv1.ListenerReasonInvalidRouteKinds},
			},
		},
		{
			name: "port below minimum is unavailable",
			listenerList: []gwv1.Listener{
				{Name: "low-port", Port: 0, Protocol: gwv1.HTTPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"low-port": {isValid: false, reason: gwv1.ListenerReasonPortUnavailable},
			},
		},
		{
			name: "port above maximum is unavailable",
			listenerList: []gwv1.Listener{
				{Name: "high-port", Port: 65536, Protocol: gwv1.HTTPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"high-port": {isValid: false, reason: gwv1.ListenerReasonPortUnavailable},
			},
		},
		{
			name: "ALB rejects TCP protocol",
			listenerList: []gwv1.Listener{
				{Name: "tcp", Port: 80, Protocol: gwv1.TCPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"tcp": {isValid: false, reason: gwv1.ListenerReasonUnsupportedProtocol},
			},
		},
		{
			name: "ALB rejects UDP protocol",
			listenerList: []gwv1.Listener{
				{Name: "udp", Port: 80, Protocol: gwv1.UDPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"udp": {isValid: false, reason: gwv1.ListenerReasonUnsupportedProtocol},
			},
		},
		{
			name: "ALB rejects TLS protocol",
			listenerList: []gwv1.Listener{
				{Name: "tls", Port: 443, Protocol: gwv1.TLSProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"tls": {isValid: false, reason: gwv1.ListenerReasonUnsupportedProtocol},
			},
		},
		{
			name: "NLB rejects HTTP protocol",
			listenerList: []gwv1.Listener{
				{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.NLBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"http": {isValid: false, reason: gwv1.ListenerReasonUnsupportedProtocol},
			},
		},
		{
			name: "NLB rejects HTTPS protocol",
			listenerList: []gwv1.Listener{
				{Name: "https", Port: 443, Protocol: gwv1.HTTPSProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.NLBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"https": {isValid: false, reason: gwv1.ListenerReasonUnsupportedProtocol},
			},
		},
		{
			name: "ALB rejects unknown protocol",
			listenerList: []gwv1.Listener{
				{Name: "foo", Port: 80, Protocol: gwv1.ProtocolType("FOO"), AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"foo": {isValid: false, reason: gwv1.ListenerReasonUnsupportedProtocol},
			},
		},
		{
			name: "NLB rejects unknown protocol",
			listenerList: []gwv1.Listener{
				{Name: "foo", Port: 80, Protocol: gwv1.ProtocolType("FOO"), AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.NLBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"foo": {isValid: false, reason: gwv1.ListenerReasonUnsupportedProtocol},
			},
		},
		{
			name: "NLB accepts TLS protocol",
			listenerList: []gwv1.Listener{
				{Name: "tls", Port: 443, Protocol: gwv1.TLSProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.NLBGatewayController,
			expectedHasError: false,
			expected: map[gwv1.SectionName]expectedListener{
				"tls": {isValid: true, reason: gwv1.ListenerReasonAccepted},
			},
		},
		{
			name: "ALB accepts HTTPS protocol",
			listenerList: []gwv1.Listener{
				{Name: "https", Port: 443, Protocol: gwv1.HTTPSProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: false,
			expected: map[gwv1.SectionName]expectedListener{
				"https": {isValid: true, reason: gwv1.ListenerReasonAccepted},
			},
		},
		{
			name: "protocol conflict on same port with incompatible protocols",
			listenerList: []gwv1.Listener{
				{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
				{Name: "https", Port: 80, Protocol: gwv1.HTTPSProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"http":  {isValid: true, reason: gwv1.ListenerReasonAccepted},
				"https": {isValid: false, reason: gwv1.ListenerReasonProtocolConflict},
			},
		},
		{
			name: "TCP then UDP on same port is allowed",
			listenerList: []gwv1.Listener{
				{Name: "tcp", Port: 80, Protocol: gwv1.TCPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
				{Name: "udp", Port: 80, Protocol: gwv1.UDPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.NLBGatewayController,
			expectedHasError: false,
			expected: map[gwv1.SectionName]expectedListener{
				"tcp": {isValid: true, reason: gwv1.ListenerReasonAccepted},
				"udp": {isValid: true, reason: gwv1.ListenerReasonAccepted},
			},
		},
		{
			name: "UDP then TCP on same port is allowed",
			listenerList: []gwv1.Listener{
				{Name: "udp", Port: 80, Protocol: gwv1.UDPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
				{Name: "tcp", Port: 80, Protocol: gwv1.TCPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.NLBGatewayController,
			expectedHasError: false,
			expected: map[gwv1.SectionName]expectedListener{
				"udp": {isValid: true, reason: gwv1.ListenerReasonAccepted},
				"tcp": {isValid: true, reason: gwv1.ListenerReasonAccepted},
			},
		},
		{
			name: "same protocol on same port does not conflict",
			listenerList: []gwv1.Listener{
				{Name: "http-a", Port: 80, Protocol: gwv1.HTTPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
				{Name: "http-b", Port: 80, Protocol: gwv1.HTTPProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: false,
			expected: map[gwv1.SectionName]expectedListener{
				"http-a": {isValid: true, reason: gwv1.ListenerReasonAccepted},
				"http-b": {isValid: true, reason: gwv1.ListenerReasonAccepted},
			},
		},
		{
			name: "listener with hostname and no conflict is accepted",
			listenerList: []gwv1.Listener{
				{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType, Hostname: hostname("example.com"), AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: false,
			expected: map[gwv1.SectionName]expectedListener{
				"http": {isValid: true, reason: gwv1.ListenerReasonAccepted},
			},
		},
		{
			name: "hostname conflict on same port and hostname",
			listenerList: []gwv1.Listener{
				{Name: "http-a", Port: 80, Protocol: gwv1.HTTPProtocolType, Hostname: hostname("example.com"), AllowedRoutes: &gwv1.AllowedRoutes{}},
				{Name: "http-b", Port: 80, Protocol: gwv1.HTTPProtocolType, Hostname: hostname("example.com"), AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"http-a": {isValid: true, reason: gwv1.ListenerReasonAccepted},
				"http-b": {isValid: false, reason: gwv1.ListenerReasonHostnameConflict},
			},
		},
		{
			name: "different hostnames on same port are accepted",
			listenerList: []gwv1.Listener{
				{Name: "http-a", Port: 80, Protocol: gwv1.HTTPProtocolType, Hostname: hostname("a.example.com"), AllowedRoutes: &gwv1.AllowedRoutes{}},
				{Name: "http-b", Port: 80, Protocol: gwv1.HTTPProtocolType, Hostname: hostname("b.example.com"), AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: false,
			expected: map[gwv1.SectionName]expectedListener{
				"http-a": {isValid: true, reason: gwv1.ListenerReasonAccepted},
				"http-b": {isValid: true, reason: gwv1.ListenerReasonAccepted},
			},
		},
		{
			name: "pre-seeded protocol map causes conflict on first listener",
			listenerList: []gwv1.Listener{
				{Name: "https", Port: 80, Protocol: gwv1.HTTPSProtocolType, AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			portProtocolMap: map[gwv1.PortNumber]gwv1.ProtocolType{
				80: gwv1.HTTPProtocolType,
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"https": {isValid: false, reason: gwv1.ListenerReasonProtocolConflict},
			},
		},
		{
			name: "pre-seeded hostname map causes conflict on first listener",
			listenerList: []gwv1.Listener{
				{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType, Hostname: hostname("example.com"), AllowedRoutes: &gwv1.AllowedRoutes{}},
			},
			portHostnameMap: map[string]bool{
				"80-example.com": true,
			},
			portProtocolMap: map[gwv1.PortNumber]gwv1.ProtocolType{
				80: gwv1.HTTPProtocolType,
			},
			controllerName:   gateway_constants.ALBGatewayController,
			expectedHasError: true,
			expected: map[gwv1.SectionName]expectedListener{
				"http": {isValid: false, reason: gwv1.ListenerReasonHostnameConflict},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			portHostnameMap := tt.portHostnameMap
			if portHostnameMap == nil {
				portHostnameMap = make(map[string]bool)
			}
			portProtocolMap := tt.portProtocolMap
			if portProtocolMap == nil {
				portProtocolMap = make(map[gwv1.PortNumber]gwv1.ProtocolType)
			}

			results := validateListenerList(tt.listenerList, portHostnameMap, portProtocolMap, tt.controllerName, tt.generation)

			assert.Equal(t, tt.generation, results.Generation)
			assert.Equal(t, tt.expectedHasError, results.HasErrors)
			assert.Len(t, results.Results, len(tt.expected))

			for name, exp := range tt.expected {
				actual, ok := results.Results[name]
				assert.True(t, ok, "expected result for listener %s", name)
				assert.Equal(t, exp.isValid, actual.IsValid, "IsValid mismatch for listener %s", name)
				assert.Equal(t, exp.reason, actual.Reason, "Reason mismatch for listener %s", name)
				assert.Equal(t, name, actual.ListenerName)
			}
		})
	}
}
