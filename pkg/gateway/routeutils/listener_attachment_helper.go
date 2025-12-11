package routeutils

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// listenerAttachmentHelper is an internal utility interface that can be used to determine if a listener will allow
// a route to attach to it.
type listenerAttachmentHelper interface {
	listenerAllowsAttachment(ctx context.Context, gw gwv1.Gateway, listener gwv1.Listener, route preLoadRouteDescriptor, matchedParentRef *gwv1.ParentReference, hostnamesFromHttpRoutes map[types.NamespacedName][]gwv1.Hostname, hostnamesFromGrpcRoutes map[types.NamespacedName][]gwv1.Hostname) ([]gwv1.Hostname, bool, *RouteData, error)
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
// Returns: (compatibleHostnames, allowed, failedRouteData, error)
func (attachmentHelper *listenerAttachmentHelperImpl) listenerAllowsAttachment(ctx context.Context, gw gwv1.Gateway, listener gwv1.Listener, route preLoadRouteDescriptor, matchedParentRef *gwv1.ParentReference, hostnamesFromHttpRoutes map[types.NamespacedName][]gwv1.Hostname, hostnamesFromGrpcRoutes map[types.NamespacedName][]gwv1.Hostname) ([]gwv1.Hostname, bool, *RouteData, error) {
	port := matchedParentRef.Port
	sectionName := matchedParentRef.SectionName
	// check namespace
	namespaceOK, err := attachmentHelper.namespaceCheck(ctx, gw, listener, route)
	if err != nil {
		return nil, false, nil, err
	}
	if !namespaceOK {
		rd := GenerateRouteData(false, true, string(gwv1.RouteReasonNotAllowedByListeners), RouteStatusInfoRejectedMessageNamespaceNotMatch, route.GetRouteNamespacedName(), route.GetRouteKind(), route.GetRouteGeneration(), gw, port, sectionName)
		return nil, false, &rd, nil
	}

	// check kind
	kindOK := attachmentHelper.kindCheck(listener, route)
	if !kindOK {
		rd := GenerateRouteData(false, true, string(gwv1.RouteReasonNotAllowedByListeners), RouteStatusInfoRejectedMessageKindNotMatch, route.GetRouteNamespacedName(), route.GetRouteKind(), route.GetRouteGeneration(), gw, port, sectionName)
		return nil, false, &rd, nil
	}

	// check hostname and get compatible hostnames
	var compatibleHostnames []gwv1.Hostname
	if route.GetRouteKind() == HTTPRouteKind || route.GetRouteKind() == GRPCRouteKind || route.GetRouteKind() == TLSRouteKind {
		var hostnameOK bool
		compatibleHostnames, hostnameOK, err = attachmentHelper.hostnameCheck(listener, route)
		if err != nil {
			return nil, false, nil, err
		}
		if !hostnameOK {
			rd := GenerateRouteData(false, true, string(gwv1.RouteReasonNoMatchingListenerHostname), RouteStatusInfoRejectedMessageNoMatchingHostname, route.GetRouteNamespacedName(), route.GetRouteKind(), route.GetRouteGeneration(), gw, port, sectionName)
			return nil, false, &rd, nil
		}
	}

	// check cross serving hostname uniqueness
	if route.GetRouteKind() == HTTPRouteKind || route.GetRouteKind() == GRPCRouteKind {
		hostnameUniquenessOK, conflictRoute := attachmentHelper.crossServingHostnameUniquenessCheck(route, hostnamesFromHttpRoutes, hostnamesFromGrpcRoutes)
		if !hostnameUniquenessOK {
			message := fmt.Sprintf("HTTPRoute and GRPCRoute have overlap hostname, attachment is rejected. Conflict route: %s", conflictRoute)
			rd := GenerateRouteData(false, true, string(gwv1.RouteReasonNotAllowedByListeners), message, route.GetRouteNamespacedName(), route.GetRouteKind(), route.GetRouteGeneration(), gw, port, sectionName)
			return nil, false, &rd, nil
		}
	}

	return compatibleHostnames, true, nil, nil
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
		allowedRoutes = sets.New[RouteKind](DefaultProtocolToRouteKindMap[listener.Protocol]...)
	} else {
		// TODO - Not sure how to handle versioning (correctly) here.
		// So going to ignore the group checks for now :x
		allowedRoutes = sets.New[RouteKind]()
		for _, v := range listener.AllowedRoutes.Kinds {
			allowedRoutes.Insert(RouteKind(v.Kind))
		}
	}

	isAllowed := allowedRoutes.Has(route.GetRouteKind())

	if !isAllowed {
		return false
	}

	if listener.Protocol == gwv1.TLSProtocolType {

		var tlsMode *gwv1.TLSModeType

		if listener.TLS != nil && listener.TLS.Mode != nil {
			tlsMode = listener.TLS.Mode
		}
		switch route.GetRouteKind() {
		case TCPRouteKind:
			// Listener must allow termination at lb
			return tlsMode == nil || *tlsMode == gwv1.TLSModeTerminate
		case TLSRouteKind:
			// This is kind of different.
			// For AWS NLB, the original TLS will be terminated, however
			// the LB will establish a new TLS connection to the backend.
			// Users that want to persist the same TLS connection should use TCP
			return tlsMode != nil && *tlsMode == gwv1.TLSModePassthrough
		}
		// Unsupported route type.
		return false
	}

	return true
}

func (attachmentHelper *listenerAttachmentHelperImpl) hostnameCheck(listener gwv1.Listener, route preLoadRouteDescriptor) ([]gwv1.Hostname, bool, error) {
	// If route has no hostnames but listener does, use listener hostname
	if route.GetHostnames() == nil || len(route.GetHostnames()) == 0 {
		if listener.Hostname != nil {
			return []gwv1.Hostname{*listener.Hostname}, true, nil
		}
		return nil, true, nil
	}

	// If listener has no hostname, route can attach
	if listener.Hostname == nil {
		return nil, true, nil
	}

	// validate listener hostname, return if listener hostname is not valid
	isListenerHostnameValid, err := IsHostNameInValidFormat(string(*listener.Hostname))
	if err != nil {
		attachmentHelper.logger.Error(err, "listener hostname is not valid", "listener", listener.Name, "hostname", *listener.Hostname)
		initialErrorMessage := fmt.Sprintf("listener hostname %s is not valid (listener name %s)", listener.Name, *listener.Hostname)
		return nil, false, wrapError(errors.Errorf("%s", initialErrorMessage), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonUnsupportedValue, nil, nil)
	}

	if !isListenerHostnameValid {
		return nil, false, nil
	}

	compatibleHostnames := []gwv1.Hostname{}
	for _, hostname := range route.GetHostnames() {
		// validate route hostname, skip invalid hostname
		isHostnameValid, err := IsHostNameInValidFormat(string(hostname))
		if err != nil || !isHostnameValid {
			attachmentHelper.logger.V(1).Info("route hostname is not valid, continue...", "route", route.GetRouteNamespacedName(), "hostname", hostname)
			continue
		}

		// check if two hostnames have overlap (compatible) and get the more specific one
		if compatible, ok := getCompatibleHostname(string(hostname), string(*listener.Hostname)); ok {
			compatibleHostnames = append(compatibleHostnames, gwv1.Hostname(compatible))
		}
	}

	if len(compatibleHostnames) == 0 {
		return nil, false, nil
	}

	// Return computed compatible hostnames without storing in route
	return compatibleHostnames, true, nil
}

func (attachmentHelper *listenerAttachmentHelperImpl) crossServingHostnameUniquenessCheck(route preLoadRouteDescriptor, hostnamesFromHttpRoutes map[types.NamespacedName][]gwv1.Hostname, hostnamesFromGrpcRoutes map[types.NamespacedName][]gwv1.Hostname) (bool, string) {
	namespacedName := route.GetRouteNamespacedName()
	hostnames := route.GetHostnames()
	routeKind := route.GetRouteKind()
	var conflictMap map[types.NamespacedName][]gwv1.Hostname

	switch routeKind {
	case GRPCRouteKind:
		hostnamesFromGrpcRoutes[namespacedName] = hostnames
		if len(hostnamesFromHttpRoutes) > 0 {
			conflictMap = hostnamesFromHttpRoutes
		}
	case HTTPRouteKind:
		hostnamesFromHttpRoutes[namespacedName] = hostnames
		if len(hostnamesFromGrpcRoutes) > 0 {
			conflictMap = hostnamesFromGrpcRoutes
		}
	}

	for _, hostname := range hostnames {
		for conflictNamespacedName, conflictHostnames := range conflictMap {
			for _, conflictHostname := range conflictHostnames {
				if isHostnameCompatible(string(hostname), string(conflictHostname)) {
					return false, conflictNamespacedName.String()
				}
			}
		}
	}
	return true, ""
}
