package routeutils

type RouteDescriptor interface {
	GetRouteNamespace() string
	GetRouteName() string
	GetRouteKind() string
	GetAttachedRules() []BackendDescription
	GetRawRoute() interface{}
}
