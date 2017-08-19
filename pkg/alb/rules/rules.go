package rules

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/awsutil"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/coreos/alb-ingress-controller/pkg/alb/rule"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroups"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	extensions "k8s.io/api/extensions/v1beta1"
)

// Rules contains a slice of Rules
type Rules []*rule.Rule

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

type NewRulesFromIngressOptions struct {
	Hostname      string
	Logger        *log.Logger
	ListenerRules Rules
	Rule          *extensions.IngressRule
	Priority      int
}

func NewRulesFromIngress(o *NewRulesFromIngressOptions) (Rules, int, error) {
	output := o.ListenerRules

	if len(o.Rule.HTTP.Paths) == 0 {
		return nil, 0, fmt.Errorf("Ingress doesn't have any paths defined. This is not a very good ingress.")
	}

	// // Build the default rule. Since the Kubernetes ingress has no notion of this, we pick the first backend.
	// r := rule.NewRule(0, o.Hostname, o.Rule.HTTP.Paths[0].Path, o.Rule.HTTP.Paths[0].Backend.ServiceName, o.Logger)
	// if i := output.FindByPriority(r.Desired.Priority); i >= 0 {
	// 	output[i].Desired = r.Desired
	// } else {
	// 	output = append(output, r)
	// }

	for _, path := range o.Rule.HTTP.Paths {
		// Start with a new rule
		r := rule.NewRule(o.Priority, o.Hostname, path.Path, path.Backend.ServiceName, o.Logger)

		// If this rule is already defined, copy the desired state over
		if i := output.FindByPriority(r.Desired.Priority); i >= 0 {
			output[i].Desired = r.Desired
		} else {
			output = append(output, r)
		}
	}

	return output, o.Priority, nil
}

type ReconcileOptions struct {
	Eventf        func(string, string, string, ...interface{})
	ListenerArn   *string
	ListenerRules *Rules
	TargetGroups  targetgroups.TargetGroups
}
