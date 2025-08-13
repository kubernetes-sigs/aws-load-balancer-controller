package routeutils

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

// ListL4Routes retrieves all Layer 4 routes (TCP, UDP, TLS) from the cluster.
func ListL4Routes(ctx context.Context, k8sClient client.Client) ([]preLoadRouteDescriptor, error) {
	l4Routes := make([]preLoadRouteDescriptor, 0)
	var failedRoutes []RouteKind
	tcpRoutes, err := ListTCPRoutes(ctx, k8sClient)
	if err != nil {
		failedRoutes = append(failedRoutes, TCPRouteKind)
	}
	l4Routes = append(l4Routes, tcpRoutes...)
	udpRoutes, err := ListUDPRoutes(ctx, k8sClient)
	if err != nil {
		failedRoutes = append(failedRoutes, UDPRouteKind)
	}
	l4Routes = append(l4Routes, udpRoutes...)
	tlsRoutes, err := ListTLSRoutes(ctx, k8sClient)
	if err != nil {
		failedRoutes = append(failedRoutes, TLSRouteKind)
	}
	l4Routes = append(l4Routes, tlsRoutes...)
	if len(failedRoutes) > 0 {
		err = fmt.Errorf("failed to list L4 routes, %v", failedRoutes)
	}
	return l4Routes, err
}

// ListL7Routes retrieves all Layer 7 routes (HTTP, gRPC) from the cluster.
func ListL7Routes(ctx context.Context, k8sClient client.Client) ([]preLoadRouteDescriptor, error) {
	l7Routes := make([]preLoadRouteDescriptor, 0)
	var failedRoutes []RouteKind
	httpRoutes, err := ListHTTPRoutes(ctx, k8sClient)
	if err != nil {
		failedRoutes = append(failedRoutes, HTTPRouteKind)
	}
	l7Routes = append(l7Routes, httpRoutes...)
	grpcRoutes, err := ListGRPCRoutes(ctx, k8sClient)
	if err != nil {
		failedRoutes = append(failedRoutes, GRPCRouteKind)
	}
	l7Routes = append(l7Routes, grpcRoutes...)
	if len(failedRoutes) > 0 {
		err = fmt.Errorf("failed to list L7 routes, %v", failedRoutes)
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

// IsHostNameInValidFormat follows RFC1123 requirement except
// 1. no IP allowed
// 2. wildcard is only allowed as leftmost character
// Allowed Characters: Hostname labels must only contain lowercase ASCII letters (a-z), digits (0-9), and hyphens (-).
// Starting with a Digit: RFC 1123 allows labels to begin with a digit, which is a departure from the previous RFC 952 restriction.
// Length: Each label in a hostname can be between 1 and 63 characters long.
// Overall Hostname Length: The entire hostname, including the periods separating labels, cannot exceed 253 characters.
// Case: Hostnames are case-insensitive.
// Underscore: Underscores are not permitted in hostnames.
// Other Symbols: No other symbols, punctuation, or whitespace is allowed in hostnames
// Most of the requirements above is already checked by CRD pattern: Pattern=`^(\*\.)?[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
// Thus this function only checks for 1. if it is IP 2. label length is between 1 and 63
func IsHostNameInValidFormat(hostName string) (bool, error) {
	if net.ParseIP(hostName) != nil {

		return false, fmt.Errorf("hostname can not be IP address")
	}
	labels := strings.Split(hostName, ".")
	if strings.HasPrefix(hostName, "*.") {
		labels = labels[1:]
	}
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 {
			return false, fmt.Errorf("invalid hostname label length, length must between 1 and 63")
		}
	}
	return true, nil
}

// isHostnameCompatible checks if given two hostnames are compatible with each other
// this function is used to check if listener hostname and Route hostname match
func isHostnameCompatible(hostnameOne, hostnameTwo string) bool {
	// exact match
	if hostnameOne == hostnameTwo {
		return true
	}

	// suffix match - hostnameOne is a wildcard
	if strings.HasPrefix(hostnameOne, "*.") && strings.HasSuffix(hostnameTwo, hostnameOne[1:]) {
		return true
	}
	// suffix match - hostnameTwo is a wildcard
	if strings.HasPrefix(hostnameTwo, "*.") && strings.HasSuffix(hostnameOne, hostnameTwo[1:]) {
		return true
	}
	return false
}

func generateInvalidMessageWithRouteDetails(initialMessage string, routeKind RouteKind, routeIdentifier types.NamespacedName) string {
	return fmt.Sprintf("%s. Invalid data can be found in route (%s, %s:%s)", initialMessage, routeKind, routeIdentifier.Namespace, routeIdentifier.Name)
}
