package listeners

import (
	"github.com/aws/aws-sdk-go/service/elbv2"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/coreos/alb-ingress-controller/pkg/alb/listener"
	"github.com/coreos/alb-ingress-controller/pkg/alb/rule"
	"github.com/coreos/alb-ingress-controller/pkg/alb/rules"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroups"
	"github.com/coreos/alb-ingress-controller/pkg/annotations"
	albelbv2 "github.com/coreos/alb-ingress-controller/pkg/aws/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
)

// Listeners is a slice of Listener pointers
type Listeners []*listener.Listener

// Find returns the position of the listener, returning -1 if unfound.
func (ls Listeners) Find(l *elbv2.Listener) int {
	for p, v := range ls {
		if !v.NeedsModification(l) {
			return p
		}
	}
	return -1
}

// Reconcile kicks off the state synchronization for every Listener in this Listeners instances.
// TODO: function has changed a lot, test
func (ls Listeners) Reconcile(rOpts *ReconcileOptions) (Listeners, error) {
	output := ls
	if len(ls) < 1 {
		return nil, nil
	}

	for _, l := range ls {
		lOpts := &listener.ReconcileOptions{
			Eventf:          rOpts.Eventf,
			LoadBalancerArn: rOpts.LoadBalancerArn,
			TargetGroups:    rOpts.TargetGroups,
		}
		if err := l.Reconcile(lOpts); err != nil {
			return nil, err
		}

		rsOpts := &rules.ReconcileOptions{
			Eventf:       rOpts.Eventf,
			ListenerArn:  l.Current.ListenerArn,
			TargetGroups: rOpts.TargetGroups,
		}
		if rs, err := l.Rules.Reconcile(rsOpts); err != nil {
			return nil, err
		} else {
			l.Rules = rs
		}
		if !l.Deleted {
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
		l.StripCurrentState()
	}
}

// NewCurrentListeners returns a new listeners.Listeners based on an elbv2.Listeners.
func NewCurrentListeners(listeners []*elbv2.Listener, logger *log.Logger) (Listeners, error) {
	var output Listeners

	for _, l := range listeners {
		logger.Infof("Fetching Rules for Listener %s", *l.ListenerArn)
		rs, err := albelbv2.ELBV2svc.DescribeRules(&elbv2.DescribeRulesInput{ListenerArn: l.ListenerArn})
		if err != nil {
			return nil, err
		}

		newListener := listener.NewCurrentListener(l, logger)

		for _, r := range rs.Rules {
			logger.Debugf("Assembling rule for: %s", log.Prettify(r.Conditions))
			newRule := rule.NewCurrentRule(r, logger)

			newListener.Rules = append(newListener.Rules, newRule)
		}

		output = append(output, newListener)
	}
	return output, nil
}

type NewDesiredListenersOptions struct {
	Ingress     *extensions.Ingress
	Listeners   Listeners
	Annotations *annotations.Annotations
	Logger      *log.Logger
	Priority    int
}

func NewDesiredListeners(o *NewDesiredListenersOptions) (Listeners, error) {
	var output Listeners

	// Generate a listener for each port in the annotations
	for _, port := range o.Annotations.Ports {
		// Track down the existing listener for this port
		var thisListener *listener.Listener
		for _, l := range o.Listeners {
			if *l.Current.Port == port {
				thisListener = l
			}
		}

		newListener := listener.NewDesiredListener(&listener.NewDesiredListenerOptions{
			Port:           port,
			CertificateArn: o.Annotations.CertificateArn,
			Logger:         o.Logger,
		})

		if thisListener != nil {
			thisListener.Desired = newListener.Desired
			newListener = thisListener
		}

		for _, rule := range o.Ingress.Spec.Rules {
			var err error

			newListener.Rules, err = rules.NewDesiredRules(&rules.NewDesiredRulesOptions{
				Logger:        o.Logger,
				ListenerRules: newListener.Rules,
				Rule:          &rule,
			})
			if err != nil {
				return nil, err
			}

		}
		output = append(output, newListener)
	}

	return output, nil
}

type ReconcileOptions struct {
	Eventf          func(string, string, string, ...interface{})
	LoadBalancerArn *string
	TargetGroups    targetgroups.TargetGroups
}
