package crddetect

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
)

func TestApplyGatewayFeatureFlags(t *testing.T) {
	testCases := []struct {
		name         string
		presentKinds map[string]sets.Set[string]
		albEnabled   bool
		nlbEnabled   bool
	}{
		{
			name:         "no kinds present",
			presentKinds: map[string]sets.Set[string]{},
			albEnabled:   false,
			nlbEnabled:   false,
		},
		{
			name: "v1 present",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion: sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"),
			},
			albEnabled: true,
			nlbEnabled: false,
		},
		{
			name: "alpha2 present",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
			},
			albEnabled: false,
			nlbEnabled: false,
		},
		{
			name: "all present",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
			},
			albEnabled: true,
			nlbEnabled: true,
		},
		{
			name: "gateway missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
			},
			albEnabled: false,
			nlbEnabled: false,
		},
		{
			name: "gateway class missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "HTTPRoute", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
			},
			albEnabled: false,
			nlbEnabled: false,
		},
		{
			name: "httproute missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
			},
			albEnabled: false,
			nlbEnabled: true,
		},
		{
			name: "grpcroute missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
			},
			albEnabled: false,
			nlbEnabled: true,
		},
		{
			name: "tlsroute missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
			},
			albEnabled: true,
			nlbEnabled: false,
		},
		{
			name: "tcproute missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("UDPRoute"),
			},
			albEnabled: true,
			nlbEnabled: false,
		},
		{
			name: "udproute missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute"),
			},
			albEnabled: true,
			nlbEnabled: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.NewFeatureGates()
			applyGatewayFeatureFlags(tc.presentKinds, cfg, logr.Discard())

			assert.Equal(t, tc.albEnabled, cfg.Enabled(config.ALBGatewayAPI))
			assert.Equal(t, tc.nlbEnabled, cfg.Enabled(config.NLBGatewayAPI))
		})
	}
}
