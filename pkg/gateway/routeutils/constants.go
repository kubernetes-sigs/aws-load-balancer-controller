package routeutils

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	TCPRouteKind  = "TCPRoute"
	UDPRouteKind  = "UDPRoute"
	TLSRouteKind  = "TLSRoute"
	HTTPRouteKind = "HTTPRoute"
	GRPCRouteKind = "GRPCRoute"
)

var allRoutes = map[string]func(context context.Context, client client.Client) ([]preLoadRouteDescriptor, error){
	TCPRouteKind:  ListTCPRoutes,
	UDPRouteKind:  ListUDPRoutes,
	TLSRouteKind:  ListTLSRoutes,
	HTTPRouteKind: ListHTTPRoutes,
	GRPCRouteKind: ListGRPCRoutes,
}

var defaultProtocolToRouteKindMap = map[gwv1.ProtocolType]string{
	gwv1.TCPProtocolType:   TCPRouteKind,
	gwv1.UDPProtocolType:   UDPRouteKind,
	gwv1.TLSProtocolType:   TLSRouteKind,
	gwv1.HTTPProtocolType:  HTTPRouteKind,
	gwv1.HTTPSProtocolType: HTTPRouteKind,
}
