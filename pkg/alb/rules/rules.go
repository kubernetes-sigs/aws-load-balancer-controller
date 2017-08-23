package rules

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/awsutil"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/coreos/alb-ingress-controller/pkg/alb/rule"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroups"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
)

// Rules contains a slice of Rules
type Rules []*rule.Rule

type NewDesiredRulesOptions struct {
	Priority      int
	Logger        *log.Logger
	ListenerRules Rules
	Rule          *extensions.IngressRule
}

// NewDesiredRules returns a Rules created by appending the IngressRule paths to a ListenerRules.
// The returned priority is the highest priority added to the rules list.
func NewDesiredRules(o *NewDesiredRulesOptions) (Rules, int, error) {
	rs := o.ListenerRules

	if len(o.Rule.HTTP.Paths) == 0 {
		return nil, 0, fmt.Errorf("Ingress doesn't have any paths defined. This is not a very good ingress.")
	}

	// If there are no pre-existing rules on the listener, inject a default rule.
	// Since the Kubernetes ingress has no notion of this, we pick the first backend.
	if o.Priority == 0 {
		r := rule.NewDesiredRule(&rule.NewDesiredRuleOptions{
			Priority: o.Priority,
			Hostname: o.Rule.Host,
			Path:     o.Rule.HTTP.Paths[0].Path,
			SvcName:  o.Rule.HTTP.Paths[0].Backend.ServiceName,
			Logger:   o.Logger,
		})
		if !rs.merge(r) {
			rs = append(rs, r)
		}
		o.Priority++
	}

	for _, path := range o.Rule.HTTP.Paths {
		r := rule.NewDesiredRule(&rule.NewDesiredRuleOptions{
			Priority: o.Priority,
			Hostname: o.Rule.Host,
			Path:     path.Path,
			SvcName:  path.Backend.ServiceName,
			Logger:   o.Logger,
		})
		if !rs.merge(r) {
			rs = append(rs, r)
		}
		o.Priority++
	}

	return rs, o.Priority, nil
}

func (rs Rules) merge(r *rule.Rule) bool {
	if i := rs.FindByPriority(r.Desired.Priority); i >= 0 {
		rs[i].Desired = r.Desired
		return true
	}
	return false
}

// Reconcile kicks off the state synchronization for every Rule in this Rules slice.
func (rs Rules) Reconcile(rsOpts *ReconcileOptions) (Rules, error) {
	var output Rules

	rOpts := &rule.ReconcileOptions{
		Eventf:       rsOpts.Eventf,
		ListenerArn:  rsOpts.ListenerArn,
		TargetGroups: rsOpts.TargetGroups,
	}

	for _, r := range rs {
		if err := r.Reconcile(rOpts); err != nil {
			return nil, err
		}
		if !r.Deleted {
			output = append(output, r)
		}
	}

	return output, nil
}

// FindByPriority returns the position in the Rules slice of the rule parameter
func (rs Rules) FindByPriority(priority *string) int {
	for p, v := range rs {
		if v.Current == nil {
			continue
		}
		if awsutil.DeepEqual(v.Current.Priority, priority) {
			return p
		}
	}
	return -1
}

// StripDesiredState removes the desired state from all Rules in the slice.
func (rs Rules) StripDesiredState() {
	for _, r := range rs {
		r.StripDesiredState()
	}
}

// StripCurrentState removes the current statefrom all Rule instances.
func (rs Rules) StripCurrentState() {
	for _, r := range rs {
		r.StripCurrentState()
	}
}

type ReconcileOptions struct {
	Eventf        func(string, string, string, ...interface{})
	ListenerArn   *string
	ListenerRules *Rules
	TargetGroups  targetgroups.TargetGroups
}
