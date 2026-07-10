package crddetect

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

// RouteVersions holds the Gateway API group version the controller uses for
// TCPRoute and UDPRoute watches, lists and status updates. TCPRoute and
// UDPRoute graduated to v1 in Gateway API 1.6; clusters running older CRDs
// only serve v1alpha2.
type RouteVersions struct {
	TCPRouteGroupVersion string
	UDPRouteGroupVersion string
}

// IsTCPRouteV1 returns true when the controller should use the v1 API for TCPRoute.
func (r RouteVersions) IsTCPRouteV1() bool {
	return r.TCPRouteGroupVersion == GatewayV1GroupVersion
}

// IsUDPRouteV1 returns true when the controller should use the v1 API for UDPRoute.
func (r RouteVersions) IsUDPRouteV1() bool {
	return r.UDPRouteGroupVersion == GatewayV1GroupVersion
}

// resolveRouteVersions picks the highest served group version per route kind,
// defaulting to v1alpha2 (the pre-1.6 behavior) when the kind isn't served anywhere.
func resolveRouteVersions(availableResources map[string]sets.Set[string]) RouteVersions {
	return RouteVersions{
		TCPRouteGroupVersion: resolveKindGroupVersion("TCPRoute", availableResources),
		UDPRouteGroupVersion: resolveKindGroupVersion("UDPRoute", availableResources),
	}
}

func resolveKindGroupVersion(kind string, availableResources map[string]sets.Set[string]) string {
	if kinds, ok := availableResources[GatewayV1GroupVersion]; ok && kinds.Has(kind) {
		return GatewayV1GroupVersion
	}
	return GatewayV1Alpha2GroupVersion
}
