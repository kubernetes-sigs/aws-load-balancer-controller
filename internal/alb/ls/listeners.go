package ls

import (
	"context"

	"github.com/aws/aws-sdk-go/service/elbv2"
	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
)

// Reconcile kicks off the state synchronization for every Listener in this Listeners instances.
func (ls Listeners) Reconcile(ctx context.Context, rOpts *ReconcileOptions) (Listeners, error) {
	output := Listeners{}

	for _, l := range ls {
		if err := l.Reconcile(ctx, rOpts); err != nil {
			return nil, err
		}

		if !l.deleted {
			output = append(output, l)
		}
	}

	return output, nil
}

// StripDesiredState removes the DesiredListener from all Listeners in the slice.
func (ls Listeners) StripDesiredState() {
	for _, l := range ls {
		l.StripDesiredState()
	}
}

// StripCurrentState takes all listeners and sets their CurrentListener to nil. Most commonly used
// when an ELB must be re-created fully. When the deletion of the ELB occurs, the listeners attached
// are also deleted, thus the ingress controller must know they no longer exist.
//
// Additionally, since Rules are also removed its Listener is, this also calles StripDesiredState on
// the Rules attached to each listener.
func (ls Listeners) StripCurrentState() {
	for _, l := range ls {
		l.stripCurrentState()
	}
}

type NewCurrentListenersOptions struct {
	TargetGroups tg.TargetGroups
	Listeners    []*elbv2.Listener
}

// NewCurrentListeners returns a new listeners.Listeners based on an elbv2.Listeners.
func NewCurrentListeners(o *NewCurrentListenersOptions) (Listeners, error) {
	var output Listeners

	for _, l := range o.Listeners {
		newListener, err := NewCurrentListener(&NewCurrentListenerOptions{
			Listener:     l,
			TargetGroups: o.TargetGroups,
		})
		if err != nil {
			return nil, err
		}

		output = append(output, newListener)
	}
	return output, nil
}

type NewDesiredListenersOptions struct {
	Ingress           *extensions.Ingress
	Store             store.Storer
	ExistingListeners Listeners
	TargetGroups      tg.TargetGroups
	Priority          int
}

func NewDesiredListeners(o *NewDesiredListenersOptions) (Listeners, error) {
	var output Listeners

	annos, err := o.Store.GetIngressAnnotations(k8s.MetaNamespaceKey(o.Ingress))
	if err != nil {
		return nil, err
	}

	// Generate a listener for each port in the annotations
	for _, port := range annos.LoadBalancer.Ports {
		// Track down the existing listener for this port
		var thisListener *Listener
		for _, l := range o.ExistingListeners {
			// This condition should not be possible, but we've seen some strange behavior
			// where listeners exist and are missing their current state.
			if l.ls.current == nil {
				continue
			}
			if *l.ls.current.Port == port.Port {
				thisListener = l
			}
		}

		newListener, err := NewDesiredListener(&NewDesiredListenerOptions{
			Port:             port,
			CertificateArn:   annos.Listener.CertificateArn,
			SslPolicy:        annos.Listener.SslPolicy,
			Ingress:          o.Ingress,
			Store:            o.Store,
			TargetGroups:     o.TargetGroups,
			ExistingListener: thisListener,
		})
		if err != nil {
			return nil, err
		}

		output = append(output, newListener)
	}

	// Generate a listener for each existing known port that is no longer annotated
	// representing it needs to be deleted
	for _, l := range o.ExistingListeners {
		exists := false
		for _, port := range annos.LoadBalancer.Ports {
			if l.ls.current == nil {
				continue
			}
			if *l.ls.current.Port == port.Port {
				exists = true
				break
			}
		}

		if !exists {
			output = append(output, &Listener{ls: ls{current: l.ls.current}})
		}
	}

	return output, nil
}
