package routeutils

import (
	"context"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ListenerToRouteMapper interface {
	Map(context context.Context, gw gwv1.Gateway, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, error)
}

var _ ListenerToRouteMapper = &listenerToRouteMapperImpl{}

type listenerToRouteMapperImpl struct {
	listenerAttachmentHelper listenerAttachmentHelper
	routeAttachmentHelper    routeAttachmentHelper
}

func (ltr *listenerToRouteMapperImpl) Map(ctx context.Context, gw gwv1.Gateway, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, error) {
	result := make(map[int][]preLoadRouteDescriptor)

	routesForGateway := make([]preLoadRouteDescriptor, 0)
	for _, route := range routes {
		if ltr.routeAttachmentHelper.doesRouteAttachToGateway(gw, route) {
			routesForGateway = append(routesForGateway, route)
		}
	}

	// Approach is to greedily add as many relevant routes to each listener.
	for _, listener := range gw.Spec.Listeners {
		for _, route := range routesForGateway {

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
