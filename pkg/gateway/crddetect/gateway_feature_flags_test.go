package crddetect

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
)

// fakeDiscoveryClient is a test double for k8s.DiscoveryClient.
type fakeDiscoveryClient struct {
	resources map[string][]string
}

func (f *fakeDiscoveryClient) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	kinds, ok := f.resources[groupVersion]
	if !ok {
		return &metav1.APIResourceList{}, nil
	}
	list := &metav1.APIResourceList{GroupVersion: groupVersion}
	for _, kind := range kinds {
		list.APIResources = append(list.APIResources, metav1.APIResource{Kind: kind})
	}
	return list, nil
}

func TestApplyGatewayFeatureFlags(t *testing.T) {
	lbcKinds := sets.New[string]("TargetGroupConfiguration", "LoadBalancerConfiguration", "ListenerRuleConfiguration")

	testCases := []struct {
		name               string
		presentKinds       map[string]sets.Set[string]
		albEnabled         bool
		nlbEnabled         bool
		listenerSetEnabled bool
	}{
		{
			name:               "no kinds present",
			presentKinds:       map[string]sets.Set[string]{},
			albEnabled:         false,
			nlbEnabled:         false,
			listenerSetEnabled: false,
		},
		{
			name: "v1 present but LBC CRDs missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion: sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"),
			},
			albEnabled:         false,
			nlbEnabled:         false,
			listenerSetEnabled: false,
		},
		{
			name: "v1 and LBC CRDs present",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:  sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"),
				LBCGatewayGroupVersion: lbcKinds,
			},
			albEnabled:         true,
			nlbEnabled:         false,
			listenerSetEnabled: false,
		},
		{
			name: "alpha2 present",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
			},
			albEnabled:         false,
			nlbEnabled:         false,
			listenerSetEnabled: false,
		},
		{
			name: "all standard CRDs present but LBC CRDs missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
			},
			albEnabled:         false,
			nlbEnabled:         false,
			listenerSetEnabled: false,
		},
		{
			name: "all present including LBC CRDs but ListenerSet missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
				LBCGatewayGroupVersion:      lbcKinds,
			},
			albEnabled:         true,
			nlbEnabled:         true,
			listenerSetEnabled: false,
		},
		{
			name: "all present including ListenerSet and LBC CRDs",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute", "ListenerSet"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
				LBCGatewayGroupVersion:      lbcKinds,
			},
			albEnabled:         true,
			nlbEnabled:         true,
			listenerSetEnabled: true,
		},
		{
			name: "gateway missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
				LBCGatewayGroupVersion:      lbcKinds,
			},
			albEnabled:         false,
			nlbEnabled:         false,
			listenerSetEnabled: false,
		},
		{
			name: "gateway class missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "HTTPRoute", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
				LBCGatewayGroupVersion:      lbcKinds,
			},
			albEnabled:         false,
			nlbEnabled:         false,
			listenerSetEnabled: false,
		},
		{
			name: "httproute missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
				LBCGatewayGroupVersion:      lbcKinds,
			},
			albEnabled:         false,
			nlbEnabled:         true,
			listenerSetEnabled: false,
		},
		{
			name: "grpcroute missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
				LBCGatewayGroupVersion:      lbcKinds,
			},
			albEnabled:         false,
			nlbEnabled:         true,
			listenerSetEnabled: false,
		},
		{
			name: "tlsroute missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute", "UDPRoute"),
				LBCGatewayGroupVersion:      lbcKinds,
			},
			albEnabled:         true,
			nlbEnabled:         false,
			listenerSetEnabled: false,
		},
		{
			name: "tcproute missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("UDPRoute"),
				LBCGatewayGroupVersion:      lbcKinds,
			},
			albEnabled:         true,
			nlbEnabled:         false,
			listenerSetEnabled: false,
		},
		{
			name: "udproute missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:       sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"),
				GatewayV1Alpha2GroupVersion: sets.New[string]("TCPRoute"),
				LBCGatewayGroupVersion:      lbcKinds,
			},
			albEnabled:         true,
			nlbEnabled:         false,
			listenerSetEnabled: false,
		},
		{
			name: "tcproute and udproute only under v1 (gateway api 1.6 standard channel)",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:  sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute", "TCPRoute", "UDPRoute"),
				LBCGatewayGroupVersion: lbcKinds,
			},
			albEnabled:         true,
			nlbEnabled:         true,
			listenerSetEnabled: false,
		},
		{
			name: "ListenerSet present without full gateway CRDs",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion: sets.New[string]("ListenerSet"),
			},
			albEnabled:         false,
			nlbEnabled:         false,
			listenerSetEnabled: true,
		},
		{
			name: "LBC CRDs present but no standard gateway CRDs",
			presentKinds: map[string]sets.Set[string]{
				LBCGatewayGroupVersion: lbcKinds,
			},
			albEnabled:         false,
			nlbEnabled:         false,
			listenerSetEnabled: false,
		},
		{
			name: "partial LBC CRDs - TargetGroupConfiguration missing",
			presentKinds: map[string]sets.Set[string]{
				GatewayV1GroupVersion:  sets.New[string]("Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute"),
				LBCGatewayGroupVersion: sets.New[string]("LoadBalancerConfiguration", "ListenerRuleConfiguration"),
			},
			albEnabled:         false,
			nlbEnabled:         false,
			listenerSetEnabled: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.NewFeatureGates()
			applyGatewayFeatureFlags(tc.presentKinds, cfg, logr.Discard())

			assert.Equal(t, tc.albEnabled, cfg.Enabled(config.ALBGatewayAPI))
			assert.Equal(t, tc.nlbEnabled, cfg.Enabled(config.NLBGatewayAPI))
			assert.Equal(t, tc.listenerSetEnabled, cfg.Enabled(config.GatewayListenerSet))
		})
	}
}

func TestApplyGatewayCRDDetection(t *testing.T) {
	lbcKinds := []string{"TargetGroupConfiguration", "LoadBalancerConfiguration", "ListenerRuleConfiguration"}

	testCases := []struct {
		name               string
		resources          map[string][]string
		albEnabled         bool
		nlbEnabled         bool
		listenerSetEnabled bool
		expectedTCPVersion string
		expectedUDPVersion string
		explicitFlags      bool
	}{
		{
			name: "all CRDs present with alpha2 routes - resolves to alpha2",
			resources: map[string][]string{
				GatewayV1GroupVersion:       {"Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute"},
				GatewayV1Alpha2GroupVersion: {"TCPRoute", "UDPRoute"},
				LBCGatewayGroupVersion:      lbcKinds,
			},
			albEnabled:         true,
			nlbEnabled:         true,
			listenerSetEnabled: false,
			expectedTCPVersion: GatewayV1Alpha2GroupVersion,
			expectedUDPVersion: GatewayV1Alpha2GroupVersion,
			explicitFlags:      false,
		},
		{
			name: "TCPRoute and UDPRoute only under v1 (gateway api 1.6 standard channel) - NLB not disabled",
			resources: map[string][]string{
				GatewayV1GroupVersion:  {"Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "TLSRoute", "TCPRoute", "UDPRoute"},
				LBCGatewayGroupVersion: lbcKinds,
			},
			albEnabled:         true,
			nlbEnabled:         true,
			listenerSetEnabled: false,
			expectedTCPVersion: GatewayV1GroupVersion,
			expectedUDPVersion: GatewayV1GroupVersion,
			explicitFlags:      false,
		},
		{
			name: "discovery still runs when all flags explicitly set",
			resources: map[string][]string{
				GatewayV1GroupVersion: {"TCPRoute", "UDPRoute"},
			},
			// We test that routeVersions is returned even when all flags are explicit.
			// Feature flags are not touched in that case.
			albEnabled:         true, // explicitly set before call
			nlbEnabled:         true,
			listenerSetEnabled: false,
			expectedTCPVersion: GatewayV1GroupVersion,
			expectedUDPVersion: GatewayV1GroupVersion,
			explicitFlags:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeDiscoveryClient{resources: tc.resources}
			cfg := config.NewFeatureGates()

			// For test cases with explicitly set flags, mark all flags as non-defaulted.
			if tc.explicitFlags {
				cfg.Enable(config.ALBGatewayAPI)
				cfg.Enable(config.NLBGatewayAPI)
				cfg.Enable(config.GatewayListenerSet)
			}

			routeVersions, err := ApplyGatewayCRDDetection(client, cfg, logr.Discard())
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedTCPVersion, routeVersions.TCPRouteGroupVersion)
			assert.Equal(t, tc.expectedUDPVersion, routeVersions.UDPRouteGroupVersion)

			if tc.explicitFlags {
				// Assert that explicitly-set flags retain their values (applyGatewayFeatureFlags did not run).
				assert.True(t, cfg.Enabled(config.ALBGatewayAPI))
				assert.True(t, cfg.Enabled(config.NLBGatewayAPI))
				assert.True(t, cfg.Enabled(config.GatewayListenerSet))
			} else {
				// Assert that flags match the expected values from discovery.
				assert.Equal(t, tc.albEnabled, cfg.Enabled(config.ALBGatewayAPI))
				assert.Equal(t, tc.nlbEnabled, cfg.Enabled(config.NLBGatewayAPI))
				assert.Equal(t, tc.listenerSetEnabled, cfg.Enabled(config.GatewayListenerSet))
			}
		})
	}
}
