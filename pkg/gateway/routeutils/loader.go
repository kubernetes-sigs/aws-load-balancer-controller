package routeutils

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// LoadRouteFilter is an interface that consumers can use to tell the loader which routes to load.
type LoadRouteFilter interface {
	IsApplicable(kind RouteKind) bool
}

// routeFilterImpl implements LoadRouteFilter
type routeFilterImpl struct {
	acceptedKinds sets.Set[RouteKind]
}

func (r *routeFilterImpl) IsApplicable(kind RouteKind) bool {
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
	LoadRoutesForGateway(ctx context.Context, gw gwv1.Gateway, filter LoadRouteFilter, controllerName string) (*LoaderResult, error)
}

type LoaderResult struct {
	Routes            map[int32][]RouteDescriptor
	AttachedRoutesMap map[gwv1.SectionName]int32
	ValidationResults ListenerValidationResults
}

var _ Loader = &loaderImpl{}

type loaderImpl struct {
	mapper          listenerToRouteMapper
	routeSubmitter  RouteReconcilerSubmitter
	k8sClient       client.Client
	logger          logr.Logger
	allRouteLoaders map[RouteKind]func(context context.Context, client client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error)
}

func NewLoader(k8sClient client.Client, routeSubmitter RouteReconcilerSubmitter, logger logr.Logger) Loader {
	return &loaderImpl{
		mapper:          newListenerToRouteMapper(k8sClient, logger.WithName("route-mapper")),
		routeSubmitter:  routeSubmitter,
		k8sClient:       k8sClient,
		allRouteLoaders: allRoutes,
		logger:          logger,
	}
}

// LoadRoutesForGateway loads all relevant data for a single Gateway.
func (l *loaderImpl) LoadRoutesForGateway(ctx context.Context, gw gwv1.Gateway, filter LoadRouteFilter, controllerName string) (*LoaderResult, error) {
	// 1. Load all relevant routes according to the filter

	loadedRoutes := make([]preLoadRouteDescriptor, 0)

	routeStatusUpdates := make([]RouteData, 0)

	defer func() {
		seenCache := sets.NewString()
		// As we process the failures first, we ensure that we don't flip flop the route status from
		// failed -> ok.
		for _, v := range routeStatusUpdates {
			k := generateRouteDataCacheKey(v)
			if seenCache.Has(k) {
				continue
			}
			seenCache.Insert(k)
			l.routeSubmitter.Enqueue(v)
		}
	}()

	for route, loader := range l.allRouteLoaders {
		applicable := filter.IsApplicable(route)
		l.logger.V(1).Info("Processing route", "route", route, "is applicable", applicable)
		if applicable {
			data, err := loader(ctx, l.k8sClient)
			if err != nil {
				return nil, err
			}
			loadedRoutes = append(loadedRoutes, data...)
		}
	}

	// validate listeners configuration and get listener status
	listenerValidationResults := ValidateListeners(gw, controllerName, ctx, l.k8sClient)

	// 2. Remove routes that aren't granted attachment by the listener.
	// Map any routes that are granted attachment to the listener port that allows the attachment.
	mappedRoutes, compatibleHostnamesByPort, statusUpdates, matchedParentRefs, err := l.mapper.mapGatewayAndRoutes(ctx, gw, loadedRoutes)

	routeStatusUpdates = append(routeStatusUpdates, statusUpdates...)

	if err != nil {
		return nil, err
	}

	// Count attached routes per listener for listener status update
	attachedRouteMap := buildAttachedRouteMap(gw, mappedRoutes)

	// 3. Load the underlying resource(s) for each route that is configured.
	loadedRoute, childRouteLoadUpdates, err := l.loadChildResources(ctx, mappedRoutes, compatibleHostnamesByPort, gw, matchedParentRefs)
	routeStatusUpdates = append(routeStatusUpdates, childRouteLoadUpdates...)
	if err != nil {
		return nil, err
	}

	// 4. update status for accepted routes - generate per matched parentRef
	for _, routeList := range loadedRoute {
		for _, route := range routeList {
			routeKey := route.GetRouteIdentifier()
			if matchedRefs, ok := matchedParentRefs[routeKey]; ok {
				for _, parentRef := range matchedRefs {
					routeStatusUpdates = append(routeStatusUpdates, GenerateRouteData(true, true, string(gwv1.RouteConditionAccepted), RouteStatusInfoAcceptedMessage, route.GetRouteNamespacedName(), route.GetRouteKind(), route.GetRouteGeneration(), gw, parentRef.Port, parentRef.SectionName))
				}
			}
		}
	}

	return &LoaderResult{
		Routes:            loadedRoute,
		AttachedRoutesMap: attachedRouteMap,
		ValidationResults: listenerValidationResults,
	}, nil
}

// loadChildResources responsible for loading all resources that a route descriptor references.
func (l *loaderImpl) loadChildResources(ctx context.Context, preloadedRoutes map[int][]preLoadRouteDescriptor, compatibleHostnamesByPort map[int32]map[string][]gwv1.Hostname, gw gwv1.Gateway, matchedParentRefs map[string][]gwv1.ParentReference) (map[int32][]RouteDescriptor, []RouteData, error) {
	// Cache to reduce duplicate route lookups.
	// Kind -> [NamespacedName:Previously Loaded Descriptor]
	resourceCache := make(map[string]RouteDescriptor)
	loadedRouteData := make(map[int32][]RouteDescriptor)
	failedRoutes := make([]RouteData, 0)

	for port, preloadedRouteList := range preloadedRoutes {
		for _, preloadedRoute := range preloadedRouteList {
			namespacedNameRoute := preloadedRoute.GetRouteNamespacedName()
			routeKind := preloadedRoute.GetRouteKind()
			cacheKey := fmt.Sprintf("%s-%s-%s", routeKind, namespacedNameRoute.Name, namespacedNameRoute.Namespace)

			cachedRoute, ok := resourceCache[cacheKey]
			if ok {
				loadedRouteData[int32(port)] = append(loadedRouteData[int32(port)], cachedRoute)
				continue
			}

			generatedRoute, loadAttachedRulesErrors := preloadedRoute.loadAttachedRules(ctx, l.k8sClient)
			if len(loadAttachedRulesErrors) > 0 {
				for _, lare := range loadAttachedRulesErrors {
					var loaderErr LoaderError
					if errors.As(lare.Err, &loaderErr) {
						routeReason := loaderErr.GetRouteReason()
						// Categorize reasons into Accepted vs ResolvedRefs conditions
						var accepted, resolvedRefs bool
						switch routeReason {
						case gwv1.RouteReasonNotAllowedByListeners,
							gwv1.RouteReasonNoMatchingListenerHostname,
							gwv1.RouteReasonNoMatchingParent,
							gwv1.RouteReasonUnsupportedValue,
							gwv1.RouteReasonPending,
							gwv1.RouteReasonIncompatibleFilters:
							// These affect Accepted condition
							accepted = false
							resolvedRefs = true
						case gwv1.RouteReasonRefNotPermitted,
							gwv1.RouteReasonInvalidKind,
							gwv1.RouteReasonBackendNotFound,
							gwv1.RouteReasonUnsupportedProtocol:
							// These affect ResolvedRefs condition
							accepted = true
							resolvedRefs = false
						default:
							// Unknown reason, fail both
							accepted = false
							resolvedRefs = false
						}
						// Generate error status for each matched parentRef
						routeKey := preloadedRoute.GetRouteIdentifier()
						parentRefs := matchedParentRefs[routeKey]
						for _, parentRef := range parentRefs {
							failedRoutes = append(failedRoutes, GenerateRouteData(accepted, resolvedRefs, string(routeReason), loaderErr.GetRouteMessage(), preloadedRoute.GetRouteNamespacedName(), preloadedRoute.GetRouteKind(), preloadedRoute.GetRouteGeneration(), gw, parentRef.Port, parentRef.SectionName))
						}

					}
					if lare.Fatal {
						return nil, failedRoutes, lare.Err
					}
				}
			}

			loadedRouteData[int32(port)] = append(loadedRouteData[int32(port)], generatedRoute)
			resourceCache[cacheKey] = generatedRoute
		}
	}

	// Set compatible hostnames by port for all routes
	for _, route := range resourceCache {
		hostnamesByPort := make(map[int32][]gwv1.Hostname)
		routeKey := route.GetRouteIdentifier()
		for port, compatibleHostnames := range compatibleHostnamesByPort {
			if hostnames, exists := compatibleHostnames[routeKey]; exists {
				hostnamesByPort[port] = hostnames
			}
		}
		if len(hostnamesByPort) > 0 {
			route.setCompatibleHostnamesByPort(hostnamesByPort)
		}
	}

	return loadedRouteData, failedRoutes, nil
}

func generateRouteDataCacheKey(rd RouteData) string {
	port := ""
	if rd.ParentRef.Port != nil {
		port = fmt.Sprintf("%d", *rd.ParentRef.Port)
	}
	sectionName := ""
	if rd.ParentRef.SectionName != nil {
		sectionName = string(*rd.ParentRef.SectionName)
	}
	namespace := ""
	if rd.ParentRef.Namespace != nil {
		namespace = string(*rd.ParentRef.Namespace)
	}

	parentKind := ""
	if rd.ParentRef.Kind != nil {
		parentKind = string(*rd.ParentRef.Kind)
	}

	return fmt.Sprintf("%s-%s-%s-%s-%s-%s-%s-%s", rd.RouteMetadata.RouteName, rd.RouteMetadata.RouteNamespace, rd.RouteMetadata.RouteKind, rd.ParentRef.Name, namespace, port, sectionName, parentKind)
}

func buildAttachedRouteMap(gw gwv1.Gateway, mappedRoutes map[int][]preLoadRouteDescriptor) map[gwv1.SectionName]int32 {
	// Discover listeners once
	discoveredListeners := DiscoverListeners(&gw)

	attachedRouteMap := make(map[gwv1.SectionName]int32)
	for _, dl := range discoveredListeners.All {
		attachedRouteMap[dl.Name] = 0
	}

	for port, routeList := range mappedRoutes {
		if name, exists := discoveredListeners.GetNameByPort(int32(port)); exists {
			attachedRouteMap[name] = int32(len(routeList))
		}
	}
	return attachedRouteMap
}
