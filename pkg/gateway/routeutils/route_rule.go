package routeutils

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// RouteRule is a type agnostic representation of Routing Rules.
type RouteRule interface {
	GetRawRouteRule() interface{}
	GetSectionName() *gwv1.SectionName
	GetBackends() []Backend
}
