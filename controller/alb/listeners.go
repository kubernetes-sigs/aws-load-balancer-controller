package alb

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
)

// Listeners is a slice of Listener pointers
type Listeners []*Listener

// Find returns the position of the listener, returning -1 if unfound.
func (ls Listeners) Find(listener *elbv2.Listener) int {
	for p, v := range ls {
		if !v.needsModification(listener) {
			return p
		}
	}
	return -1
}

// SyncState kicks of the state synchronization for every Listener in this Listeners instances.
func (ls Listeners) SyncState(lb *LoadBalancer, tgs *TargetGroups) Listeners {
	// TODO: We currently only support 1 listener. Possibly only 1 TG?  We need logic that can associate specific
	// TargetGroups with specific listeners.
	var listeners Listeners
	if len(ls) < 1 {
		return listeners
	}

	for _, listener := range ls {
		l := listener.SyncState(lb)
		l.Rules = l.Rules.SyncState(lb, l)
		if l != nil && !l.deleted {
			listeners = append(listeners, l)
		}
	}

	return listeners
}

// StripDesiredState removes the DesiredListener from all Listeners in the slice.
func (ls Listeners) StripDesiredState() {
	for _, listener := range ls {
		listener.DesiredListener = nil
	}
}

// StripCurrentState takes all listeners and sets their CurrentListener to nil. Most commonly used
// when an ELB must be re-created fully. When the deletion of the ELB occurs, the listeners attached
// are also deleted, thus the ingress controller must know they no longer exist.
//
// Additionally, since Rules are also removed its Listener is, this also calles StripDesiredState on
// the Rules attached to each listener.
func (ls Listeners) StripCurrentState() {
	for _, listener := range ls {
		listener.CurrentListener = nil
		listener.Rules.StripCurrentState()
	}
}
