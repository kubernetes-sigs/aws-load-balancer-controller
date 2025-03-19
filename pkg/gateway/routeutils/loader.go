package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
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

// L4RouteFilter use this to load routes only pertaining to the L4 Gateway Implementation (AWS NLB)
var L4RouteFilter LoadRouteFilter = &routeFilterImpl{
	acceptedKinds: sets.New(UDPRouteKind, TCPRouteKind, TLSRouteKind),
}

// L7RouteFilter use this to load routes only pertaining to the L7 Gateway Implementation (AWS ALB)
var L7RouteFilter LoadRouteFilter = &routeFilterImpl{
	acceptedKinds: sets.New(HTTPRouteKind, GRPCRouteKind),
}

type Loader interface {
	LoadRoutesForGateway(ctx context.Context, gw *gwv1.Gateway, filter LoadRouteFilter) (map[int][]RouteDescriptor, error)
}

var _ Loader = &loaderImpl{}

type loaderImpl struct {
	mapper    ListenerToRouteMapper
	k8sClient client.Client
}

func (l *loaderImpl) LoadRoutesForGateway(ctx context.Context, gw *gwv1.Gateway, filter LoadRouteFilter) (map[int][]RouteDescriptor, error) {
	// 1. Load all relevant routes according to the filter
	loadedRoutes := make([]preLoadRouteDescriptor, 0)
	for route, loader := range allRoutes {
		if filter.IsApplicable(route) {
			data, err := loader(ctx, l.k8sClient)
			if err != nil {
				return nil, err
			}
			loadedRoutes = append(loadedRoutes, data...)
		}
	}

	// 2. Remove routes that aren't granted attachment by the listener.
	// Map any routes that are granted attachment to the listener port that allows the attachment.
	mappedRoutes, err := l.mapper.Map(ctx, gw, loadedRoutes)
	if err != nil {
		return nil, err
	}

	// 3. Load the underlying resource(s) for each route that is configured.
	return l.loadChildResources(ctx, mappedRoutes)
}

func (l *loaderImpl) loadChildResources(ctx context.Context, preloadedRoutes map[int][]preLoadRouteDescriptor) (map[int][]RouteDescriptor, error) {
	// Cache to reduce duplicate route look ups.
	// Kind -> [NamespacedName:Previously Loaded Descriptor]
	resourceCache := make(map[string]map[types.NamespacedName]RouteDescriptor)

	loadedRouteData := make(map[int][]RouteDescriptor)

	for port, preloadedRouteList := range preloadedRoutes {
		for _, preloadedRoute := range preloadedRouteList {
			namespacedNameRoute := preloadedRoute.GetRouteNamespacedName()
			routeKind := preloadedRoute.GetRouteKind()

			kindSpecificCache, ok := resourceCache[routeKind]

			if !ok {
				resourceCache[routeKind] = make(map[types.NamespacedName]RouteDescriptor)
				kindSpecificCache = resourceCache[routeKind]
			}

			cachedRoute, ok := kindSpecificCache[namespacedNameRoute]
			if ok {
				loadedRouteData[port] = append(loadedRouteData[port], cachedRoute)
				continue
			}

			generatedRoute, err := preloadedRoute.loadAttachedRules(ctx, l.k8sClient)
			if err != nil {
				return nil, err
			}
			loadedRouteData[port] = append(loadedRouteData[port], generatedRoute)
			kindSpecificCache[namespacedNameRoute] = generatedRoute
		}
	}

	return loadedRouteData, nil
}
