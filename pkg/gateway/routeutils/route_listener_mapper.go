package routeutils

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// listenerRouteMapResult holds all outputs from the listener-to-route mapping process.
type listenerRouteMapResult struct {
	routesByPort              map[int32][]preLoadRouteDescriptor
	compatibleHostnamesByPort map[int32]map[string]sets.Set[gwv1.Hostname]
	failedRoutes              []RouteData
	matchedParentRefs         map[string][]gwv1.ParentReference
	attachedListeners         []gwv1.Listener
}

// attachmentState groups the mutable accumulator maps threaded through the attachment methods.
type attachmentState struct {
	compatibleHostnamesByPort map[int32]map[string]sets.Set[gwv1.Hostname]
	hostnamesFromHttpRoutes   map[int32]sets.Set[gwv1.Hostname]
	hostnamesFromGrpcRoutes   map[int32]sets.Set[gwv1.Hostname]
	matchedParentRefs         map[string][]gwv1.ParentReference
}

// parentResolution bundles the per-parent data resolved from a parentRef.
type parentResolution struct {
	refTuple                      routeParentRefTuple
	parentNamespace               string
	targetListeners               []gwv1.Listener
	listenerSectionNameToListener map[gwv1.SectionName]gwv1.Listener
	acceptedRouteMap              map[gwv1.SectionName][]routeParentRefTuple
}

// listenerToRouteMapper is an internal utility that will map a list of routes to the listeners of a gateway
// if the gateway and/or route are incompatible, then the route is discarded.
type listenerToRouteMapper interface {
	mapListenersAndRoutes(ctx context.Context, gw gwv1.Gateway, listeners allListeners, routes []preLoadRouteDescriptor, listenerValidationResults ValidatedGatewayListeners) (listenerRouteMapResult, error)
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

// resolveParentRef resolves a parentRef into the data needed for attachment attempts.
// Returns nil if the parentRef doesn't match any known parent.
func (ltr *listenerToRouteMapperImpl) resolveParentRef(
	parentRef gwv1.ParentReference,
	route preLoadRouteDescriptor,
	gw gwv1.Gateway,
	listeners allListeners,
	gatewayListenerMapping map[gwv1.SectionName][]routeParentRefTuple,
	gatewayListenerSectionNameToListener map[gwv1.SectionName]gwv1.Listener,
	listenerSetListeners map[types.NamespacedName]map[gwv1.SectionName][]routeParentRefTuple,
	listenerSetListenerSectionNameToListener map[types.NamespacedName]map[gwv1.SectionName]gwv1.Listener,
) *parentResolution {
	if parentRef.Kind == nil || *parentRef.Kind == gatewayKind {
		if !doesResourceAttachToGateway(parentRef, route.GetRouteNamespacedName().Namespace, gw) {
			return nil
		}
		return &parentResolution{
			refTuple:                      routeParentRefTuple{route: route, parentRef: parentRef},
			parentNamespace:               gw.Namespace,
			targetListeners:               listeners.GatewayListeners,
			listenerSectionNameToListener: gatewayListenerSectionNameToListener,
			acceptedRouteMap:              gatewayListenerMapping,
		}
	}

	if parentRef.Kind != nil && *parentRef.Kind == listenerSetKind {
		namespace := route.GetRouteNamespacedName().Namespace
		if parentRef.Namespace != nil {
			namespace = string(*parentRef.Namespace)
		}
		nsn := types.NamespacedName{
			Namespace: namespace,
			Name:      string(parentRef.Name),
		}

		referencedListenerSet, found := listeners.ListenerSetListeners.acceptedListenerSets[nsn]
		if !found {
			return nil
		}

		return &parentResolution{
			refTuple:                      routeParentRefTuple{route: route, parentRef: parentRef},
			parentNamespace:               referencedListenerSet.Namespace,
			targetListeners:               extractListenersFromSources(listeners.ListenerSetListeners.listenersPerListenerSet[nsn]),
			listenerSectionNameToListener: listenerSetListenerSectionNameToListener[nsn],
			acceptedRouteMap:              listenerSetListeners[nsn],
		}
	}

	return nil
}

// mapListenersAndRoutes will map route to the corresponding listener ports using the Gateway API spec rules.
func (ltr *listenerToRouteMapperImpl) mapListenersAndRoutes(ctx context.Context, gw gwv1.Gateway, listeners allListeners, routes []preLoadRouteDescriptor, listenerValidationResults ValidatedGatewayListeners) (listenerRouteMapResult, error) {
	// Step 1 - Generate initial state maps, to allow for quick calculations.

	state := &attachmentState{
		compatibleHostnamesByPort: make(map[int32]map[string]sets.Set[gwv1.Hostname]),
		hostnamesFromHttpRoutes:   make(map[int32]sets.Set[gwv1.Hostname]),
		hostnamesFromGrpcRoutes:   make(map[int32]sets.Set[gwv1.Hostname]),
		matchedParentRefs:         make(map[string][]gwv1.ParentReference),
	}

	failedRoutes := make([]RouteData, 0)

	finalListeners := make([]gwv1.Listener, 0)
	routesForListenerPorts := make(map[int32][]preLoadRouteDescriptor)
	seen := make(map[int32]sets.Set[string])

	// gatewayListenerMapping - maps the routes that have successfully attached to a listener belonging to a Gateway.
	gatewayListenerMapping := map[gwv1.SectionName][]routeParentRefTuple{}
	// gatewayListenerSectionNameToListener - maps the listener section name to the listener object, specifically for the Gateway.
	gatewayListenerSectionNameToListener := make(map[gwv1.SectionName]gwv1.Listener)

	for _, l := range listeners.GatewayListeners {
		gatewayListenerMapping[l.Name] = make([]routeParentRefTuple, 0)
		gatewayListenerSectionNameToListener[l.Name] = l
		// Populate initial value for this port, this ensures that we materialize listeners w/ no routes.
		_, ok := seen[l.Port]
		if !ok {
			seen[l.Port] = make(sets.Set[string])
			routesForListenerPorts[l.Port] = make([]preLoadRouteDescriptor, 0)
		}
	}

	// Now do the same for listeners coming from a ListenerSet.

	// listenerSetListeners - maps the routes that have successfully attached to each listener belong to a ListenerSet.
	listenerSetListeners := make(map[types.NamespacedName]map[gwv1.SectionName][]routeParentRefTuple)
	// listenerSetListenerSectionNameToListener - maps the listener section name to listener, for each ListenerSet.
	listenerSetListenerSectionNameToListener := make(map[types.NamespacedName]map[gwv1.SectionName]gwv1.Listener)

	for nsn, listenerSet := range listeners.ListenerSetListeners.listenersPerListenerSet {
		ltr.logger.Info("Got this listener set", "namespace", nsn)
		listenerSetListenerSectionNameToListener[nsn] = make(map[gwv1.SectionName]gwv1.Listener)
		listenerSetListeners[nsn] = make(map[gwv1.SectionName][]routeParentRefTuple)
		for _, listenerSetSource := range listenerSet {
			listenerSetListenerSectionNameToListener[nsn][listenerSetSource.listener.Name] = listenerSetSource.listener
			listenerSetListeners[nsn][listenerSetSource.listener.Name] = make([]routeParentRefTuple, 0)
			// Populate initial value for this port, this ensures that we materialize listeners w/ no routes.
			_, ok := seen[listenerSetSource.listener.Port]
			if !ok {
				seen[listenerSetSource.listener.Port] = make(sets.Set[string])
				routesForListenerPorts[listenerSetSource.listener.Port] = make([]preLoadRouteDescriptor, 0)
			}
		}
	}

	// Step 2 - Go through each route and each parent ref on the route.
	// We are looking for parent refs that are attempting attachment to a listener(s) on the Gateway
	// OR a ListenerSet that is attached to the Gateway.
	for _, route := range routes {
		for _, parentRef := range route.GetParentRefs() {
			resolved := ltr.resolveParentRef(
				parentRef, route, gw, listeners,
				gatewayListenerMapping, gatewayListenerSectionNameToListener,
				listenerSetListeners, listenerSetListenerSectionNameToListener,
			)

			// Resolved being nil signifies that the parent ref doesn't match our Gateway or any of the ListenerSets.
			if resolved == nil {
				continue
			}

			// Getting here means that the route wants to attach. We need to be sure that the Listener completes the handshake.
			localFailedRoutes, err := ltr.attemptListenerRouteAttachment(ctx, resolved, state)
			if err != nil {
				return listenerRouteMapResult{}, err
			}
			failedRoutes = append(failedRoutes, localFailedRoutes...)
		}
	}

	for sn, snRoutes := range gatewayListenerMapping {
		lvr, ok := listenerValidationResults.GatewayListenerValidation.Results[sn]
		if ok {
			lvr.AttachedRoutesCount = int32(len(snRoutes))
			listenerValidationResults.GatewayListenerValidation.Results[sn] = lvr
		}
	}

	for nsn, snRouteMapping := range listenerSetListeners {
		for sn, snRoutes := range snRouteMapping {
			validationmap, foundListenerSet := listenerValidationResults.ListenerSetListenerValidation[nsn]
			if foundListenerSet {
				lvr, foundSection := validationmap.Results[sn]
				if foundSection {
					lvr.AttachedRoutesCount = int32(len(snRoutes))
					validationmap.Results[sn] = lvr
				}
			}
		}
	}

	ltr.generateRoutePerPortSet(gatewayListenerMapping, gatewayListenerSectionNameToListener, seen, routesForListenerPorts, &finalListeners, listenerValidationResults.GatewayListenerValidation.Results)

	for listenerSetNsn, listenerRouteMapping := range listenerSetListeners {
		ltr.generateRoutePerPortSet(listenerRouteMapping, listenerSetListenerSectionNameToListener[listenerSetNsn], seen, routesForListenerPorts, &finalListeners, listenerValidationResults.ListenerSetListenerValidation[listenerSetNsn].Results)
	}

	for _, v := range finalListeners {
		ltr.logger.Info(fmt.Sprintf("Got this listener %+v [gw %+v]", v, k8s.NamespacedName(&gw)))
	}

	return listenerRouteMapResult{
		routesByPort:              routesForListenerPorts,
		compatibleHostnamesByPort: state.compatibleHostnamesByPort,
		failedRoutes:              failedRoutes,
		matchedParentRefs:         state.matchedParentRefs,
		attachedListeners:         finalListeners,
	}, nil
}

func (ltr *listenerToRouteMapperImpl) attemptListenerRouteAttachment(ctx context.Context, resolved *parentResolution, state *attachmentState) ([]RouteData, error) {
	var anyAttach bool
	failedRoutes := make([]RouteData, 0)
	refTuple := resolved.refTuple

	// Check to see if the listener(s) specified by the route parent ref will allow attachment
	if refTuple.parentRef.SectionName == nil {
		// When the section name is nil, this means that the route wants to attach to every listener in the parent object.
		for _, listener := range resolved.targetListeners {
			if refTuple.parentRef.Port == nil || *refTuple.parentRef.Port == listener.Port {
				rd, err := ltr.attemptRouteSectionAttachment(ctx, resolved.parentNamespace, listener, refTuple, resolved.acceptedRouteMap, state)
				if err != nil {
					return failedRoutes, err
				}
				if rd != nil {
					failedRoutes = append(failedRoutes, *rd)
				}
				anyAttach = anyAttach || rd == nil
			}
		}
	} else {
		// When section name is specified, the route only is attempting attachment to that specific listener.
		targetListener, ok := resolved.listenerSectionNameToListener[*refTuple.parentRef.SectionName]
		if ok && (refTuple.parentRef.Port == nil || *refTuple.parentRef.Port == targetListener.Port) {
			rd, err := ltr.attemptRouteSectionAttachment(ctx, resolved.parentNamespace, targetListener, refTuple, resolved.acceptedRouteMap, state)
			if err != nil {
				return failedRoutes, err
			}
			if rd != nil {
				failedRoutes = append(failedRoutes, *rd)
			}
			anyAttach = rd == nil
		}
	}

	// Per Gateway API spec: "If 1 of 2 Gateway listeners accept attachment from the referencing Route,
	// the Route MUST be considered successfully attached."
	// For specific parentRefs (with sectionName/port), report individual status.
	// For generic parentRefs (no sectionName/port), skip failures if any listener accepted.
	if anyAttach {
		routeID := refTuple.route.GetRouteIdentifier()
		state.matchedParentRefs[routeID] = append(state.matchedParentRefs[routeID], refTuple.parentRef)
		return []RouteData{}, nil
	}

	// Attachment wasn't even attempted, probably because the parent ref had an invalid section name / port.
	if len(failedRoutes) == 0 {
		failedRoutes = append(failedRoutes, GenerateRouteData(false, true, string(gwv1.RouteReasonNoMatchingParent), RouteStatusInfoRejectedMessageParentNotMatch, refTuple.route.GetRouteNamespacedName(), refTuple.route.GetRouteKind(), refTuple.route.GetRouteGeneration(), refTuple.parentRef))
	}

	return failedRoutes, nil
}

func (ltr *listenerToRouteMapperImpl) attemptRouteSectionAttachment(ctx context.Context, parentNamespace string, targetListener gwv1.Listener, refTuple routeParentRefTuple, acceptedRouteMap map[gwv1.SectionName][]routeParentRefTuple, state *attachmentState) (*RouteData, error) {
	compatibleHostnames, failedRouteData, err := ltr.listenerAttachmentHelper.listenerAllowsAttachment(ctx, parentNamespace, targetListener, refTuple.route, refTuple.parentRef, state.hostnamesFromHttpRoutes, state.hostnamesFromGrpcRoutes)
	if err != nil {
		return nil, err
	}

	if failedRouteData != nil {
		return failedRouteData, nil
	}

	acceptedRouteMap[targetListener.Name] = append(acceptedRouteMap[targetListener.Name], refTuple)
	// Store compatible hostnames per port per route per kind
	if state.compatibleHostnamesByPort[targetListener.Port] == nil {
		state.compatibleHostnamesByPort[targetListener.Port] = make(map[string]sets.Set[gwv1.Hostname])
	}
	routeID := refTuple.route.GetRouteIdentifier()
	if state.compatibleHostnamesByPort[targetListener.Port][routeID] == nil {
		state.compatibleHostnamesByPort[targetListener.Port][routeID] = make(sets.Set[gwv1.Hostname])
	}
	// Append hostnames for routes that attach to multiple listeners on the same port
	for _, ch := range compatibleHostnames {
		state.compatibleHostnamesByPort[targetListener.Port][routeID].Insert(ch)
	}
	return nil, nil
}

func (ltr *listenerToRouteMapperImpl) generateRoutePerPortSet(listenerRouteMapping map[gwv1.SectionName][]routeParentRefTuple, sectionNameToPort map[gwv1.SectionName]gwv1.Listener, seen map[int32]sets.Set[string], result map[int32][]preLoadRouteDescriptor, finalListeners *[]gwv1.Listener, listenerValidationResult map[gwv1.SectionName]ListenerValidationResult) {
	for sectionName, listenerRoutes := range listenerRouteMapping {
		validation, ok := listenerValidationResult[sectionName]
		if !ok || !validation.IsValid {
			continue
		}

		targetPort := sectionNameToPort[sectionName].Port
		*finalListeners = append(*finalListeners, sectionNameToPort[sectionName])
		for _, lr := range listenerRoutes {
			id := lr.route.GetRouteIdentifier()
			if seen[targetPort].Has(id) {
				continue
			}
			seen[targetPort].Insert(id)
			result[targetPort] = append(result[targetPort], lr.route)
		}
	}
}

func extractListenersFromSources(src []listenerSetListenerSource) []gwv1.Listener {
	result := make([]gwv1.Listener, 0, len(src))
	for _, v := range src {
		result = append(result, v.listener)
	}
	return result
}
