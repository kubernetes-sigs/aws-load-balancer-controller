package alb

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/config"
	awsutil "github.com/coreos/alb-ingress-controller/pkg/util/aws"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	extensions "k8s.io/api/extensions/v1beta1"
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
func (ls Listeners) Reconcile(rOpts *ReconcileOptions) error {
	if len(ls) < 1 {
		return nil
	}

	newListenerList := ls

	for i, listener := range ls {
		if err := listener.Reconcile(rOpts); err != nil {
			return err
		}
		if err := listener.Rules.Reconcile(rOpts, listener); err != nil {
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

	rOpts.loadbalancer.Listeners = newListenerList
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

// NewListenersFromAWSListeners returns a new alb.Listeners based on an elbv2.Listeners.
func NewListenersFromAWSListeners(listeners []*elbv2.Listener, logger *log.Logger) (Listeners, error) {
	var output Listeners

	for _, listener := range listeners {
		logger.Infof("Fetching Rules for Listener %s", *listener.ListenerArn)
		rules, err := awsutil.ALBsvc.DescribeRules(&elbv2.DescribeRulesInput{ListenerArn: listener.ListenerArn})
		if err != nil {
			return nil, err
		}

		l := NewListenerFromAWSListener(listener, logger)

		for _, rule := range rules.Rules {
			logger.Debugf("Assembling rule for: %s", log.Prettify(rule.Conditions))
			r := NewRuleFromAWSRule(rule, logger)

			l.Rules = append(l.Rules, r)
		}

		output = append(output, l)
	}
	return output, nil
}

type NewListenersFromIngressOptions struct {
	Ingress      *extensions.Ingress
	LoadBalancer *LoadBalancer
	Annotations  *config.Annotations
	Logger       *log.Logger
	Priority     int
}

// TODO: need to carry priority counter across both NewRulesFromIngress and this
func NewListenersFromIngress(o *NewListenersFromIngressOptions) (Listeners, error) {
	var err error
	output := o.LoadBalancer.Listeners
	var priority int

	for _, rule := range o.Ingress.Spec.Rules {
		// Listeners are constructed based on path and port.
		// Start with a new listener
		listenerList := NewListener(o.Annotations, o.Logger)
		hostname := rule.Host

		for _, listener := range listenerList {
			// If this listener is already defined, copy the desired state over
			if i := output.Find(listener.DesiredListener); i >= 0 {
				output[i].DesiredListener = listener.DesiredListener
				listener = output[i]
			} else {
				output = append(output, listener)
			}

			listener.Rules, priority, err = NewRulesFromIngress(&NewRulesFromIngressOptions{
				Hostname: hostname,
				Logger:   o.Logger,
				Listener: listener,
				Rule:     &rule,
				Priority: priority,
			})
			if err != nil {
				return nil, err
			}

		}
	}

	return output, nil
}
