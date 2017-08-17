package rules

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	ruleP "github.com/coreos/alb-ingress-controller/pkg/alb/rule"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroups"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	extensions "k8s.io/api/extensions/v1beta1"
)

// Rules contains a slice of Rules
type Rules []*ruleP.Rule

// Reconcile kicks off the state synchronization for every Rule in this Rules slice.
func (r Rules) Reconcile(rOpts *ReconcileOptions) (Rules, error) {
	var output Rules
	for _, rule := range r {
		ruleOpts := ruleP.NewReconcileOptions()
		ruleOpts.SetEventf(rOpts.Eventf)
		ruleOpts.SetListenerArn(rOpts.ListenerArn)
		ruleOpts.SetTargetGroups(rOpts.TargetGroups)
		if err := rule.Reconcile(ruleOpts); err != nil {
			return nil, err
		}
		if !rule.Deleted {
			output = append(output, rule)
		}
	}

	return output, nil
}

// FindByPriority returns the position in the Rules slice of the rule parameter
func (r Rules) FindByPriority(rule *elbv2.Rule) int {
	for p, v := range r {
		if v.CurrentRule == nil {
			continue
		}
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

	for _, path := range o.Rule.HTTP.Paths { // first path is skipped as it is assumed to be default
		// Start with a new rule
		rule := ruleP.NewRule(o.Priority, o.Hostname, path.Path, path.Backend.ServiceName, o.Logger)

		// If this rule is already defined, copy the desired state over
		if i := output.FindByPriority(rule.DesiredRule); i >= 0 {
			output[i].DesiredRule = rule.DesiredRule
		} else {
			output = append(output, rule)
		}
	}

	return output, o.Priority, nil
}

type ReconcileOptions struct {
	Eventf        func(string, string, string, ...interface{})
	ListenerArn   *string
	ListenerRules *Rules
	TargetGroups  *targetgroups.TargetGroups
}

func NewReconcileOptions() *ReconcileOptions {
	return &ReconcileOptions{}
}

func (r *ReconcileOptions) SetListenerArn(arn *string) *ReconcileOptions {
	r.ListenerArn = arn
	return r
}

func (r *ReconcileOptions) SetEventf(f func(string, string, string, ...interface{})) *ReconcileOptions {
	r.Eventf = f
	return r
}

func (r *ReconcileOptions) SetTargetGroups(targetgroups *targetgroups.TargetGroups) *ReconcileOptions {
	r.TargetGroups = targetgroups
	return r
}
