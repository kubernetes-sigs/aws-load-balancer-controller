package routeutils

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
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
	compatibleHostnamesByPort := make(map[int32]map[string]sets.Set[gwv1.Hostname])

	hostnamesFromHttpRoutes := make(map[int32]sets.Set[gwv1.Hostname])
	hostnamesFromGrpcRoutes := make(map[int32]sets.Set[gwv1.Hostname])

	initialListenerMapping := map[gwv1.SectionName][]routeParentRefTuple{}

	gatewayListenerSectionNameToListener := make(map[gwv1.SectionName]gwv1.Listener)

	for _, l := range listeners.GatewayListeners {
		initialListenerMapping[l.Name] = make([]routeParentRefTuple, 0)
		gatewayListenerSectionNameToListener[l.Name] = l
	}

	listenerSetListeners := make(map[types.NamespacedName]map[gwv1.SectionName][]routeParentRefTuple)
	listenerSetListenerSectionNameToListener := make(map[types.NamespacedName]map[gwv1.SectionName]gwv1.Listener)

	for nsn, listenerSet := range listeners.ListenerSetListeners.listenersPerListenerSet {
		listenerSetListenerSectionNameToListener[nsn] = make(map[gwv1.SectionName]gwv1.Listener)
		listenerSetListeners[nsn] = make(map[gwv1.SectionName][]routeParentRefTuple)
		for _, listenerSetSource := range listenerSet {
			listenerSetListenerSectionNameToListener[nsn][listenerSetSource.listener.Name] = listenerSetSource.listener
			listenerSetListeners[nsn][listenerSetSource.listener.Name] = make([]routeParentRefTuple, 0)
		}
	}

	failedRoutes := make([]RouteData, 0)

	// route identifier -> all parent references
	matchedParentRefsResult := make(map[string][]gwv1.ParentReference)

	for _, route := range routes {
		for _, parentRef := range route.GetParentRefs() {
			var refTuple routeParentRefTuple
			doTry := doesResourceAttachToGateway(parentRef, route.GetRouteNamespacedName().Namespace, gw)
			var parentNamespace string
			var targetListeners []gwv1.Listener
			var sectionToListenerMap map[gwv1.SectionName]gwv1.Listener
			var acceptedListenersMap map[gwv1.SectionName][]routeParentRefTuple
			if parentRef.Kind == nil || *parentRef.Kind == gatewayKind {
				refTuple = routeParentRefTuple{
					route:      route,
					parentRef:  parentRef,
					parent:     gw,
					parentKind: gatewayKind,
				}
				doTry = true
				parentNamespace = gw.Namespace
				sectionToListenerMap = gatewayListenerSectionNameToListener
				acceptedListenersMap = initialListenerMapping
			} else if parentRef.Kind != nil && *parentRef.Kind == listenerSetKind {

				namespace := route.GetRouteNamespacedName().Namespace
				if parentRef.Namespace != nil {
					namespace = string(*parentRef.Namespace)
				}
				nsn := types.NamespacedName{
					Namespace: namespace,
					Name:      string(parentRef.Name),
				}

				referencedListenerSet, ok := listeners.ListenerSetListeners.acceptedListenerSets[nsn]
				if !ok {
					// We don't know about this listener set, ignore it.
					continue
				}

				// listener set things //
				refTuple = routeParentRefTuple{
					route:      route,
					parentRef:  parentRef,
					parent:     referencedListenerSet,
					parentKind: listenerSetKind,
				}
				doTry = true
				parentNamespace = referencedListenerSet.Namespace
				sectionToListenerMap = listenerSetListenerSectionNameToListener[nsn]
				acceptedListenersMap = listenerSetListeners[nsn]
			}
			if doTry {
				localFailedRoutes, err := ltr.attemptListenerRouteAttachment(ctx, parentNamespace, targetListeners, sectionToListenerMap, refTuple, acceptedListenersMap, compatibleHostnamesByPort, hostnamesFromHttpRoutes, hostnamesFromGrpcRoutes, matchedParentRefsResult)
				if err != nil {
					return nil, nil, nil, nil, nil, err
				}
				failedRoutes = append(failedRoutes, localFailedRoutes...)
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

func (ltr *listenerToRouteMapperImpl) attemptListenerRouteAttachment(ctx context.Context, parentNamespace string, targetListeners []gwv1.Listener, listenerSectionNameToListener map[gwv1.SectionName]gwv1.Listener, refTuple routeParentRefTuple, acceptedRouteMap map[gwv1.SectionName][]routeParentRefTuple, compatibleHostnamesByPort map[int32]map[string]sets.Set[gwv1.Hostname], hostnamesFromHttpRoutes map[int32]sets.Set[gwv1.Hostname], hostnamesFromGrpcRoutes map[int32]sets.Set[gwv1.Hostname], matchedParentRefsResult map[string][]gwv1.ParentReference) ([]RouteData, error) {
	var anyAttach bool
	failedRoutes := make([]RouteData, 0)
	// Check to see if the listener(s) specified by the route parent ref will allow attachment
	if refTuple.parentRef.SectionName == nil {
		for _, listener := range targetListeners {
			if refTuple.parentRef.Port == nil || *refTuple.parentRef.Port == listener.Port {
				rd, err := ltr.attemptRouteSectionAttachment(ctx, parentNamespace, listener, refTuple, acceptedRouteMap, compatibleHostnamesByPort, hostnamesFromHttpRoutes, hostnamesFromGrpcRoutes)
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
		targetListener, ok := listenerSectionNameToListener[*refTuple.parentRef.SectionName]
		if !ok {
			return failedRoutes, nil
		}
		rd, err := ltr.attemptRouteSectionAttachment(ctx, parentNamespace, targetListener, refTuple, acceptedRouteMap, compatibleHostnamesByPort, hostnamesFromHttpRoutes, hostnamesFromGrpcRoutes)
		if err != nil {
			return failedRoutes, err
		}
		if rd != nil {
			failedRoutes = append(failedRoutes, *rd)
		}
		anyAttach = rd == nil
	}

	if anyAttach {
		_, ok := matchedParentRefsResult[refTuple.route.GetRouteIdentifier()]
		if !ok {
			matchedParentRefsResult[refTuple.route.GetRouteIdentifier()] = make([]gwv1.ParentReference, 0)
		}
		matchedParentRefsResult[refTuple.route.GetRouteIdentifier()] = append(matchedParentRefsResult[refTuple.route.GetRouteIdentifier()], refTuple.parentRef)
		return []RouteData{}, nil
	}
	// Attachment was attempted, but rejected by a listener policy (e.g. namespace, route kind limitation)
	if len(failedRoutes) > 0 {
		failedRoutes = append(failedRoutes, failedRoutes...)
	} else {
		// Attachment wasn't even attempted, probably because the parent ref had an invalid section name / port.
		failedRoutes = append(failedRoutes, GenerateRouteData(false, true, string(gwv1.RouteReasonNoMatchingParent), RouteStatusInfoRejectedMessageParentNotMatch, refTuple.route.GetRouteNamespacedName(), refTuple.route.GetRouteKind(), refTuple.route.GetRouteGeneration(), refTuple.parentRef))
	}

	return failedRoutes, nil
}

func (ltr *listenerToRouteMapperImpl) attemptRouteSectionAttachment(ctx context.Context, parentNamespace string, targetListener gwv1.Listener, refTuple routeParentRefTuple, acceptedRouteMap map[gwv1.SectionName][]routeParentRefTuple, compatibleHostnamesByPort map[int32]map[string]sets.Set[gwv1.Hostname], hostnamesFromHttpRoutes map[int32]sets.Set[gwv1.Hostname], hostnamesFromGrpcRoutes map[int32]sets.Set[gwv1.Hostname]) (*RouteData, error) {
	compatibleHostnames, failedRouteData, err := ltr.listenerAttachmentHelper.listenerAllowsAttachment(ctx, parentNamespace, targetListener, refTuple.route, &refTuple.parentRef, hostnamesFromHttpRoutes, hostnamesFromGrpcRoutes)
	if err != nil {
		return nil, err
	}

	if failedRouteData == nil {
		acceptedRouteMap[*refTuple.parentRef.SectionName] = append(acceptedRouteMap[*refTuple.parentRef.SectionName], refTuple)
		// Store compatible hostnames per port per route per kind
		if compatibleHostnamesByPort[targetListener.Port] == nil {
			compatibleHostnamesByPort[targetListener.Port] = make(map[string]sets.Set[gwv1.Hostname])
		}
		if compatibleHostnamesByPort[targetListener.Port][refTuple.route.GetRouteIdentifier()] == nil {
			compatibleHostnamesByPort[targetListener.Port][refTuple.route.GetRouteIdentifier()] = make(sets.Set[gwv1.Hostname])
		}
		// Append hostnames for routes that attach to multiple listeners on the same port
		for _, ch := range compatibleHostnames {
			compatibleHostnamesByPort[targetListener.Port][refTuple.route.GetRouteIdentifier()].Insert(ch)
		}
	}
	return failedRouteData, nil
}
