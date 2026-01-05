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
	mapGatewayAndRoutes(context context.Context, gw gwv1.Gateway, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, map[int32]map[string][]gwv1.Hostname, []RouteData, map[string][]gwv1.ParentReference, error)
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
// Returns: (routesByPort, compatibleHostnamesByPort, failedRoutes, matchedParentRefs, error)
func (ltr *listenerToRouteMapperImpl) mapGatewayAndRoutes(ctx context.Context, gw gwv1.Gateway, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, map[int32]map[string][]gwv1.Hostname, []RouteData, map[string][]gwv1.ParentReference, error) {
	// Discover listeners once at the beginning
	discoveredListeners := DiscoverListeners(&gw)

	result := make(map[int][]preLoadRouteDescriptor)
	compatibleHostnamesByPort := make(map[int32]map[string][]gwv1.Hostname)

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
	failedRoutesMap := make(map[string][]RouteData)

	// Next, greedily looking for the route to attach to.
	for _, discoveredListener := range discoveredListeners.All {
		listener := discoveredListener.Listener
		// used for cross serving check
		hostnamesFromHttpRoutes := make(map[types.NamespacedName][]gwv1.Hostname)
		hostnamesFromGrpcRoutes := make(map[types.NamespacedName][]gwv1.Hostname)
		for _, route := range routesForGateway {
			routeKey := route.GetRouteIdentifier()

			// We need to check both paths (route -> listener) and (listener -> route)
			// for connection viability.
			allowed, matchedParentRef := ltr.routeAttachmentHelper.routeAllowsAttachmentToListener(gw, listener, route)
			if !allowed {
				ltr.logger.V(1).Info("Route doesnt allow attachment")
				continue
			}
			// Track that this parentRef matched a listener (prevents NoMatchingParent)
			if matchedParentRef != nil {
				parentRefKey := getParentRefKey(*matchedParentRef, gw)
				if parentRefsMatchedListener[routeKey] == nil {
					parentRefsMatchedListener[routeKey] = make(map[string]bool)
				}
				parentRefsMatchedListener[routeKey][parentRefKey] = true
			}

			compatibleHostnames, allowedAttachment, failedRouteData, err := ltr.listenerAttachmentHelper.listenerAllowsAttachment(ctx, gw, listener, route, matchedParentRef, hostnamesFromHttpRoutes, hostnamesFromGrpcRoutes)
			if err != nil {
				return nil, nil, nil, nil, err
			}

			if failedRouteData != nil {
				failedRoutesMap[routeKey] = append(failedRoutesMap[routeKey], *failedRouteData)
			}

			ltr.logger.V(1).Info("listener allows attachment", "route", route.GetRouteNamespacedName(), "allowedAttachment", allowedAttachment)

			if allowedAttachment {
				port := int32(listener.Port)
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
				// Only track in matchedParentRefs when attachment succeeds
				if matchedParentRef != nil {
					parentRefKey := getParentRefKey(*matchedParentRef, gw)
					if matchedParentRefs[routeKey] == nil {
						matchedParentRefs[routeKey] = make(map[string]gwv1.ParentReference)
					}
					if _, exists := matchedParentRefs[routeKey][parentRefKey]; !exists {
						matchedParentRefs[routeKey][parentRefKey] = *matchedParentRef
					}
				}
			}

		}
	}

	// Generate NoMatchingParent only for parentRefs that never matched any listener
	for _, route := range routesForGateway {
		routeKey := route.GetRouteIdentifier()

		for _, parentRef := range route.GetParentRefs() {
			var namespace string
			if parentRef.Namespace != nil {
				namespace = string(*parentRef.Namespace)
			} else {
				namespace = route.GetRouteNamespacedName().Namespace
			}
			if string(parentRef.Name) != gw.Name || namespace != gw.Namespace {
				continue
			}

			parentRefKey := getParentRefKey(parentRef, gw)
			if _, matched := parentRefsMatchedListener[routeKey][parentRefKey]; !matched {
				rd := GenerateRouteData(false, true, string(gwv1.RouteReasonNoMatchingParent), RouteStatusInfoRejectedMessageParentNotMatch, route.GetRouteNamespacedName(), route.GetRouteKind(), route.GetRouteGeneration(), gw, parentRef.Port, parentRef.SectionName)
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

	return result, compatibleHostnamesByPort, failedRoutes, matchedParentRefsResult, nil
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
