package routeutils

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
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
	LoadRoutesForGateway(ctx context.Context, gw gwv1.Gateway, filter LoadRouteFilter, controllerName string, defaultTGConfig *elbv2gw.TargetGroupConfiguration) (*LoaderResult, error)
}

type LoaderResult struct {
	Routes            map[int32][]RouteDescriptor
	Listeners         []gwv1.Listener
	AttachedRoutesMap map[gwv1.SectionName]int32
	ValidationResults ValidatedGatewayListeners
}

var _ Loader = &loaderImpl{}

type loaderImpl struct {
	mapper          listenerToRouteMapper
	lsLoader        listenerSetLoader
	routeSubmitter  RouteReconcilerSubmitter
	k8sClient       client.Client
	logger          logr.Logger
	allRouteLoaders map[RouteKind]func(context context.Context, client client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error)
}

func NewLoader(k8sClient client.Client, routeSubmitter RouteReconcilerSubmitter, logger logr.Logger) Loader {
	return &loaderImpl{
		mapper:          newListenerToRouteMapper(k8sClient, logger.WithName("route-mapper")),
		lsLoader:        newListenerSetLoader(k8sClient, logger.WithName("listener-set-loader")),
		routeSubmitter:  routeSubmitter,
		k8sClient:       k8sClient,
		allRouteLoaders: allRoutes,
		logger:          logger,
	}
}

// LoadRoutesForGateway loads all relevant data for a single Gateway.
func (l *loaderImpl) LoadRoutesForGateway(ctx context.Context, gw gwv1.Gateway, filter LoadRouteFilter, controllerName string, defaultTGConfig *elbv2gw.TargetGroupConfiguration) (*LoaderResult, error) {
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

	listenerSetListeners, rejectedListenerSets, err := l.lsLoader.retrieveListenersFromListenerSets(ctx, gw)

	if err != nil {
		return nil, err
	}

	if len(rejectedListenerSets) > 0 {
		// Submit these rejected listener sets to a status updater //
	}

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

	gatewayListeners := allListeners{
		GatewayListeners:     gw.Spec.Listeners,
		ListenerSetListeners: listenerSetListeners,
	}
	listenerValidationResults := validateListeners(gatewayListeners, controllerName)

	//  2. Map routes to relevant listeners
	mapResult, err := l.mapper.mapListenersAndRoutes(ctx, gw, gatewayListeners, loadedRoutes)
	if mapResult.failedRoutes != nil {
		routeStatusUpdates = append(routeStatusUpdates, mapResult.failedRoutes...)
	}
	if err != nil {
		return nil, err
	}

	// 3. Load the underlying resource(s) for each route that is configured.
	loadedRoute, childRouteLoadUpdates, err := l.loadChildResources(ctx, mapResult.routesByPort, mapResult.compatibleHostnamesByPort, gw, mapResult.matchedParentRefs, defaultTGConfig)
	routeStatusUpdates = append(routeStatusUpdates, childRouteLoadUpdates...)
	if err != nil {
		return nil, err
	}

	// 4. update status for accepted routes - generate per matched parentRef
	for _, routeList := range loadedRoute {
		for _, route := range routeList {
			routeKey := route.GetRouteIdentifier()
			if matchedRefs, ok := mapResult.matchedParentRefs[routeKey]; ok {
				for _, parentRef := range matchedRefs {
					routeStatusUpdates = append(routeStatusUpdates, GenerateRouteData(true, true, string(gwv1.RouteConditionAccepted), RouteStatusInfoAcceptedMessage, route.GetRouteNamespacedName(), route.GetRouteKind(), route.GetRouteGeneration(), parentRef))
				}
			}
		}
	}

	return &LoaderResult{
		Routes:            loadedRoute,
		Listeners:         gw.Spec.Listeners,
		AttachedRoutesMap: mapResult.routesPerListener,
		ValidationResults: listenerValidationResults,
	}, nil
}

// loadChildResources responsible for loading all resources that a route descriptor references.
func (l *loaderImpl) loadChildResources(ctx context.Context, preloadedRoutes map[int32][]preLoadRouteDescriptor, compatibleHostnamesByPort map[int32]map[string]sets.Set[gwv1.Hostname], gw gwv1.Gateway, matchedParentRefs map[string][]gwv1.ParentReference, gatewayDefaultTGConfig *elbv2gw.TargetGroupConfiguration) (map[int32][]RouteDescriptor, []RouteData, error) {
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
				loadedRouteData[port] = append(loadedRouteData[port], cachedRoute)
				continue
			}

			generatedRoute, loadAttachedRulesErrors := preloadedRoute.loadAttachedRules(ctx, l.k8sClient, gatewayDefaultTGConfig)
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
							failedRoutes = append(failedRoutes, GenerateRouteData(accepted, resolvedRefs, string(routeReason), loaderErr.GetRouteMessage(), preloadedRoute.GetRouteNamespacedName(), preloadedRoute.GetRouteKind(), preloadedRoute.GetRouteGeneration(), parentRef))
						}

					}
					if lare.Fatal {
						return nil, failedRoutes, lare.Err
					}
				}
			}

			loadedRouteData[port] = append(loadedRouteData[port], generatedRoute)
			resourceCache[cacheKey] = generatedRoute
		}
	}

	// Set compatible hostnames by port for all routes
	for _, route := range resourceCache {
		hostnamesByPort := make(map[int32][]gwv1.Hostname)
		routeKey := route.GetRouteIdentifier()
		for port, compatibleHostnames := range compatibleHostnamesByPort {
			if hostnames, exists := compatibleHostnames[routeKey]; exists {
				hostnamesByPort[port] = hostnames.UnsortedList()
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
	return fmt.Sprintf("%s-%s-%s-%s-%s-%s-%s", rd.RouteMetadata.RouteName, rd.RouteMetadata.RouteNamespace, rd.RouteMetadata.RouteKind, rd.ParentRef.Name, namespace, port, sectionName)
}
