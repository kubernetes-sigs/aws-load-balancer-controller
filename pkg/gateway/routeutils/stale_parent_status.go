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
	specParentRefs, parentStatuses := getRouteParentRefsAndStatuses(route.GetRawRoute())
	if len(parentStatuses) == 0 {
		return nil
	}

	routeNamespace := route.GetRouteNamespacedName().Namespace
	referenced := sets.New[string]()
	for _, ref := range specParentRefs {
		referenced.Insert(parentRefKey(ref, routeNamespace))
	}

	var stale []gwv1.ParentReference
	for _, status := range parentStatuses {
		if string(status.ControllerName) != controllerName {
			continue
		}
		if !referenced.Has(parentRefKey(status.ParentRef, routeNamespace)) {
			stale = append(stale, status.ParentRef)
		}
	}
	return stale
}

// getRouteParentRefsAndStatuses returns spec.parentRefs and status.parents of the underlying route object.
func getRouteParentRefsAndStatuses(rawRoute interface{}) ([]gwv1.ParentReference, []gwv1.RouteParentStatus) {
	switch r := rawRoute.(type) {
	case *gwv1.HTTPRoute:
		return r.Spec.ParentRefs, r.Status.Parents
	case *gwv1.GRPCRoute:
		return r.Spec.ParentRefs, r.Status.Parents
	case *gwv1.TLSRoute:
		return r.Spec.ParentRefs, r.Status.Parents
	case *gwv1.TCPRoute:
		return r.Spec.ParentRefs, r.Status.Parents
	case *gwv1.UDPRoute:
		return r.Spec.ParentRefs, r.Status.Parents
	}
	return nil, nil
}

// parentRefKey identifies a parentRef the same way the route status reconciler does, so that an entry
// considered stale here is exactly an entry the reconciler will drop.
func parentRefKey(ref gwv1.ParentReference, routeNamespace string) string {
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
