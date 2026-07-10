package crddetect

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
)

func Test_resolveRouteVersions(t *testing.T) {
	testCases := []struct {
		name      string
		available map[string]sets.Set[string]
		expected  RouteVersions
	}{
		{
			name: "v1 served - use v1 (gateway api >= 1.6)",
			available: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New("Gateway", "GatewayClass", "TCPRoute", "UDPRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string](),
			},
			expected: RouteVersions{
				TCPRouteGroupVersion: GatewayV1GroupVersion,
				UDPRouteGroupVersion: GatewayV1GroupVersion,
			},
		},
		{
			name: "only v1alpha2 served - fall back (gateway api < 1.6)",
			available: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New("Gateway", "GatewayClass", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New("TCPRoute", "UDPRoute"),
			},
			expected: RouteVersions{
				TCPRouteGroupVersion: GatewayV1Alpha2GroupVersion,
				UDPRouteGroupVersion: GatewayV1Alpha2GroupVersion,
			},
		},
		{
			name: "both served - prefer v1 (gateway api 1.6 experimental channel)",
			available: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New("TCPRoute", "UDPRoute"),
				GatewayV1Alpha2GroupVersion: sets.New("TCPRoute", "UDPRoute"),
			},
			expected: RouteVersions{
				TCPRouteGroupVersion: GatewayV1GroupVersion,
				UDPRouteGroupVersion: GatewayV1GroupVersion,
			},
		},
		{
			name:      "neither served - default to v1alpha2 (today's behavior)",
			available: map[string]sets.Set[string]{},
			expected: RouteVersions{
				TCPRouteGroupVersion: GatewayV1Alpha2GroupVersion,
				UDPRouteGroupVersion: GatewayV1Alpha2GroupVersion,
			},
		},
		{
			name: "mixed - tcp on v1, udp only on v1alpha2",
			available: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New("TCPRoute"),
				GatewayV1Alpha2GroupVersion: sets.New("UDPRoute"),
			},
			expected: RouteVersions{
				TCPRouteGroupVersion: GatewayV1GroupVersion,
				UDPRouteGroupVersion: GatewayV1Alpha2GroupVersion,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, resolveRouteVersions(tc.available))
		})
	}
}

func TestRouteVersions_IsV1(t *testing.T) {
	v1Versions := RouteVersions{TCPRouteGroupVersion: GatewayV1GroupVersion, UDPRouteGroupVersion: GatewayV1GroupVersion}
	assert.True(t, v1Versions.IsTCPRouteV1())
	assert.True(t, v1Versions.IsUDPRouteV1())

	alphaVersions := RouteVersions{TCPRouteGroupVersion: GatewayV1Alpha2GroupVersion, UDPRouteGroupVersion: GatewayV1Alpha2GroupVersion}
	assert.False(t, alphaVersions.IsTCPRouteV1())
	assert.False(t, alphaVersions.IsUDPRouteV1())
}
