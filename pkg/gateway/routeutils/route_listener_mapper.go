package routeutils

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// listenerToRouteMapper is an internal utility that will map a list of routes to the listeners of a gateway
// if the gateway and/or route are incompatible, then the route is discarded.
type listenerToRouteMapper interface {
	mapGatewayAndRoutes(context context.Context, gw gwv1.Gateway, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, map[int32]map[string][]gwv1.Hostname, []RouteData, error)
}

var _ listenerToRouteMapper = &listenerToRouteMapperImpl{}

type listenerToRouteMapperImpl struct {
	listenerAttachmentHelper listenerAttachmentHelper
	routeAttachmentHelper    routeAttachmentHelper
	logger                   logr.Logger
}

func newListenerToRouteMapper(k8sClient client.Client, logger logr.Logger) listenerToRouteMapper {
	return &listenerToRouteMapperImpl{
		listenerAttachmentHelper: newListenerAttachmentHelper(k8sClient, logger.WithName("listener-attachment-helper")),
		routeAttachmentHelper:    newRouteAttachmentHelper(logger.WithName("route-attachment-helper")),
		logger:                   logger,
	}
}

// mapGatewayAndRoutes will map route to the corresponding listener ports using the Gateway API spec rules.
// Returns: (routesByPort, compatibleHostnamesByPort, failedRoutes, error)
func (ltr *listenerToRouteMapperImpl) mapGatewayAndRoutes(ctx context.Context, gw gwv1.Gateway, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, map[int32]map[string][]gwv1.Hostname, []RouteData, error) {
	result := make(map[int][]preLoadRouteDescriptor)
	compatibleHostnamesByPort := make(map[int32]map[string][]gwv1.Hostname)
	failedRoutes := make([]RouteData, 0)

	// First filter out any routes that are not intended for this Gateway.
	routesForGateway := make([]preLoadRouteDescriptor, 0)
	for _, route := range routes {
		allowsAttachment := ltr.routeAttachmentHelper.doesRouteAttachToGateway(gw, route)
		ltr.logger.V(1).Info("Route is eligible for attachment", "route", route.GetRouteNamespacedName(), "allowed attachment", allowsAttachment)
		if allowsAttachment {
			routesForGateway = append(routesForGateway, route)
		}
	}

	// Dedupe - Check if route already exists for this port before adding
	seenRoutesPerPort := make(map[int]map[string]bool)
	// Next, greedily looking for the route to attach to.
	for _, listener := range gw.Spec.Listeners {
		// used for cross serving check
		hostnamesFromHttpRoutes := make(map[types.NamespacedName][]gwv1.Hostname)
		hostnamesFromGrpcRoutes := make(map[types.NamespacedName][]gwv1.Hostname)
		for _, route := range routesForGateway {
			// We need to check both paths (route -> listener) and (listener -> route)
			// for connection viability.
			if !ltr.routeAttachmentHelper.routeAllowsAttachmentToListener(listener, route) {
				ltr.logger.V(1).Info("Route doesnt allow attachment")
				continue
			}

			compatibleHostnames, allowedAttachment, failedRouteData, err := ltr.listenerAttachmentHelper.listenerAllowsAttachment(ctx, gw, listener, route, hostnamesFromHttpRoutes, hostnamesFromGrpcRoutes)
			if err != nil {
				return nil, nil, failedRoutes, err
			}

			if failedRouteData != nil {
				failedRoutes = append(failedRoutes, *failedRouteData)
			}

			ltr.logger.V(1).Info("listener allows attachment", "route", route.GetRouteNamespacedName(), "allowedAttachment", allowedAttachment)

			if allowedAttachment {
				port := int32(listener.Port)
				routeKey := fmt.Sprintf("%s-%s", route.GetRouteKind(), route.GetRouteNamespacedName())
				if seenRoutesPerPort[int(port)] == nil {
					seenRoutesPerPort[int(port)] = make(map[string]bool)
				}
				if !seenRoutesPerPort[int(port)][routeKey] {
					seenRoutesPerPort[int(port)][routeKey] = true
					result[int(port)] = append(result[int(port)], route)
				}

				// Store compatible hostnames per port per route per kind
				if compatibleHostnamesByPort[port] == nil {
					compatibleHostnamesByPort[port] = make(map[string][]gwv1.Hostname)
				}
				// Append hostnames for routes that attach to multiple listeners on the same port
				compatibleHostnamesByPort[port][routeKey] = append(compatibleHostnamesByPort[port][routeKey], compatibleHostnames...)
			}

		}
	}
	return result, compatibleHostnamesByPort, failedRoutes, nil
}
