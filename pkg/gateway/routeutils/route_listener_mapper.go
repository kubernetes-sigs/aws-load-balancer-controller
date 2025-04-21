package routeutils

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// listenerToRouteMapper is an internal utility that will map a list of routes to the listeners of a gateway
// if the gateway and/or route are incompatible, then route is discarded.
type listenerToRouteMapper interface {
	mapGatewayAndRoutes(context context.Context, gw gwv1.Gateway, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, error)
}

var _ listenerToRouteMapper = &listenerToRouteMapperImpl{}

type listenerToRouteMapperImpl struct {
	listenerAttachmentHelper listenerAttachmentHelper
	routeAttachmentHelper    routeAttachmentHelper
}

func newListenerToRouteMapper(k8sClient client.Client) listenerToRouteMapper {
	return &listenerToRouteMapperImpl{
		listenerAttachmentHelper: newListenerAttachmentHelper(k8sClient),
		routeAttachmentHelper:    newRouteAttachmentHelper(),
	}
}

// mapGatewayAndRoutes will map route to the corresponding listener ports using the Gateway API spec rules.
func (ltr *listenerToRouteMapperImpl) mapGatewayAndRoutes(ctx context.Context, gw gwv1.Gateway, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, error) {
	result := make(map[int][]preLoadRouteDescriptor)

	// First filter out any routes that are not intended for this Gateway.
	routesForGateway := make([]preLoadRouteDescriptor, 0)
	for _, route := range routes {
		if ltr.routeAttachmentHelper.doesRouteAttachToGateway(gw, route) {
			routesForGateway = append(routesForGateway, route)
		}
	}

	// Next, greedily looking for the route to attach to.
	for _, listener := range gw.Spec.Listeners {
		for _, route := range routesForGateway {

			// We need to check both paths (route -> listener) and (listener -> route)
			// for connection viability.
			if !ltr.routeAttachmentHelper.routeAllowsAttachmentToListener(listener, route) {
				continue
			}

			allowedAttachment, err := ltr.listenerAttachmentHelper.listenerAllowsAttachment(ctx, gw, listener, route)
			if err != nil {
				return nil, err
			}

			if allowedAttachment {
				result[int(listener.Port)] = append(result[int(listener.Port)], route)
			}
		}
	}
	return result, nil
}
