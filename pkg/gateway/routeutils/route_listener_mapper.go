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
	mapListenersAndRoutes(context context.Context, gw gwv1.Gateway, listeners allListeners, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, map[int32]map[string][]gwv1.Hostname, []RouteData, map[string][]gwv1.ParentReference, map[gwv1.SectionName]int32, error)
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
func (ltr *listenerToRouteMapperImpl) mapListenersAndRoutes(ctx context.Context, gw gwv1.Gateway, listeners allListeners, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, map[int32]map[string][]gwv1.Hostname, []RouteData, map[string][]gwv1.ParentReference, map[gwv1.SectionName]int32, error) {
	mappedRoutes := listenersWithRoutes{
		GatewayListeners:                 map[gwv1.SectionName][]routeParentRefTuple{},
		GatewayListenerSectionNameToPort: map[gwv1.SectionName]int32{},
		ListenerSetListeners:             map[types.NamespacedName]map[gwv1.SectionName][]routeParentRefTuple{},
	}

	for _, l := range listeners.GatewayListeners {
		mappedRoutes.GatewayListeners[l.Name] = make([]routeParentRefTuple, 0)
		mappedRoutes.GatewayListenerSectionNameToPort[l.Name] = l.Port
	}

	for listenerSetNamespacedName, listenerSetListeners := range listeners.ListenerSetListeners.listenersPerListenerSet {

		vvv := make(map[gwv1.SectionName][]routeParentRefTuple)
		for _, listener := range listenerSetListeners {
			vvv[listener.listener.Name] = make([]routeParentRefTuple, 0)
		}

		mappedRoutes.ListenerSetListeners[listenerSetNamespacedName] = vvv
	}

	// First filter out any routes that are not intended for this Gateway.
	for _, route := range routes {
		for _, parentRef := range route.GetParentRefs() {
			fmt.Printf("KIND = %+v\n", parentRef.Kind)
			if parentRef.Kind == nil || *parentRef.Kind == gatewayKind {
				fmt.Printf("Found parent reference to gateway (%+v)\n", parentRef)
				if doesResourceAttachToGateway(parentRef, route.GetRouteNamespacedName().Namespace, gw) {
					fmt.Printf("got potential attach (%+v)(%+v)\n", route, parentRef)
					if parentRef.SectionName == nil {
						for k := range mappedRoutes.GatewayListeners {
							if parentRef.Port == nil || *parentRef.Port == mappedRoutes.GatewayListenerSectionNameToPort[k] {
								mappedRoutes.GatewayListeners[k] = append(mappedRoutes.GatewayListeners[k], routeParentRefTuple{
									route:     route,
									parentRef: parentRef,
								})
							}
						}
					} else {
						if parentRef.Port == nil || *parentRef.Port == mappedRoutes.GatewayListenerSectionNameToPort[*parentRef.SectionName] {
							mappedRoutes.GatewayListeners[*parentRef.SectionName] = append(mappedRoutes.GatewayListeners[*parentRef.SectionName], routeParentRefTuple{
								route:     route,
								parentRef: parentRef,
							})
						}
					}
				}

			} else if parentRef.Kind != nil && *parentRef.Kind == listenerSetKind {
				// listener set things //
			}
		}
	}

	result := make(map[int][]preLoadRouteDescriptor)
	compatibleHostnamesByPort := make(map[int32]map[string][]gwv1.Hostname)

	routesForGateway := make([]routeParentRefTuple, 0)
	fmt.Printf("GOT THIS!? %+v\n", mappedRoutes.GatewayListeners)
	for _, rrrs := range mappedRoutes.GatewayListeners {
		for _, r := range rrrs {
			routesForGateway = append(routesForGateway, r)
		}
	}

	// Dedupe - Check if route already exists for this port before adding
	seenRoutesPerPort := make(map[int]map[string]bool)
	// parentRefsMatchedListener tracks parentRefs that matched a listener's selector (routeAllowsAttachmentToListener=true)
	// even if they failed subsequent validation (e.g., namespace policy). This prevents NoMatchingParent errors
	// when the parentRef correctly identified a listener but was rejected for other reasons.
	// routeKey -> parentRefKey -> bool (true if matched a listener)
	parentRefsMatchedListener := make(map[string]map[string]bool)
	// matchedParentRefs stores the full ParentReference objects for parentRefs that successfully attached
	// (both routeAllowsAttachmentToListener=true AND listenerAllowsAttachment=true). These are returned
	// to the caller to indicate which specific parentRefs resulted in successful route-listener bindings.
	// routeKey -> parentRefKey -> ParentReference object
	matchedParentRefs := make(map[string]map[string]gwv1.ParentReference)

	// Track the number of routes connected to a listener, used in Listener status reported to the Gateway object.
	routesPerListener := make(map[gwv1.SectionName]int32)

	failedRoutesMap := make(map[string][]RouteData)

	// Next, greedily looking for the route to attach to.
	for _, listener := range listeners.GatewayListeners {
		routesPerListener[listener.Name] = 0
		// used for cross serving check
		hostnamesFromHttpRoutes := make(map[types.NamespacedName][]gwv1.Hostname)
		hostnamesFromGrpcRoutes := make(map[types.NamespacedName][]gwv1.Hostname)
		for _, route := range routesForGateway {
			routeKey := route.route.GetRouteIdentifier()

			// Track that this parentRef matched a listener (prevents NoMatchingParent)
			parentRefKey := getParentRefKey(route.parentRef, gw)
			if parentRefsMatchedListener[routeKey] == nil {
				parentRefsMatchedListener[routeKey] = make(map[string]bool)
			}
			parentRefsMatchedListener[routeKey][parentRefKey] = true

			compatibleHostnames, allowedAttachment, failedRouteData, err := ltr.listenerAttachmentHelper.listenerAllowsAttachment(ctx, gw, listener, route.route, &route.parentRef, hostnamesFromHttpRoutes, hostnamesFromGrpcRoutes)
			if err != nil {
				return nil, nil, nil, nil, nil, err
			}

			if failedRouteData != nil {
				failedRoutesMap[routeKey] = append(failedRoutesMap[routeKey], *failedRouteData)
			}

			ltr.logger.V(1).Info("listener allows attachment", "route", route.route.GetRouteNamespacedName(), "allowedAttachment", allowedAttachment)

			if allowedAttachment {
				routesPerListener[listener.Name] = routesPerListener[listener.Name] + 1
				port := listener.Port
				if seenRoutesPerPort[int(port)] == nil {
					seenRoutesPerPort[int(port)] = make(map[string]bool)
				}
				if !seenRoutesPerPort[int(port)][routeKey] {
					seenRoutesPerPort[int(port)][routeKey] = true
					result[int(port)] = append(result[int(port)], route.route)
				}

				// Store compatible hostnames per port per route per kind
				if compatibleHostnamesByPort[port] == nil {
					compatibleHostnamesByPort[port] = make(map[string][]gwv1.Hostname)
				}
				// Append hostnames for routes that attach to multiple listeners on the same port
				compatibleHostnamesByPort[port][routeKey] = append(compatibleHostnamesByPort[port][routeKey], compatibleHostnames...)
				// Only track in matchedParentRefs when attachment succeeds
				parentRefKey := getParentRefKey(route.parentRef, gw)
				if matchedParentRefs[routeKey] == nil {
					matchedParentRefs[routeKey] = make(map[string]gwv1.ParentReference)
				}
				if _, exists := matchedParentRefs[routeKey][parentRefKey]; !exists {
					matchedParentRefs[routeKey][parentRefKey] = route.parentRef
				}
			}
		}
	}

	// Generate NoMatchingParent only for parentRefs that never matched any listener
	for _, route := range routesForGateway {
		routeKey := route.route.GetRouteIdentifier()

		for _, parentRef := range route.route.GetParentRefs() {
			var namespace string
			if parentRef.Namespace != nil {
				namespace = string(*parentRef.Namespace)
			} else {
				namespace = route.route.GetRouteNamespacedName().Namespace
			}
			if string(parentRef.Name) != gw.Name || namespace != gw.Namespace {
				continue
			}

			parentRefKey := getParentRefKey(parentRef, gw)
			if _, matched := parentRefsMatchedListener[routeKey][parentRefKey]; !matched {
				rd := GenerateRouteData(false, true, string(gwv1.RouteReasonNoMatchingParent), RouteStatusInfoRejectedMessageParentNotMatch, route.route.GetRouteNamespacedName(), route.route.GetRouteKind(), route.route.GetRouteGeneration(), gw, parentRef.Port, parentRef.SectionName)
				failedRoutesMap[routeKey] = append(failedRoutesMap[routeKey], rd)
			}
		}
	}

	// Per Gateway API spec: "If 1 of 2 Gateway listeners accept attachment from the referencing Route,
	// the Route MUST be considered successfully attached."
	// For specific parentRefs (with sectionName/port), report individual status.
	// For generic parentRefs (no sectionName/port), skip failures if any listener accepted.
	failedRoutes := make([]RouteData, 0)
	for routeKey, routeDataList := range failedRoutesMap {
		for _, routeData := range routeDataList {
			parentRefKey := getParentRefKey(routeData.ParentRef, gw)
			// Check if this specific parentRef succeeded
			if _, succeeded := matchedParentRefs[routeKey][parentRefKey]; succeeded {
				continue
			}
			// For generic parentRefs (no sectionName/port), skip if route attached anywhere
			if routeData.ParentRef.SectionName == nil && routeData.ParentRef.Port == nil {
				if _, hasMatches := matchedParentRefs[routeKey]; hasMatches {
					continue
				}
			}
			failedRoutes = append(failedRoutes, routeData)
		}
	}

	// Convert matchedParentRefs to return format
	matchedParentRefsResult := make(map[string][]gwv1.ParentReference)
	for routeKey, refMap := range matchedParentRefs {
		for _, ref := range refMap {
			matchedParentRefsResult[routeKey] = append(matchedParentRefsResult[routeKey], ref)
		}
	}

	return result, compatibleHostnamesByPort, failedRoutes, matchedParentRefsResult, routesPerListener, nil
}

// getParentRefKey generates a unique key for a parentRef
func getParentRefKey(parentRef gwv1.ParentReference, gw gwv1.Gateway) string {
	var namespace string
	if parentRef.Namespace != nil {
		namespace = string(*parentRef.Namespace)
	} else {
		namespace = gw.Namespace
	}

	sectionName := ""
	if parentRef.SectionName != nil {
		sectionName = string(*parentRef.SectionName)
	}

	port := ""
	if parentRef.Port != nil {
		port = fmt.Sprintf("%d", *parentRef.Port)
	}

	return fmt.Sprintf("%s/%s/%s/%s", namespace, string(parentRef.Name), sectionName, port)
}
