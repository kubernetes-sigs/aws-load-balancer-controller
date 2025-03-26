package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type LoadRouteFilter interface {
	IsApplicable(kind string) bool
}

type routeFilterImpl struct {
	acceptedKinds sets.Set[string]
}

func (r *routeFilterImpl) IsApplicable(kind string) bool {
	return r.acceptedKinds.Has(kind)
}

var L4RouteFilter LoadRouteFilter = &routeFilterImpl{
	acceptedKinds: sets.New(UDPRouteKind, TCPRouteKind, TLSRouteKind),
}

var L7RouteFilter LoadRouteFilter = &routeFilterImpl{
	acceptedKinds: sets.New(HTTPRouteKind, GRPCRouteKind),
}

type Loader interface {
	LoadRoutesForGateway(ctx context.Context, client client.Client, gw *gwv1.Gateway, filter LoadRouteFilter) (map[int][]RouteDescriptor, error)
}

var _ Loader = &loaderImpl{}

type loaderImpl struct {
	mapper ListenerToRouteMapper
}

func (l *loaderImpl) LoadRoutesForGateway(ctx context.Context, k8sclient client.Client, gw *gwv1.Gateway, filter LoadRouteFilter) (map[int][]RouteDescriptor, error) {
	// 1. Load all relevant routes according to the filter
	loadedRoutes := make([]RouteDescriptor, 0)
	for route, loader := range allRoutes {
		if filter.IsApplicable(route) {
			data, err := loader(ctx, k8sclient)
			if err != nil {
				return nil, err
			}
			loadedRoutes = append(loadedRoutes, data...)
		}
	}

	// 2. Remove routes that aren't granted attachment by the listener.
	// Map any routes that are granted attachment to the listener port that allows the attachment.
	return l.mapper.Map(ctx, k8sclient, gw, loadedRoutes)
}
