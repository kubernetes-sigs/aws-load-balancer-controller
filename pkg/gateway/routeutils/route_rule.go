package routeutils

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type RouteRule interface {
	GetRawRouteRule() interface{}
	GetSectionName() *gwv1.SectionName
	GetBackends() []Backend
}
