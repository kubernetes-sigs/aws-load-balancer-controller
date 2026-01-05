package routeutils

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ListenerParentType string

const (
	ListenerParentTypeGateway     = "Gateway"
	ListenerParentTypeListenerSet = "ListenerSet"
)

// DiscoveredListener represents a listener with its index for efficient access
type DiscoveredListener struct {
	Listener   gwv1.Listener
	Parent     interface{}
	ParentType ListenerParentType
}

// discoverListeners discovers available listeners for this Gateway.
func discoverListeners(gw gwv1.Gateway) []DiscoveredListener {
	discoveredListeners := make([]DiscoveredListener, 0)

	for _, listener := range gw.Spec.Listeners {
		dl := DiscoveredListener{
			Listener:   listener,
			Parent:     gw,
			ParentType: ListenerParentTypeGateway,
		}
		discoveredListeners = append(discoveredListeners, dl)
	}

	return discoveredListeners
}
