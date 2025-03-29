package routeutils

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// routeAttachmentHelper is an internal utility that is responsible for providing functionality related to route filtering.
type routeAttachmentHelper interface {
	doesRouteAttachToGateway(gw gwv1.Gateway, route preLoadRouteDescriptor) bool
	routeAllowsAttachmentToListener(listener gwv1.Listener, route preLoadRouteDescriptor) bool
}

var _ routeAttachmentHelper = &routeAttachmentHelperImpl{}

type routeAttachmentHelperImpl struct {
}

// doesRouteAttachToGateway is responsible for determining if a route and gateway should be connected.
// This function implements the Gateway API spec for determining Gateway -> Route attachment.
func (rah *routeAttachmentHelperImpl) doesRouteAttachToGateway(gw gwv1.Gateway, route preLoadRouteDescriptor) bool {
	for _, parentRef := range route.GetParentRefs() {

		// Default for kind is Gateway.
		if parentRef.Kind != nil && *parentRef.Kind != "Gateway" {
			continue
		}

		var namespaceToCompare string

		if parentRef.Namespace != nil {
			namespaceToCompare = string(*parentRef.Namespace)
		} else {
			namespaceToCompare = gw.Namespace
		}

		if string(parentRef.Name) == gw.Name && gw.Namespace == namespaceToCompare {
			return true
		}
	}

	return false
}

// routeAllowsAttachmentToListener is responsible for determining if a route and listener should be connected. This function is slightly different than
// listenerAttachmentHelper as it handles listener -> route relationships. This utility handles route -> listener relationships.
// In order for a relationship to be established, both listener and route must agree to the connection.
// This function implements the Gateway API spec for route -> listener attachment.
// This function assumes that the caller has already validated that the gateway that owns the listener allows for route
// attachment.
func (rah *routeAttachmentHelperImpl) routeAllowsAttachmentToListener(listener gwv1.Listener, route preLoadRouteDescriptor) bool {
	for _, parentRef := range route.GetParentRefs() {

		if parentRef.SectionName != nil && string(*parentRef.SectionName) != string(listener.Name) {
			continue
		}

		if parentRef.Port != nil && *parentRef.Port != listener.Port {
			continue
		}

		return true
	}

	return false
}
