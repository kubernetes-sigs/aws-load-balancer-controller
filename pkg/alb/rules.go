package alb

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/elbv2"
	awsutil "github.com/coreos/alb-ingress-controller/pkg/util/aws"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	extensions "k8s.io/api/extensions/v1beta1"
)

// Rules contains a slice of Rules
type Rules []*Rule

// Reconcile kicks off the state synchronization for every Rule in this Rules slice.
func (r Rules) Reconcile(rOpts *ReconcileOptions, l *Listener) error {

	for _, rule := range r {
		if err := rule.Reconcile(rOpts, l); err != nil {
			return err
		}
		if rule.deleted {
			i := l.Rules.FindByPriority(rule.CurrentRule)
			if i < 0 {
				return fmt.Errorf("Failed to locate rule: %s", awsutil.Prettify(rule))
			}
			l.Rules = append(l.Rules[:i], l.Rules[i+1:]...)
		}
	}

	return nil
}

// Find returns the position in the Rules slice of the rule parameter
func (r Rules) FindByPriority(rule *elbv2.Rule) int {
	for p, v := range r {
		if awsutil.DeepEqual(v.CurrentRule.Priority, rule.Priority) {
			return p
		}
	}
	return -1
}

// StripDesiredState removes the DesiredListener from all Rules in the slice.
func (r Rules) StripDesiredState() {
	for _, rule := range r {
		rule.DesiredRule = nil
	}
}

// StripCurrentState removes the CurrentRule reference from all Rule instances. Most commonly used
// when the Listener it related to has been deleted.
func (r Rules) StripCurrentState() {
	for _, rule := range r {
		rule.CurrentRule = nil
	}
}

type NewRulesFromIngressOptions struct {
	Hostname string
	Logger   *log.Logger
	Listener *Listener
	Rule     *extensions.IngressRule
	Priority int
}

func NewRulesFromIngress(o *NewRulesFromIngressOptions) (Rules, int, error) {
	output := o.Listener.Rules

	for _, path := range o.Rule.HTTP.Paths {
		// Start with a new rule
		rule := NewRule(o.Priority, o.Hostname, path.Path, path.Backend.ServiceName, o.Logger)

		// If this rule is already defined, copy the desired state over
		if i := output.FindByPriority(rule.DesiredRule); i >= 0 {
			output[i].DesiredRule = rule.DesiredRule
		} else {
			output = append(output, rule)
		}
		o.Priority++
	}

	return output, o.Priority, nil
}
