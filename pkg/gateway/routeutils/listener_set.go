package routeutils

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type handshakeState string

const (
	// acceptedHandshakeState - both resource and gateway agree to attachement
	acceptedHandshakeState handshakeState = "accepted"
	// gatewayRejectedHandshakeState - the gateway configuration rejects this configuration
	gatewayRejectedHandshakeState handshakeState = "rejected"
	// irrelevantResourceHandshakeState - the resource has no relation to the gateway
	irrelevantResourceHandshakeState handshakeState = "irrelevant"
)

type listenerSetLoader interface {
	retrieveListenersFromListenerSets(ctx context.Context, gateway gwv1.Gateway) (listenerSetLoadResult, []*gwv1.ListenerSet, error)
}

type listenerSetLoaderImpl struct {
	k8sClient         client.Client
	namespaceSelector namespaceSelector
	logger            logr.Logger
}

func newListenerSetLoader(k8sClient client.Client, logger logr.Logger) listenerSetLoader {
	return &listenerSetLoaderImpl{
		k8sClient:         k8sClient,
		namespaceSelector: newNamespaceSelector(k8sClient),
		logger:            logger,
	}
}

type listenerSetLoadResult struct {
	listenersPerListenerSet map[types.NamespacedName][]listenerSetListenerSource
	acceptedListenerSets    []*gwv1.ListenerSet
}

func (l *listenerSetLoaderImpl) retrieveListenersFromListenerSets(ctx context.Context, gateway gwv1.Gateway) (listenerSetLoadResult, []*gwv1.ListenerSet, error) {
	listenerSets := &gwv1.ListenerSetList{}
	err := l.k8sClient.List(ctx, listenerSets)
	if err != nil {
		return listenerSetLoadResult{}, nil, err
	}

	rejectedListenerSets := make([]*gwv1.ListenerSet, 0)

	listenersPerListenerSet := make(map[types.NamespacedName][]listenerSetListenerSource)
	acceptedListenerSets := make([]*gwv1.ListenerSet, 0)
	for i, item := range listenerSets.Items {
		handshake, err := l.listenerSetGatewayHandshake(ctx, item, gateway)
		if err != nil {
			return listenerSetLoadResult{}, nil, err
		}
		switch handshake {
		case acceptedHandshakeState:
			var convertedListeners []listenerSetListenerSource
			for _, listener := range item.Spec.Listeners {
				convertedListeners = append(convertedListeners, l.convertListenerSetListenerToGatewayListener(item, listener))
			}
			itemPtr := &listenerSets.Items[i]
			listenersPerListenerSet[k8s.NamespacedName(itemPtr)] = convertedListeners
			acceptedListenerSets = append(acceptedListenerSets, itemPtr)
			break
		case gatewayRejectedHandshakeState:
			rejectedListenerSets = append(rejectedListenerSets, &listenerSets.Items[i])
			break
		case irrelevantResourceHandshakeState:
			// Nothing to do here, the listener set and gateway have no relation.
			break
		}

	}

	return listenerSetLoadResult{
		listenersPerListenerSet: listenersPerListenerSet,
		acceptedListenerSets:    acceptedListenerSets,
	}, rejectedListenerSets, nil
}

func (l *listenerSetLoaderImpl) listenerSetGatewayHandshake(ctx context.Context, listenerSet gwv1.ListenerSet, gw gwv1.Gateway) (handshakeState, error) {
	// Check if ListenerSet is requesting attachment to this Gateway.
	attach := doesResourceAttachToGateway(l.convertListenerSetParentRef(listenerSet.Spec.ParentRef), listenerSet.Namespace, gw)
	if !attach {
		return irrelevantResourceHandshakeState, nil
	}

	var allowedNamespaces gwv1.FromNamespaces
	var labelSelector *metav1.LabelSelector
	if gw.Spec.AllowedListeners == nil || gw.Spec.AllowedListeners.Namespaces == nil || gw.Spec.AllowedListeners.Namespaces.From == nil {
		allowedNamespaces = gwv1.NamespacesFromNone
	} else {
		allowedNamespaces = *gw.Spec.AllowedListeners.Namespaces.From
		labelSelector = gw.Spec.AllowedListeners.Namespaces.Selector
	}

	// Getting here means that the ListenerSet has requested attachment, we need to check if Gateway allows it.
	allowed, err := doesResourceAllowNamespace(ctx, allowedNamespaces, labelSelector, l.namespaceSelector, listenerSet.Namespace, gw)
	if err != nil {
		return gatewayRejectedHandshakeState, err
	}

	if allowed {
		return acceptedHandshakeState, nil
	}

	return gatewayRejectedHandshakeState, nil
}

func (l *listenerSetLoaderImpl) convertListenerSetParentRef(ref gwv1.ParentGatewayReference) gwv1.ParentReference {
	return gwv1.ParentReference{
		Group:     ref.Group,
		Kind:      ref.Kind,
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}
}

func (l *listenerSetLoaderImpl) convertListenerSetListenerToGatewayListener(listenerSet gwv1.ListenerSet, entry gwv1.ListenerEntry) listenerSetListenerSource {
	v := gwv1.Listener{
		Name:          entry.Name,
		Hostname:      entry.Hostname,
		Port:          entry.Port,
		Protocol:      entry.Protocol,
		TLS:           entry.TLS,
		AllowedRoutes: entry.AllowedRoutes,
	}
	return listenerSetListenerSource{
		parentRef: listenerSet,
		listener:  v,
	}
}

var _ listenerSetLoader = &listenerSetLoaderImpl{}
