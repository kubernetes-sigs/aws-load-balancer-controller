package routeutils

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ListL4Routes retrieves all Layer 4 routes (TCP, UDP, TLS) from the cluster.
func ListL4Routes(ctx context.Context, k8sClient client.Client) ([]preLoadRouteDescriptor, error) {
	l4Routes := make([]preLoadRouteDescriptor, 0)
	var routekinds []RouteKind
	tcpRoutes, err := ListTCPRoutes(ctx, k8sClient)
	if err != nil {
		routekinds = append(routekinds, TCPRouteKind)
	}
	l4Routes = append(l4Routes, tcpRoutes...)
	udpRoutes, err := ListUDPRoutes(ctx, k8sClient)
	if err != nil {
		routekinds = append(routekinds, UDPRouteKind)
	}
	l4Routes = append(l4Routes, udpRoutes...)
	tlsRoutes, err := ListTLSRoutes(ctx, k8sClient)
	if err != nil {
		routekinds = append(routekinds, TLSRouteKind)
	}
	l4Routes = append(l4Routes, tlsRoutes...)
	if len(routekinds) > 0 {
		err = fmt.Errorf("failed to list L4 routes, %s", routekinds)
	}
	return l4Routes, err
}

// ListL7Routes retrieves all Layer 7 routes (HTTP, gRPC) from the cluster.
func ListL7Routes(ctx context.Context, k8sClient client.Client) ([]preLoadRouteDescriptor, error) {
	l7Routes := make([]preLoadRouteDescriptor, 0)
	var routekinds []RouteKind
	httpRoutes, err := ListHTTPRoutes(ctx, k8sClient)
	if err != nil {
		routekinds = append(routekinds, HTTPRouteKind)
	}
	l7Routes = append(l7Routes, httpRoutes...)
	grpcRoutes, err := ListGRPCRoutes(ctx, k8sClient)
	if err != nil {
		routekinds = append(routekinds, GRPCRouteKind)
	}
	l7Routes = append(l7Routes, grpcRoutes...)
	if len(routekinds) > 0 {
		err = fmt.Errorf("failed to list L7 routes, %s", routekinds)
	}
	return l7Routes, err
}

// FilterRoutesBySvc filters a slice of routes based on service reference.
// Returns a new slice containing only routes that reference the specified service.
func FilterRoutesBySvc(routes []preLoadRouteDescriptor, svc *corev1.Service) []preLoadRouteDescriptor {
	if svc == nil || len(routes) == 0 {
		return []preLoadRouteDescriptor{}
	}
	filteredRoutes := make([]preLoadRouteDescriptor, 0, len(routes))
	svcID := types.NamespacedName{
		Namespace: svc.Namespace,
		Name:      svc.Name,
	}
	for _, route := range routes {
		if isServiceReferredByRoute(route, svcID) {
			filteredRoutes = append(filteredRoutes, route)
		}
	}
	return filteredRoutes
}

// isServiceReferredByRoute checks if a route references a specific service.
// Assuming we are only supporting services as backendRefs on Routes
func isServiceReferredByRoute(route preLoadRouteDescriptor, svcID types.NamespacedName) bool {
	for _, backendRef := range route.GetBackendRefs() {
		namespace := route.GetRouteNamespacedName().Namespace
		if backendRef.Namespace != nil {
			namespace = string(*backendRef.Namespace)
		}

		if string(backendRef.Name) == svcID.Name && namespace == svcID.Namespace {
			return true
		}
	}
	return false
}
