package routeutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestValidateListeners(t *testing.T) {
	tests := []struct {
		name                string
		discoveredListeners []DiscoveredListener
		controllerName      string
		expectedErrors      bool
		expectedCount       int
		expectedReasons     []gwv1.ListenerConditionReason
	}{
		{
			name:                "empty listeners",
			discoveredListeners: []DiscoveredListener{},
			controllerName:      gateway_constants.ALBGatewayController,
			expectedErrors:      false,
			expectedCount:       0,
		},
		{
			name: "valid HTTP listener",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
						Name:          "http",
						Port:          80,
						Protocol:      gwv1.HTTPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  false,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonAccepted},
		},
		{
			name: "invalid port - too low",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
						Name:          "invalid-low",
						Port:          0,
						Protocol:      gwv1.HTTPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonPortUnavailable},
		},
		{
			name: "invalid port - too high",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
						Name:          "invalid-high",
						Port:          65536,
						Protocol:      gwv1.HTTPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonPortUnavailable},
		},
		{
			name: "ALB unsupported TCP protocol",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
						Name:          "tcp",
						Port:          80,
						Protocol:      gwv1.TCPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonUnsupportedProtocol},
		},
		{
			name: "ALB unsupported UDP protocol",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
						Name:          "udp",
						Port:          80,
						Protocol:      gwv1.UDPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonUnsupportedProtocol},
		},
		{
			name: "ALB unsupported TLS protocol",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
						Name:          "tls",
						Port:          80,
						Protocol:      gwv1.TLSProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonUnsupportedProtocol},
		},
		{
			name: "NLB unsupported HTTP protocol",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
						Name:          "http",
						Port:          80,
						Protocol:      gwv1.HTTPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
			},
			controllerName:  gateway_constants.NLBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonUnsupportedProtocol},
		},
		{
			name: "NLB unsupported HTTPS protocol",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
						Name:          "https",
						Port:          443,
						Protocol:      gwv1.HTTPSProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
			},
			controllerName:  gateway_constants.NLBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonUnsupportedProtocol},
		},
		{
			name: "protocol conflict - HTTP vs HTTPS",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
						Name:          "http",
						Port:          80,
						Protocol:      gwv1.HTTPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
				{
					Listener: gwv1.Listener{
						Name:          "https",
						Port:          80,
						Protocol:      gwv1.HTTPSProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   2,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonAccepted, gwv1.ListenerReasonProtocolConflict},
		},
		{
			name: "TCP+UDP allowed on same port",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
						Name:          "tcp",
						Port:          80,
						Protocol:      gwv1.TCPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
				{
					Listener: gwv1.Listener{
						Name:          "udp",
						Port:          80,
						Protocol:      gwv1.UDPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
			},
			controllerName:  gateway_constants.NLBGatewayController,
			expectedErrors:  false,
			expectedCount:   2,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonAccepted, gwv1.ListenerReasonAccepted},
		},
		{
			name: "hostname conflict",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
						Name:          "http1",
						Port:          80,
						Protocol:      gwv1.HTTPProtocolType,
						Hostname:      (*gwv1.Hostname)(&[]string{"example.com"}[0]),
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
				{
					Listener: gwv1.Listener{
						Name:          "http2",
						Port:          80,
						Protocol:      gwv1.HTTPProtocolType,
						Hostname:      (*gwv1.Hostname)(&[]string{"example.com"}[0]),
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   2,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonAccepted, gwv1.ListenerReasonHostnameConflict},
		},
		{
			name: "different hostnames on same port - valid",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
						Name:          "http1",
						Port:          80,
						Protocol:      gwv1.HTTPProtocolType,
						Hostname:      (*gwv1.Hostname)(&[]string{"example.com"}[0]),
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
				{
					Listener: gwv1.Listener{
						Name:          "http2",
						Port:          80,
						Protocol:      gwv1.HTTPProtocolType,
						Hostname:      (*gwv1.Hostname)(&[]string{"test.com"}[0]),
						AllowedRoutes: &gwv1.AllowedRoutes{},
					},
				},
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  false,
			expectedCount:   2,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonAccepted, gwv1.ListenerReasonAccepted},
		},
		{
			name: "invalid route kinds",
			discoveredListeners: []DiscoveredListener{
				{
					Listener: gwv1.Listener{
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
			},
			controllerName:  gateway_constants.ALBGatewayController,
			expectedErrors:  true,
			expectedCount:   1,
			expectedReasons: []gwv1.ListenerConditionReason{gwv1.ListenerReasonInvalidRouteKinds},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateListeners(tt.discoveredListeners, tt.controllerName)

			assert.Equal(t, tt.expectedErrors, result.HasErrors)
			assert.Equal(t, tt.expectedCount, len(result.Results))

			if len(tt.expectedReasons) > 0 {
				reasons := make([]gwv1.ListenerConditionReason, 0, len(result.Results))
				for _, res := range result.Results {
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
