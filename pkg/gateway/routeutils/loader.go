package routeutils

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// LoadRouteFilter is an interface that consumers can use to tell the loader which routes to load.
type LoadRouteFilter interface {
	IsApplicable(kind string) bool
}

// routeFilterImpl implements LoadRouteFilter
type routeFilterImpl struct {
	acceptedKinds sets.Set[string]
}

func (r *routeFilterImpl) IsApplicable(kind string) bool {
	return r.acceptedKinds.Has(kind)
}

/*

TLS mappings -- Should we enforce that here?

Listener Protocol |	TLS Mode 	 | Route Type Supported
TLS 	          | Passthrough  | TLSRoute
TLS 	          | Terminate 	 | TCPRoute
HTTPS 	          | Terminate 	 | HTTPRoute
GRPC 	          | Terminate 	 | GRPCRoute
*/

// L4RouteFilter use this to load routes only pertaining to the L4 Gateway Implementation (AWS NLB)
var L4RouteFilter LoadRouteFilter = &routeFilterImpl{
	acceptedKinds: sets.New(UDPRouteKind, TCPRouteKind, TLSRouteKind),
}

// L7RouteFilter use this to load routes only pertaining to the L7 Gateway Implementation (AWS ALB)
var L7RouteFilter LoadRouteFilter = &routeFilterImpl{
	acceptedKinds: sets.New(HTTPRouteKind, GRPCRouteKind),
}

// Loader will load all data Kubernetes that are pertinent to a gateway (Routes, Services, Target Group Configurations).
// It will output the data using a map which maps listener port to the various routing rules for that port.
type Loader interface {
	LoadRoutesForGateway(ctx context.Context, gw gwv1.Gateway, filter LoadRouteFilter) (map[int][]RouteDescriptor, error)
}

var _ Loader = &loaderImpl{}

type loaderImpl struct {
	mapper          listenerToRouteMapper
	k8sClient       client.Client
	allRouteLoaders map[string]func(context context.Context, client client.Client) ([]preLoadRouteDescriptor, error)
}

func NewLoader(k8sClient client.Client) Loader {
	return &loaderImpl{
		mapper:          newListenerToRouteMapper(k8sClient),
		k8sClient:       k8sClient,
		allRouteLoaders: allRoutes,
	}
}

// LoadRoutesForGateway loads all relevant data for a single Gateway.
func (l *loaderImpl) LoadRoutesForGateway(ctx context.Context, gw gwv1.Gateway, filter LoadRouteFilter) (map[int][]RouteDescriptor, error) {
	// 1. Load all relevant routes according to the filter
	loadedRoutes := make([]preLoadRouteDescriptor, 0)
	for route, loader := range l.allRouteLoaders {
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
	mappedRoutes, err := l.mapper.mapGatewayAndRoutes(ctx, gw, loadedRoutes)
	if err != nil {
		return nil, err
	}

	// 3. Load the underlying resource(s) for each route that is configured.
	return l.loadChildResources(ctx, mappedRoutes)
}

// loadChildResources responsible for loading all resources that a route descriptor references.
func (l *loaderImpl) loadChildResources(ctx context.Context, preloadedRoutes map[int][]preLoadRouteDescriptor) (map[int][]RouteDescriptor, error) {
	// Cache to reduce duplicate route look ups.
	// Kind -> [NamespacedName:Previously Loaded Descriptor]
	resourceCache := make(map[string]RouteDescriptor)

	loadedRouteData := make(map[int][]RouteDescriptor)

	for port, preloadedRouteList := range preloadedRoutes {
		for _, preloadedRoute := range preloadedRouteList {
			namespacedNameRoute := preloadedRoute.GetRouteNamespacedName()
			routeKind := preloadedRoute.GetRouteKind()
			cacheKey := fmt.Sprintf("%s-%s-%s", routeKind, namespacedNameRoute.Name, namespacedNameRoute.Namespace)

			cachedRoute, ok := resourceCache[cacheKey]
			if ok {
				loadedRouteData[port] = append(loadedRouteData[port], cachedRoute)
				continue
			}

			generatedRoute, err := preloadedRoute.loadAttachedRules(ctx, l.k8sClient)
			if err != nil {
				return nil, err
			}
			loadedRouteData[port] = append(loadedRouteData[port], generatedRoute)
			resourceCache[cacheKey] = generatedRoute
		}
	}

	return loadedRouteData, nil
}
