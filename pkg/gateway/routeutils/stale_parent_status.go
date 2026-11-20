package routeutils

import (
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	RouteStatusInfoParentRefRemovedReason  = "ParentRefRemoved"
	RouteStatusInfoParentRefRemovedMessage = "Route no longer references this parent, removing stale status entry"
)

// findStaleParentStatuses returns the parentRefs of route status entries owned by controllerName
// that are no longer present in the route's spec.parentRefs.
//
// When a route is detached from a Gateway managed by this controller (e.g. it is moved to a Gateway of a
// different GatewayClass), the route no longer maps to any of our listeners, so no status update is ever
// generated for it and our entry in status.parents is left behind, still reporting Accepted=True.
// Enqueuing a status update for these parentRefs lets the route reconciler, which rebuilds status.parents
// from spec.parentRefs, drop the stale entry.
func findStaleParentStatuses(route preLoadRouteDescriptor, controllerName string) []gwv1.ParentReference {
	parentStatuses := route.GetParentStatuses()
	if len(parentStatuses) == 0 {
		return nil
	}

	routeNamespace := route.GetRouteNamespacedName().Namespace
	referenced := sets.New[string]()
	for _, ref := range route.GetParentRefs() {
		referenced.Insert(ParentRefKey(ref, routeNamespace))
	}

	var stale []gwv1.ParentReference
	for _, status := range parentStatuses {
		if string(status.ControllerName) != controllerName {
			continue
		}
		if !referenced.Has(ParentRefKey(status.ParentRef, routeNamespace)) {
			stale = append(stale, status.ParentRef)
		}
	}
	return stale
}

// ParentRefKey returns a unique key for a parentRef, defaulting an unset namespace to the route namespace.
// It is shared by the route loader and the route status reconciler: an entry considered stale here must be
// exactly an entry the reconciler drops, so both sides must key parentRefs identically.
func ParentRefKey(ref gwv1.ParentReference, routeNamespace string) string {
	group := ""
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	kind := ""
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	namespace := routeNamespace
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	sectionName := ""
	if ref.SectionName != nil {
		sectionName = string(*ref.SectionName)
	}
	port := ""
	if ref.Port != nil {
		port = strconv.Itoa(int(*ref.Port))
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s", group, kind, namespace, ref.Name, sectionName, port)
}
