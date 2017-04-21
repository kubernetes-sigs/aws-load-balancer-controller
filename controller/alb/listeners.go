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

// Reconcile kicks off the state synchronization for every Listener in this Listeners instances.
func (ls Listeners) Reconcile(lb *LoadBalancer, tgs *TargetGroups) error {
	if len(ls) < 1 {
		return nil
	}

	newListenerList := ls

	for i, listener := range ls {
		if err := listener.Reconcile(lb); err != nil {
			return err
		}
		if err := listener.Rules.Reconcile(lb, listener); err != nil {
			return err
		}
		if listener.deleted {
			// TODO: without this check, you'll get an index out of range exception
			// during a full ALB deletion. Shouldn't have to do this check... This its
			// related to https://github.com/coreos/alb-ingress-controller/issues/25.
			if i > len(newListenerList)-1 {
				return nil
			}

			newListenerList = append(newListenerList[:i], newListenerList[i+1:]...)
		}
	}

	lb.Listeners = newListenerList
	return nil
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
