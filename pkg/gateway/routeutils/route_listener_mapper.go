package routeutils

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// listenerToRouteMapper is an internal utility that will map a list of routes to the listeners of a gateway
// if the gateway and/or route are incompatible, then the route is discarded.
type listenerToRouteMapper interface {
	mapListenersAndRoutes(context context.Context, gw gwv1.Gateway, listeners allListeners, routes []preLoadRouteDescriptor) (map[int32][]preLoadRouteDescriptor, map[int32]map[string]sets.Set[gwv1.Hostname], []RouteData, map[string][]gwv1.ParentReference, map[gwv1.SectionName]int32, error)
}

var _ listenerToRouteMapper = &listenerToRouteMapperImpl{}

type listenerToRouteMapperImpl struct {
	listenerAttachmentHelper listenerAttachmentHelper
	logger                   logr.Logger
}

func newListenerToRouteMapper(k8sClient client.Client, logger logr.Logger) listenerToRouteMapper {
	return &listenerToRouteMapperImpl{
		listenerAttachmentHelper: newListenerAttachmentHelper(k8sClient, logger.WithName("listener-attachment-helper")),
		logger:                   logger,
	}
}

// mapListenersAndRoutes will map route to the corresponding listener ports using the Gateway API spec rules.
// Returns: (routesByPort, compatibleHostnamesByPort, failedRoutes, matchedParentRefs, error)
func (ltr *listenerToRouteMapperImpl) mapListenersAndRoutes(ctx context.Context, gw gwv1.Gateway, listeners allListeners, routes []preLoadRouteDescriptor) (map[int32][]preLoadRouteDescriptor, map[int32]map[string]sets.Set[gwv1.Hostname], []RouteData, map[string][]gwv1.ParentReference, map[gwv1.SectionName]int32, error) {
	//result := make(map[int][]preLoadRouteDescriptor)
	compatibleHostnamesByPort := make(map[int32]map[string]sets.Set[gwv1.Hostname])

	hostnamesFromHttpRoutes := make(map[int32]sets.Set[gwv1.Hostname])
	hostnamesFromGrpcRoutes := make(map[int32]sets.Set[gwv1.Hostname])

	initialListenerMapping := map[gwv1.SectionName][]routeParentRefTuple{}
	gatewayListenerSectionNameToListener := make(map[gwv1.SectionName]gwv1.Listener)

	for _, l := range listeners.GatewayListeners {
		initialListenerMapping[l.Name] = make([]routeParentRefTuple, 0)
		gatewayListenerSectionNameToListener[l.Name] = l
	}

	failedRoutes := make([]RouteData, 0)

	// route identifier -> all parent references
	matchedParentRefsResult := make(map[string][]gwv1.ParentReference)

	for _, route := range routes {
		for _, parentRef := range route.GetParentRefs() {
			var refTuple routeParentRefTuple
			if parentRef.Kind == nil || *parentRef.Kind == gatewayKind {
				refTuple = routeParentRefTuple{
					route:      route,
					parentRef:  parentRef,
					parent:     gw,
					parentKind: gatewayKind,
				}
				if doesResourceAttachToGateway(parentRef, route.GetRouteNamespacedName().Namespace, gw) {
					var anyAttach bool
					localFailedRoutes := make([]RouteData, 0)
					// Check to see if the listener(s) specified by the route parent ref will allow attachment
					if refTuple.parentRef.SectionName == nil {
						for _, listener := range listeners.GatewayListeners {
							if refTuple.parentRef.Port == nil || *refTuple.parentRef.Port == listener.Port {
								compatibleHostnames, failedRouteData, err := ltr.listenerAttachmentHelper.listenerAllowsAttachment(ctx, gw, listener, refTuple.route, &refTuple.parentRef, hostnamesFromHttpRoutes, hostnamesFromGrpcRoutes)
								if err != nil {
									return nil, nil, nil, nil, nil, err
								}

								if failedRouteData == nil {
									initialListenerMapping[listener.Name] = append(initialListenerMapping[listener.Name], refTuple)
									anyAttach = true
									// Store compatible hostnames per port per route per kind
									if compatibleHostnamesByPort[listener.Port] == nil {
										compatibleHostnamesByPort[listener.Port] = make(map[string]sets.Set[gwv1.Hostname])
									}

									if compatibleHostnamesByPort[listener.Port][refTuple.route.GetRouteIdentifier()] == nil {
										compatibleHostnamesByPort[listener.Port][refTuple.route.GetRouteIdentifier()] = make(sets.Set[gwv1.Hostname])
									}
									// Append hostnames for routes that attach to multiple listeners on the same port
									for _, ch := range compatibleHostnames {
										compatibleHostnamesByPort[listener.Port][refTuple.route.GetRouteIdentifier()].Insert(ch)
									}
								} else {
									localFailedRoutes = append(localFailedRoutes, *failedRouteData)
								}
							}
						}
					} else {
						targetListener, ok := gatewayListenerSectionNameToListener[*refTuple.parentRef.SectionName]
						if !ok {
							continue
						}
						listenerPort := targetListener.Port
						if refTuple.parentRef.Port == nil || *refTuple.parentRef.Port == listenerPort {
							compatibleHostnames, failedRouteData, err := ltr.listenerAttachmentHelper.listenerAllowsAttachment(ctx, gw, targetListener, refTuple.route, &refTuple.parentRef, hostnamesFromHttpRoutes, hostnamesFromGrpcRoutes)
							if err != nil {
								return nil, nil, nil, nil, nil, err
							}

							if failedRouteData == nil {
								initialListenerMapping[*refTuple.parentRef.SectionName] = append(initialListenerMapping[*refTuple.parentRef.SectionName], refTuple)
								anyAttach = true
								// Store compatible hostnames per port per route per kind
								if compatibleHostnamesByPort[listenerPort] == nil {
									compatibleHostnamesByPort[listenerPort] = make(map[string]sets.Set[gwv1.Hostname])
								}
								if compatibleHostnamesByPort[listenerPort][refTuple.route.GetRouteIdentifier()] == nil {
									compatibleHostnamesByPort[listenerPort][refTuple.route.GetRouteIdentifier()] = make(sets.Set[gwv1.Hostname])
								}
								// Append hostnames for routes that attach to multiple listeners on the same port
								for _, ch := range compatibleHostnames {
									compatibleHostnamesByPort[listenerPort][refTuple.route.GetRouteIdentifier()].Insert(ch)
								}
							} else {
								localFailedRoutes = append(localFailedRoutes, *failedRouteData)
							}
						}
					}

					if anyAttach {
						_, ok := matchedParentRefsResult[refTuple.route.GetRouteIdentifier()]
						if !ok {
							matchedParentRefsResult[refTuple.route.GetRouteIdentifier()] = make([]gwv1.ParentReference, 0)
						}
						matchedParentRefsResult[refTuple.route.GetRouteIdentifier()] = append(matchedParentRefsResult[refTuple.route.GetRouteIdentifier()], refTuple.parentRef)
					} else {
						// Attachment was attempted, but rejected by a listener policy (e.g. namespace, route kind limitation)
						if len(localFailedRoutes) > 0 {
							failedRoutes = append(failedRoutes, localFailedRoutes...)
						} else {
							// Attachment wasn't even attempted, probably because the parent ref had an invalid section name / port.
							failedRoutes = append(failedRoutes, GenerateRouteData(false, true, string(gwv1.RouteReasonNoMatchingParent), RouteStatusInfoRejectedMessageParentNotMatch, refTuple.route.GetRouteNamespacedName(), refTuple.route.GetRouteKind(), refTuple.route.GetRouteGeneration(), refTuple.parentRef))
						}
					}
				}
			} else if parentRef.Kind != nil && *parentRef.Kind == listenerSetKind {
				// listener set things //
			}
		}
	}

	routesPerListener := make(map[gwv1.SectionName]int32)
	for sn, snRoutes := range initialListenerMapping {
		routesPerListener[sn] = int32(len(snRoutes))
	}

	routesForListenerPorts := make(map[int32][]preLoadRouteDescriptor)
	seen := make(map[int32]sets.Set[string])

	for sectionName, listenerRoutes := range initialListenerMapping {
		targetPort := gatewayListenerSectionNameToListener[sectionName].Port
		_, ok := routesForListenerPorts[targetPort]
		if !ok {
			routesForListenerPorts[targetPort] = make([]preLoadRouteDescriptor, 0)
			seen[targetPort] = make(sets.Set[string])
		}
		for _, lr := range listenerRoutes {
			id := lr.route.GetRouteIdentifier()
			if seen[targetPort].Has(id) {
				continue
			}
			seen[targetPort].Insert(id)
			routesForListenerPorts[targetPort] = append(routesForListenerPorts[targetPort], lr.route)
		}
	}

	return routesForListenerPorts, compatibleHostnamesByPort, failedRoutes, matchedParentRefsResult, routesPerListener, nil
}

// getParentRefKey generates a unique key for a parentRef
func getParentRefKey(parentRef gwv1.ParentReference, parentNamespace string) string {
	var namespace string
	if parentRef.Namespace != nil {
		namespace = string(*parentRef.Namespace)
	} else {
		namespace = parentNamespace
	}

	sectionName := ""
	if parentRef.SectionName != nil {
		sectionName = string(*parentRef.SectionName)
	}

	port := ""
	if parentRef.Port != nil {
		port = fmt.Sprintf("%d", *parentRef.Port)
	}

	kind := gatewayKind
	if parentRef.Kind != nil {
		kind = string(*parentRef.Kind)
	}

	return fmt.Sprintf("%s/%s/%s/%s/%s", kind, namespace, string(parentRef.Name), sectionName, port)
}
