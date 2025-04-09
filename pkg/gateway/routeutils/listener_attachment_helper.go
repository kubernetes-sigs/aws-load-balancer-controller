package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// listenerAttachmentHelper is an internal utility interface that can be used to determine if a listener will allow
// a route to attach to it.
type listenerAttachmentHelper interface {
	listenerAllowsAttachment(ctx context.Context, gw gwv1.Gateway, listener gwv1.Listener, route preLoadRouteDescriptor) (bool, error)
}

var _ listenerAttachmentHelper = &listenerAttachmentHelperImpl{}

// listenerAttachmentHelperImpl implements the listenerAttachmentHelper interface.
type listenerAttachmentHelperImpl struct {
	namespaceSelector namespaceSelector
}

func newListenerAttachmentHelper(k8sClient client.Client) listenerAttachmentHelper {
	return &listenerAttachmentHelperImpl{
		namespaceSelector: newNamespaceSelector(k8sClient),
	}
}

// listenerAllowsAttachment utility method to determine if a listener will allow a route to connect using
// Gateway API rules to determine compatibility between lister and route.
func (attachmentHelper *listenerAttachmentHelperImpl) listenerAllowsAttachment(ctx context.Context, gw gwv1.Gateway, listener gwv1.Listener, route preLoadRouteDescriptor) (bool, error) {
	namespaceOK, err := attachmentHelper.namespaceCheck(ctx, gw, listener, route)
	if err != nil {
		return false, err
	}

	if !namespaceOK {
		return false, nil
	}

	if !attachmentHelper.kindCheck(listener, route) {
		return false, nil
	}
	return true, nil
}

// namespaceCheck namespace check implements the Gateway API spec for namespace matching between listener
// and route to determine compatibility.
func (attachmentHelper *listenerAttachmentHelperImpl) namespaceCheck(ctx context.Context, gw gwv1.Gateway, listener gwv1.Listener, route preLoadRouteDescriptor) (bool, error) {
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
		namespaces, err := attachmentHelper.namespaceSelector.getNamespacesFromSelector(ctx, listener.AllowedRoutes.Namespaces.Selector)
		if err != nil {
			return false, err
		}

		if !namespaces.Has(namespacedName.Namespace) {
			return false, nil
		}
		return true, nil
	default:
		// Unclear what to do in this case, we'll just filter out this route.
		return false, nil
	}
}

// kindCheck kind check implements the Gateway API spec for kindCheck matching between listener
// and route to determine compatibility.
func (attachmentHelper *listenerAttachmentHelperImpl) kindCheck(listener gwv1.Listener, route preLoadRouteDescriptor) bool {

	var allowedRoutes sets.Set[string]

	/*
		...
			When unspecified or empty, the kinds of Routes
			selected are determined using the Listener protocol.
		...
	*/
	if listener.AllowedRoutes == nil || listener.AllowedRoutes.Kinds == nil || len(listener.AllowedRoutes.Kinds) == 0 {
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
