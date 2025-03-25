package routeutils

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Route Kinds
const (
	TCPRouteKind  = "TCPRoute"
	UDPRouteKind  = "UDPRoute"
	TLSRouteKind  = "TLSRoute"
	HTTPRouteKind = "HTTPRoute"
	GRPCRouteKind = "GRPCRoute"
)

// RouteKind to Route Loader. These functions will pull data directly from the kube api or local cache.
var allRoutes = map[string]func(context context.Context, client client.Client) ([]preLoadRouteDescriptor, error){
	TCPRouteKind:  ListTCPRoutes,
	UDPRouteKind:  ListUDPRoutes,
	TLSRouteKind:  ListTLSRoutes,
	HTTPRouteKind: ListHTTPRoutes,
	GRPCRouteKind: ListGRPCRoutes,
}

// Default protocol map used to infer accepted route kinds when a listener doesn't specify the `allowedRoutes` field.
var defaultProtocolToRouteKindMap = map[gwv1.ProtocolType]string{
	gwv1.TCPProtocolType:   TCPRouteKind,
	gwv1.UDPProtocolType:   UDPRouteKind,
	gwv1.TLSProtocolType:   TLSRouteKind,
	gwv1.HTTPProtocolType:  HTTPRouteKind,
	gwv1.HTTPSProtocolType: HTTPRouteKind,
}
