package listeners

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/awsutil"
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
		if !v.NeedsModificationCheck(l) {
			return p
		}
	}
	return -1
}

// Reconcile kicks off the state synchronization for every Listener in this Listeners instances.
// TODO: function has changed a lot, test
func (ls Listeners) Reconcile(rOpts *ReconcileOptions) (Listeners, error) {
	output := Listeners{}
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

		if l.Current != nil {
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

type NewCurrentListenersOptions struct {
	TargetGroups *targetgroups.TargetGroups
	Listeners    []*elbv2.Listener
	Logger       *log.Logger
}

// NewCurrentListeners returns a new listeners.Listeners based on an elbv2.Listeners.
func NewCurrentListeners(o *NewCurrentListenersOptions) (Listeners, error) {
	var output Listeners

	for _, l := range o.Listeners {
		o.Logger.Infof("Fetching Rules for Listener %s", *l.ListenerArn)
		rs, err := albelbv2.ELBV2svc.DescribeRules(&elbv2.DescribeRulesInput{ListenerArn: l.ListenerArn})
		if err != nil {
			return nil, err
		}

		newListener := listener.NewCurrentListener(&listener.NewCurrentListenerOptions{
			Listener: l,
			Logger:   o.Logger,
		})

		for _, r := range rs.Rules {
			// TODO LOOKUP svcName based on TG
			i, tg := o.TargetGroups.FindCurrentByARN(*r.Actions[0].TargetGroupArn)
			if i < 0 {
				return nil, fmt.Errorf("Failed to find a target group associated with a rule. This should not be possible. Rule: %s.", awsutil.Prettify(r.RuleArn))
			}
			o.Logger.Debugf("Assembling rule for: %s", log.Prettify(r.Conditions))
			newRule := rule.NewCurrentRule(&rule.NewCurrentRuleOptions{
				SvcName: tg.SvcName,
				Rule:    r,
				Logger:  o.Logger,
			})

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
			// This condition should not be possible, but we've seen some strange behavior
			// where listeners exist and are missing their current state.
			if l.Current == nil {
				continue
			}
			if *l.Current.Port == port.Port {
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

		var p int
		for _, rule := range o.Ingress.Spec.Rules {
			var err error

			newListener.Rules, p, err = rules.NewDesiredRules(&rules.NewDesiredRulesOptions{
				Priority:         p,
				Logger:           o.Logger,
				ListenerRules:    newListener.Rules,
				Rule:             &rule,
				IgnoreHostHeader: *o.Annotations.IgnoreHostHeader,
			})
			if err != nil {
				return nil, err
			}
		}
		output = append(output, newListener)
	}

	// Generate a listener for each existing known port that is not longer annotated
	// representing it needs to be deleted
	for _, l := range o.Listeners {
		exists := false
		for _, port := range o.Annotations.Ports {
			if l.Current == nil {
				continue
			}
			if *l.Current.Port == port.Port {
				exists = true
				break
			}
		}

		if !exists {
			output = append(output, &listener.Listener{Current: l.Current, Logger: l.Logger})
		}
	}

	return output, nil
}

type ReconcileOptions struct {
	Eventf          func(string, string, string, ...interface{})
	LoadBalancerArn *string
	TargetGroups    targetgroups.TargetGroups
}
