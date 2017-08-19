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
	Logger        *log.Logger
	ListenerRules Rules
	Rule          *extensions.IngressRule
}

// NewDesiredRules returns a Rules created by appending the IngressRule paths to a ListenerRules.
func NewDesiredRules(o *NewDesiredRulesOptions) (Rules, error) {
	output := o.ListenerRules
	nextpriority := len(o.ListenerRules)

	if len(o.Rule.HTTP.Paths) == 0 {
		return nil, fmt.Errorf("Ingress doesn't have any paths defined. This is not a very good ingress.")
	}

	// If there are no pre-existing rules on the listener, inject a default rule.
	// Since the Kubernetes ingress has no notion of this, we pick the first backend.
	if nextpriority == 0 {
		r := rule.NewDesiredRule(0, o.Rule.Host, o.Rule.HTTP.Paths[0].Path, o.Rule.HTTP.Paths[0].Backend.ServiceName, o.Logger)
		output = append(output, r)
		nextpriority++
	}

	for _, path := range o.Rule.HTTP.Paths {
		r := rule.NewDesiredRule(nextpriority, o.Rule.Host, path.Path, path.Backend.ServiceName, o.Logger)
		output = append(output, r)
		nextpriority++
	}

	return output, nil
}

// Reconcile kicks off the state synchronization for every Rule in this Rules slice.
func (rs Rules) Reconcile(rOpts *ReconcileOptions) (Rules, error) {
	var output Rules

	for _, r := range rs {
		rOpts := &rule.ReconcileOptions{
			Eventf:       rOpts.Eventf,
			ListenerArn:  rOpts.ListenerArn,
			TargetGroups: rOpts.TargetGroups,
		}
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
