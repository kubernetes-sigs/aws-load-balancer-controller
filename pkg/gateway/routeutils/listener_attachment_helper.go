package routeutils

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// listenerAttachmentHelper is an internal utility interface that can be used to determine if a listener will allow
// a route to attach to it.
type listenerAttachmentHelper interface {
	listenerAllowsAttachment(ctx context.Context, gw gwv1.Gateway, listener gwv1.Listener, route preLoadRouteDescriptor, deferredRouteReconciler RouteReconciler) (bool, error)
}

var _ listenerAttachmentHelper = &listenerAttachmentHelperImpl{}

// listenerAttachmentHelperImpl implements the listenerAttachmentHelper interface.
type listenerAttachmentHelperImpl struct {
	namespaceSelector namespaceSelector
	logger            logr.Logger
}

func newListenerAttachmentHelper(k8sClient client.Client, logger logr.Logger) listenerAttachmentHelper {
	return &listenerAttachmentHelperImpl{
		namespaceSelector: newNamespaceSelector(k8sClient),
		logger:            logger,
	}
}

// listenerAllowsAttachment utility method to determine if a listener will allow a route to connect using
// Gateway API rules to determine compatibility between lister and route.
func (attachmentHelper *listenerAttachmentHelperImpl) listenerAllowsAttachment(ctx context.Context, gw gwv1.Gateway, listener gwv1.Listener, route preLoadRouteDescriptor, deferredRouteReconciler RouteReconciler) (bool, error) {
	// check namespace
	namespaceOK, err := attachmentHelper.namespaceCheck(ctx, gw, listener, route)
	if err != nil {
		return false, err
	}
	if !namespaceOK {
		deferredRouteReconciler.Enqueue(
			GenerateRouteData(false, true, string(gwv1.RouteReasonNotAllowedByListeners), RouteStatusInfoRejectedMessageNamespaceNotMatch, route.GetRouteNamespacedName(), route.GetRouteKind(), route.GetRouteGeneration(), gw),
		)

		return false, nil
	}

	// check kind
	kindOK := attachmentHelper.kindCheck(listener, route)
	if !kindOK {
		deferredRouteReconciler.Enqueue(
			GenerateRouteData(false, true, string(gwv1.RouteReasonNotAllowedByListeners), RouteStatusInfoRejectedMessageKindNotMatch, route.GetRouteNamespacedName(), route.GetRouteKind(), route.GetRouteGeneration(), gw),
		)
		return false, nil
	}

	// check hostname
	if (route.GetRouteKind() == HTTPRouteKind || route.GetRouteKind() == GRPCRouteKind || route.GetRouteKind() == TLSRouteKind) && route.GetHostnames() != nil {
		hostnameOK, err := attachmentHelper.hostnameCheck(listener, route)
		if err != nil {
			return false, err
		}
		if !hostnameOK {
			// hostname is not ok, print out gwName and gwNamespace test-gw-alb gateway-alb
			deferredRouteReconciler.Enqueue(
				GenerateRouteData(false, true, string(gwv1.RouteReasonNoMatchingListenerHostname), RouteStatusInfoRejectedMessageNoMatchingHostname, route.GetRouteNamespacedName(), route.GetRouteKind(), route.GetRouteGeneration(), gw),
			)
			return false, nil
		}
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

	var allowedRoutes sets.Set[RouteKind]

	/*
		...
			When unspecified or empty, the kinds of Routes
			selected are determined using the Listener protocol.
		...
	*/
	if listener.AllowedRoutes == nil || listener.AllowedRoutes.Kinds == nil || len(listener.AllowedRoutes.Kinds) == 0 {
		allowedRoutes = sets.New[RouteKind](defaultProtocolToRouteKindMap[listener.Protocol])
	} else {
		// TODO - Not sure how to handle versioning (correctly) here.
		// So going to ignore the group checks for now :x
		allowedRoutes = sets.New[RouteKind]()
		for _, v := range listener.AllowedRoutes.Kinds {
			allowedRoutes.Insert(RouteKind(v.Kind))
		}
	}
	return allowedRoutes.Has(route.GetRouteKind())
}

func (attachmentHelper *listenerAttachmentHelperImpl) hostnameCheck(listener gwv1.Listener, route preLoadRouteDescriptor) (bool, error) {
	// A route can attach to listener if it does not have hostname or listener does not have hostname
	if listener.Hostname == nil || len(route.GetHostnames()) == 0 {
		return true, nil
	}

	// validate listener hostname, return if listener hostname is not valid
	isListenerHostnameValid, err := IsHostNameInValidFormat(string(*listener.Hostname))
	if err != nil {
		attachmentHelper.logger.Error(err, "listener hostname is not valid", "listener", listener.Name, "hostname", *listener.Hostname)
		initialErrorMessage := fmt.Sprintf("listener hostname %s is not valid (listener name %s)", listener.Name, *listener.Hostname)
		return false, wrapError(errors.Errorf("%s", initialErrorMessage), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonUnsupportedValue, nil, nil)
	}

	if !isListenerHostnameValid {
		return false, nil
	}

	for _, hostname := range route.GetHostnames() {
		// validate route hostname, skip invalid hostname
		isHostnameValid, err := IsHostNameInValidFormat(string(hostname))
		if err != nil || !isHostnameValid {
			attachmentHelper.logger.V(1).Info("route hostname is not valid, continue...", "route", route.GetRouteNamespacedName(), "hostname", hostname)
			continue
		}

		// check if two hostnames have overlap (compatible)
		if isHostnameCompatible(string(hostname), string(*listener.Hostname)) {
			return true, nil
		}
	}
	return false, nil
}
