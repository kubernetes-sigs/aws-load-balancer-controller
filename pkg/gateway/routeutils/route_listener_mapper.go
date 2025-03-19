package routeutils

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ListenerToRouteMapper interface {
	Map(context context.Context, gw *gwv1.Gateway, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, error)
}

var _ ListenerToRouteMapper = &listenerToRouteMapper{}

type listenerToRouteMapper struct {
	k8sClient client.Client
}

func (ltr *listenerToRouteMapper) Map(ctx context.Context, gw *gwv1.Gateway, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, error) {
	result := make(map[int][]preLoadRouteDescriptor)

	routesForGateway := make([]preLoadRouteDescriptor, 0)
	for _, route := range routes {
		if ltr.doesRouteAttachToGateway(gw, route) {
			routesForGateway = append(routesForGateway, route)
		}
	}

	// Approach is to greedily add as many relevant routes to each listener.
	for _, listener := range gw.Spec.Listeners {
		for _, route := range routesForGateway {

			if !ltr.routeAllowsAttachment(listener, route) {
				continue
			}

			allowedAttachment, err := ltr.listenerAllowsAttachment(ctx, gw, listener, route)
			if err != nil {
				return nil, err
			}

			if allowedAttachment {
				result[int(listener.Port)] = append(result[int(listener.Port)], route)
			}
		}
	}
	return result, nil
}

func (ltr *listenerToRouteMapper) doesRouteAttachToGateway(gw *gwv1.Gateway, route preLoadRouteDescriptor) bool {
	for _, parentRef := range route.GetParentRefs() {

		// Default for kind is Gateway.
		if parentRef.Kind != nil && *parentRef.Kind != "Gateway" {
			continue
		}

		var namespaceToCompare string

		if parentRef.Namespace != nil {
			namespaceToCompare = string(*parentRef.Namespace)
		} else {
			namespaceToCompare = gw.Namespace
		}

		if string(parentRef.Name) == gw.Name && gw.Namespace == namespaceToCompare {
			return true
		}
	}

	return false
}

func (ltr *listenerToRouteMapper) routeAllowsAttachment(listener gwv1.Listener, route preLoadRouteDescriptor) bool {
	// We've already validated that the route belongs to the gateway.
	for _, parentRef := range route.GetParentRefs() {

		if parentRef.SectionName != nil && string(*parentRef.SectionName) != string(listener.Name) {
			continue
		}

		if parentRef.Port != nil && *parentRef.Port != listener.Port {
			continue
		}

		return true
	}

	return false
}

func (ltr *listenerToRouteMapper) listenerAllowsAttachment(ctx context.Context, gw *gwv1.Gateway, listener gwv1.Listener, route preLoadRouteDescriptor) (bool, error) {
	namespaceOK, err := ltr.namespaceCheck(ctx, gw, listener, route)
	if err != nil {
		return false, err
	}

	if !namespaceOK {
		return false, nil
	}

	if !ltr.kindCheck(listener, route) {
		return false, nil
	}
	return true, nil
}

func (ltr *listenerToRouteMapper) namespaceCheck(ctx context.Context, gw *gwv1.Gateway, listener gwv1.Listener, route preLoadRouteDescriptor) (bool, error) {
	var allowedNamespaces gwv1.FromNamespaces

	if listener.AllowedRoutes == nil || listener.AllowedRoutes.Namespaces == nil || listener.AllowedRoutes.Namespaces.From == nil {
		allowedNamespaces = gwv1.NamespacesFromSame
	} else {
		allowedNamespaces = *listener.AllowedRoutes.Namespaces.From
	}

	namespacedName := route.GetRouteNamespacedName()

	switch allowedNamespaces {
	case gwv1.NamespacesFromSame:
		return gw.Namespace == namespacedName.Namespace, nil
	case gwv1.NamespacesFromAll:
		return true, nil
	case gwv1.NamespacesFromSelector:
		if listener.AllowedRoutes.Namespaces.Selector == nil {
			return false, nil
		}
		// This should be executed off the client-go cache, hence we do not need to perform local caching.
		namespaces, err := ltr.getNamespacesFromSelector(ctx, listener.AllowedRoutes.Namespaces.Selector)
		if err != nil {
			return false, err
		}

		if !namespaces.Has(namespacedName.Namespace) {
			return false, nil
		}
		return true, nil
	default:
		// Unclear what to do in this case, let's try to be best effort and just ignore this value.
		return false, nil
	}
}

func (ltr *listenerToRouteMapper) kindCheck(listener gwv1.Listener, route preLoadRouteDescriptor) bool {

	var allowedRoutes sets.Set[string]

	// Allowed Routes being null defaults to no checking required.
	if listener.AllowedRoutes.Kinds == nil {
		allowedRoutes = sets.New[string](defaultProtocolToRouteKindMap[listener.Protocol])
	} else {
		// TODO - Not sure how to handle versioning (correctly) here.
		// So going to ignore the group checks for now :x
		allowedRoutes = sets.New[string]()
		for _, v := range listener.AllowedRoutes.Kinds {
			allowedRoutes.Insert(string(v.Kind))
		}
	}
	return allowedRoutes.Has(route.GetRouteKind())
}

func (ltr *listenerToRouteMapper) getNamespacesFromSelector(context context.Context, selector *metav1.LabelSelector) (sets.Set[string], error) {
	namespaceList := v1.NamespaceList{}

	convertedSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}
	listOpts := client.ListOptions{LabelSelector: convertedSelector}

	err = ltr.k8sClient.List(context, &namespaceList, &listOpts)
	if err != nil {
		return nil, err
	}

	namespaces := sets.New[string]()

	for _, ns := range namespaceList.Items {
		namespaces.Insert(ns.Name)
	}

	return namespaces, nil
}
