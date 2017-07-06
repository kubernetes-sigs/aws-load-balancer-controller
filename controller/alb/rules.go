package alb

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

// Rules contains a slice of Rules
type Rules []*Rule

// Reconcile kicks off the state synchronization for every Rule in this Rules slice.
func (r Rules) Reconcile(lb *LoadBalancer, l *Listener) error {

	for _, rule := range r {
		if err := rule.Reconcile(lb, l); err != nil {
			return err
		}
		if rule.deleted {
			i := l.Rules.Find(rule.CurrentRule)
			if i < 0 {
				return fmt.Errorf("Failed to locate rule: %s", rule)
			}
			l.Rules = append(l.Rules[:i], l.Rules[i+1:]...)
		}
	}

	return nil
}

// Find returns the position in the Rules slice of the rule parameter
func (r Rules) Find(rule *elbv2.Rule) int {
	for p, v := range r {
		if v.Equals(rule) {
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
