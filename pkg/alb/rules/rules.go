package rules

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/awsutil"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/alb/rule"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

// Rules contains a slice of Rules
type Rules []*rule.Rule

type NewDesiredRulesOptions struct {
	Priority         int
	Logger           *log.Logger
	ListenerRules    Rules
	Rule             *extensions.IngressRule
	IgnoreHostHeader bool
}

// NewDesiredRules returns a Rules created by appending the IngressRule paths to a ListenerRules.
// The returned priority is the highest priority added to the rules list.
func NewDesiredRules(o *NewDesiredRulesOptions) (Rules, int, error) {
	rs := o.ListenerRules
	paths := o.Rule.HTTP.Paths

	if len(o.Rule.HTTP.Paths) == 0 {
		return nil, 0, fmt.Errorf("Ingress doesn't have any paths defined. This is not a very good ingress.")
	}

	// If there are no pre-existing rules on the listener, inject a default rule.
	// Since the Kubernetes ingress has no notion of this, we pick the first backend.
	if o.Priority == 0 {
		paths = append([]extensions.HTTPIngressPath{o.Rule.HTTP.Paths[0]}, o.Rule.HTTP.Paths...)
	}

	for _, path := range paths {
		r := rule.NewDesiredRule(&rule.NewDesiredRuleOptions{
			Priority:         o.Priority,
			Hostname:         o.Rule.Host,
			IgnoreHostHeader: o.IgnoreHostHeader,
			Path:             path.Path,
			SvcName:          path.Backend.ServiceName,
			Logger:           o.Logger,
		})
		if !rs.merge(r) {
			rs = append(rs, r)
		}
		o.Priority++
	}

	return rs, o.Priority, nil
}

func (rs Rules) merge(mergeRule *rule.Rule) bool {
	if i, r := rs.FindByPriority(mergeRule.Desired.Priority); i >= 0 {
		r.Desired = mergeRule.Desired
		r.DesiredSvcName = mergeRule.DesiredSvcName
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
func (rs Rules) FindByPriority(priority *string) (int, *rule.Rule) {
	for p, v := range rs {
		if v.Current == nil {
			continue
		}
		if awsutil.DeepEqual(v.Current.Priority, priority) {
			return p, v
		}
	}
	return -1, nil
}

// FindUnusedTGs returns a list of TargetGroups that are no longer referncd by any of
// the rules passed into this method.
func (rs Rules) FindUnusedTGs(tgs tg.TargetGroups) tg.TargetGroups {
	unused := tg.TargetGroups{}

	for _, t := range tgs {
		used := false
		for _, r := range rs {
			if r.Current != nil && r.Current.Actions[0] != nil && r.Current.Actions[0].TargetGroupArn == nil {
				continue
			}
			arn := t.CurrentARN()
			if arn == nil {
				continue
			}
			if r.Current != nil && r.Current.Actions[0] != nil && *r.Current.Actions[0].TargetGroupArn == *arn {
				used = true
				break
			}
		}
		if !used {
			unused = append(unused, t)
		}
	}

	return unused
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
	TargetGroups  tg.TargetGroups
}
