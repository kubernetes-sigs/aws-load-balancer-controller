package rs

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

type NewCurrentRulesOptions struct {
	ListenerArn  *string
	Logger       *log.Logger
	TargetGroups tg.TargetGroups
}

// NewCurrentRules
func NewCurrentRules(o *NewCurrentRulesOptions) (Rules, error) {
	var rs Rules

	o.Logger.Infof("Fetching Rules for Listener %s", *o.ListenerArn)
	rules, err := albelbv2.ELBV2svc.DescribeRules(&elbv2.DescribeRulesInput{ListenerArn: o.ListenerArn})
	if err != nil {
		return nil, err
	}

	for _, r := range rules.Rules {
		// TODO LOOKUP svcName based on TG
		i, tg := o.TargetGroups.FindCurrentByARN(*r.Actions[0].TargetGroupArn)
		if i < 0 {
			return nil, fmt.Errorf("failed to find a target group associated with a rule. This should not be possible. Rule: %s, ARN: %s", awsutil.Prettify(r.RuleArn), *r.Actions[0].TargetGroupArn)
		}

		newRule := NewCurrentRule(&NewCurrentRuleOptions{
			SvcName: tg.SvcName,
			SvcPort: tg.SvcPort,
			Rule:    r,
			Logger:  o.Logger,
		})

		rs = append(rs, newRule)
	}

	return rs, nil
}

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

	if len(paths) == 0 {
		return nil, 0, fmt.Errorf("Ingress doesn't have any paths defined. This is not a very good ingress.")
	}

	// If there are no pre-existing rules on the listener, inject a default rule.
	// Since the Kubernetes ingress has no notion of this, we pick the first backend.
	if o.Priority == 0 {
		paths = append([]extensions.HTTPIngressPath{paths[0]}, paths...)
	}

	for _, path := range paths {
		r := NewDesiredRule(&NewDesiredRuleOptions{
			Priority:         o.Priority,
			Hostname:         o.Rule.Host,
			IgnoreHostHeader: o.IgnoreHostHeader,
			Path:             path.Path,
			SvcName:          path.Backend.ServiceName,
			SvcPort:          path.Backend.ServicePort.IntVal,
			Logger:           o.Logger,
		})
		if !rs.merge(r) {
			rs = append(rs, r)
		}
		o.Priority++
	}

	return rs, o.Priority, nil
}

func (r Rules) merge(mergeRule *Rule) bool {
	if i, existingRule := r.FindByPriority(mergeRule.rs.desired.Priority); i >= 0 {
		existingRule.rs.desired = mergeRule.rs.desired
		existingRule.svc.desired = mergeRule.svc.desired
		return true
	}
	return false
}

// Reconcile kicks off the state synchronization for every Rule in this Rules slice.
func (r Rules) Reconcile(rOpts *ReconcileOptions) (Rules, error) {
	var output Rules

	for _, rule := range r {
		if err := rule.Reconcile(rOpts); err != nil {
			return nil, err
		}
		if !rule.deleted {
			output = append(output, rule)
		}
	}

	return output, nil
}

// FindByPriority returns the position in the Rules slice of the rule parameter
func (r Rules) FindByPriority(priority *string) (int, *Rule) {
	for p, v := range r {
		if v.rs.current == nil {
			continue
		}
		if awsutil.DeepEqual(v.rs.current.Priority, priority) {
			return p, v
		}
	}
	return -1, nil
}

// FindUnusedTGs returns a list of TargetGroups that are no longer referncd by any of
// the rules passed into this method.
func (r Rules) FindUnusedTGs(tgs tg.TargetGroups) tg.TargetGroups {
	var unused tg.TargetGroups

TG:
	for _, t := range tgs {
		used := false

		arn := t.CurrentARN()
		if arn == nil {
			continue
		}

		for _, rule := range r {
			if rule.rs.current == nil {
				continue TG
			}

			for _, action := range rule.rs.current.Actions {
				if *action.TargetGroupArn == *arn {
					used = true
					continue TG
				}
			}
		}

		if !used {
			unused = append(unused, t)
		}
	}

	return unused
}

// DefaultRule returns the ALBs default rule
func (r Rules) DefaultRule() *Rule {
	for _, rule := range r {
		if rule.rs.desired == nil {
			continue
		}
		if *rule.rs.desired.IsDefault {
			return rule
		}
	}
	return nil
}

// StripDesiredState removes the desired state from all Rules in the slice.
func (r Rules) StripDesiredState() {
	for _, rule := range r {
		rule.stripDesiredState()
	}
}

// StripCurrentState removes the current statefrom all Rule instances.
func (r Rules) StripCurrentState() {
	for _, rule := range r {
		rule.stripCurrentState()
	}
}
