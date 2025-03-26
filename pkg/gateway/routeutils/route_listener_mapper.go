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
	Map(context context.Context, client client.Client, gw *gwv1.Gateway, routes []RouteDescriptor) (map[int][]RouteDescriptor, error)
}

var _ ListenerToRouteMapper = &listenerToRouteMapper{}

type listenerToRouteMapper struct {
}

func (ltr *listenerToRouteMapper) Map(ctx context.Context, k8sclient client.Client, gw *gwv1.Gateway, routes []RouteDescriptor) (map[int][]RouteDescriptor, error) {
	result := make(map[int][]RouteDescriptor)

	// Approach is to greedily add as many relevant routes to each listener.
	for _, listener := range gw.Spec.Listeners {
		for _, route := range routes {
			allowedAttachment, err := ltr.listenerAllowsAttachment(ctx, k8sclient, gw, listener, route)
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

func (ltr *listenerToRouteMapper) listenerAllowsAttachment(ctx context.Context, k8sclient client.Client, gw *gwv1.Gateway, listener gwv1.Listener, route RouteDescriptor) (bool, error) {
	namespaceOK, err := ltr.namespaceCheck(ctx, k8sclient, gw, listener, route)
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

func (ltr *listenerToRouteMapper) namespaceCheck(ctx context.Context, k8sclient client.Client, gw *gwv1.Gateway, listener gwv1.Listener, route RouteDescriptor) (bool, error) {
	if listener.AllowedRoutes == nil {
		return gw.Namespace == route.GetRouteNamespace(), nil
	}

	var allowedNamespaces gwv1.FromNamespaces

	if listener.AllowedRoutes.Namespaces == nil || listener.AllowedRoutes.Namespaces.From == nil {
		allowedNamespaces = gwv1.NamespacesFromSame
	} else {
		allowedNamespaces = *listener.AllowedRoutes.Namespaces.From
	}

	switch allowedNamespaces {
	case gwv1.NamespacesFromSame:
		if gw.Namespace != route.GetRouteNamespace() {
			return false, nil
		}
		break
	case gwv1.NamespacesFromSelector:
		if listener.AllowedRoutes.Namespaces.Selector == nil {
			return false, nil
		}
		// This should be executed off the client-go cache, hence we do not need to perform local caching.
		namespaces, err := ltr.getNamespacesFromSelector(ctx, k8sclient, listener.AllowedRoutes.Namespaces.Selector)
		if err != nil {
			return false, err
		}

		if !namespaces.Has(route.GetRouteNamespace()) {
			return false, nil
		}
		break
	case gwv1.NamespacesFromAll:
		// Nothing to check
		break
	default:
		// Unclear what to do in this case, let's try to be best effort and just ignore this value.
		return false, nil
	}

	return false, nil
}

func (ltr *listenerToRouteMapper) kindCheck(listener gwv1.Listener, route RouteDescriptor) bool {

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

func (ltr *listenerToRouteMapper) getNamespacesFromSelector(context context.Context, k8sclient client.Client, selector *metav1.LabelSelector) (sets.Set[string], error) {
	namespaceList := v1.NamespaceList{}

	convertedSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}
	listOpts := client.ListOptions{LabelSelector: convertedSelector}

	err = k8sclient.List(context, &namespaceList, &listOpts)
	if err != nil {
		return nil, err
	}

	namespaces := sets.New[string]()

	for _, ns := range namespaceList.Items {
		namespaces.Insert(ns.Name)
	}

	return namespaces, nil
}
