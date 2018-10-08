package rs

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Rule contain the elbv2 rule configuration along with the Ingress backend that it is forwarding to
type Rule struct {
	Backend extensions.IngressBackend
	elbv2.Rule
}

// Rules contains the rules for a listener.
type Rules struct {
	Ingress *extensions.Ingress

	ListenerArn  string
	TargetGroups tg.TargetGroups
	Rules        []*Rule
}

// NewRules returns a new Rules pointer
func NewRules(ingress *extensions.Ingress) *Rules {
	return &Rules{
		Ingress: ingress,
	}
}

// RulesController provides functionality to manage rules
type RulesController interface {
	// Reconcile ensures the listener rules in AWS match the rules configured in the Ingress resource.
	Reconcile(context.Context, *Rules, *elbv2.Listener) error
}

// NewRulesController constructs a new rules controller
func NewRulesController(elbv2svc albelbv2.ELBV2API, store store.Storer) *rulesController {
	c := &rulesController{
		elbv2: elbv2svc,
		store: store,
	}
	c.getCurrentRulesFunc = c.getCurrentRules
	c.getDesiredRulesFunc = c.getDesiredRules
	return c
}

type rulesController struct {
	elbv2               albelbv2.ELBV2API
	store               store.Storer
	getCurrentRulesFunc func(string) ([]*Rule, error)
	getDesiredRulesFunc func(*extensions.Ingress, tg.TargetGroups, *elbv2.Listener) ([]*Rule, error)
}

// Reconcile modifies AWS resources to match the rules defined in the Ingress
func (c *rulesController) Reconcile(ctx context.Context, rules *Rules, listener *elbv2.Listener) error {
	desired, err := c.getDesiredRulesFunc(rules.Ingress, rules.TargetGroups, listener)
	if err != nil {
		return err
	}
	current, err := c.getCurrentRulesFunc(rules.ListenerArn)
	if err != nil {
		return err
	}
	additions, modifies, removals := rulesChangeSets(current, desired)

	for _, rule := range additions {
		albctx.GetLogger(ctx).Infof("Create rule %v on %v.", aws.StringValue(rule.Priority), rules.ListenerArn)
		in := &elbv2.CreateRuleInput{
			Actions:     rule.Actions,
			Conditions:  rule.Conditions,
			ListenerArn: aws.String(rules.ListenerArn),
			Priority:    priority(rule.Priority),
		}

		if _, err := c.elbv2.CreateRule(in); err != nil {
			msg := fmt.Sprintf("Error adding rule %v to %v: %v", aws.StringValue(rule.Priority), rules.ListenerArn, err.Error())
			albctx.GetLogger(ctx).Errorf(msg)
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", msg)
			return err
		}

		msg := fmt.Sprintf("Rule created, priority %v with conditions %v", aws.StringValue(rule.Priority), log.Prettify(rule.Conditions))
		albctx.GetLogger(ctx).Infof(msg)
		albctx.GetEventf(ctx)(api.EventTypeNormal, "CREATE", msg)
	}

	for _, rule := range modifies {
		albctx.GetLogger(ctx).Infof("Modifying rule %v on %v.", aws.StringValue(rule.Priority), rules.ListenerArn)
		in := &elbv2.ModifyRuleInput{
			Actions:    rule.Actions,
			Conditions: rule.Conditions,
			RuleArn:    rule.RuleArn,
		}

		if _, err := c.elbv2.ModifyRule(in); err != nil {
			msg := fmt.Sprintf("error modifying rule %s: %s", aws.StringValue(rule.Priority), err.Error())
			albctx.GetLogger(ctx).Errorf(msg)
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", msg)
			return fmt.Errorf(msg)
		}

		msg := fmt.Sprintf("Rule modified, priority %v with conditions %v", aws.StringValue(rule.Priority), log.Prettify(rule.Conditions))
		albctx.GetEventf(ctx)(api.EventTypeNormal, "MODIFY", msg)
		albctx.GetLogger(ctx).Infof(msg)
	}

	for _, rule := range removals {
		albctx.GetLogger(ctx).Infof("Deleting rule %v on %v.", aws.StringValue(rule.Priority), rules.ListenerArn)

		in := &elbv2.DeleteRuleInput{RuleArn: rule.RuleArn}
		if _, err := c.elbv2.DeleteRule(in); err != nil {
			msg := fmt.Sprintf("Error deleting %v rule: %s", aws.StringValue(rule.Priority), err.Error())
			albctx.GetLogger(ctx).Errorf(msg)
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", msg)
			return err
		}

		msg := fmt.Sprintf("Rule deleted, priority %v with conditions %v", aws.StringValue(rule.Priority), log.Prettify(rule.Conditions))
		albctx.GetEventf(ctx)(api.EventTypeNormal, "DELETE", msg)
		albctx.GetLogger(ctx).Infof(msg)
	}
	return nil
}

// rulesChangeSets compares b to a, returning a list of rules to add, modify and remove from a to match b
func rulesChangeSets(current, desired []*Rule) (add []*Rule, modify []*Rule, remove []*Rule) {
	currentMap := map[string]*Rule{}
	desiredMap := map[string]*Rule{}

	for _, i := range current {
		currentMap[aws.StringValue(i.Priority)] = i
	}
	for _, i := range desired {
		desiredMap[aws.StringValue(i.Priority)] = i
	}

	max := len(current)
	if len(desired) > max {
		max = len(desired)
	}

	for i := 1; i <= max; i++ {
		is := fmt.Sprintf("%v", i)
		c := currentMap[is]
		d := desiredMap[is]
		if c != nil && d != nil {
			d.RuleArn = c.RuleArn
		}

		if c != nil {
			sortConditions(c.Conditions)
		}
		if d != nil {
			sortConditions(d.Conditions)
		}

		if c == nil && d != nil {
			add = append(add, d)
		}

		if c != nil && d != nil && c.String() != d.String() {
			d.RuleArn = c.RuleArn
			modify = append(modify, d)
		}

		if c != nil && d == nil {
			remove = append(remove, c)
		}
	}

	return
}

func sortConditions(cond []*elbv2.RuleCondition) {
	sort.Slice(cond, func(i, j int) bool {
		condi := cond[i]
		condj := cond[j]
		sort.Slice(condi.Values, func(i, j int) bool { return aws.StringValue(condi.Values[i]) < aws.StringValue(condi.Values[j]) })
		sort.Slice(condj.Values, func(i, j int) bool { return aws.StringValue(condj.Values[i]) < aws.StringValue(condj.Values[j]) })
		return aws.StringValue(condi.Field) < aws.StringValue(condj.Field)
	})
}

func (c *rulesController) getDesiredRules(ingress *extensions.Ingress, targetGroups tg.TargetGroups, listener *elbv2.Listener) ([]*Rule, error) {
	var output []*Rule

	currentPriority := 1
	for _, rule := range ingress.Spec.Rules {
		if len(rule.HTTP.Paths) == 0 {
			return nil, fmt.Errorf("ingress doesn't have any paths defined")
		}

		for _, path := range rule.HTTP.Paths {
			r := &Rule{
				Rule: elbv2.Rule{
					IsDefault: aws.Bool(false),
					Priority:  aws.String(fmt.Sprintf("%v", currentPriority)),
				},
				Backend: path.Backend,
			}

			// Handle the annotation based actions
			if action.Use(path.Backend.ServicePort.String()) {
				annos, err := c.store.GetIngressAnnotations(k8s.MetaNamespaceKey(ingress))
				if err != nil {
					return nil, err
				}
				actionConfig, err := annos.Action.GetAction(path.Backend.ServiceName)
				if err != nil {
					return nil, err
				}
				r.Actions = []*elbv2.Action{actionConfig}
			} else {
				i := targetGroups.LookupByBackend(path.Backend)
				if i < 0 {
					return nil, fmt.Errorf("unable to locate a target group for backend %v:%v",
						path.Backend.ServiceName, path.Backend.ServicePort.String())
				}

				r.Actions = actions(&elbv2.Action{TargetGroupArn: targetGroups[i].CurrentARN()}, elbv2.ActionTypeEnumForward)
			}

			if rule.Host != "" {
				r.Conditions = append(r.Conditions, condition("host-header", rule.Host))
			}

			if path.Path != "" {
				r.Conditions = append(r.Conditions, condition("path-pattern", path.Path))
			}

			if conflictsWithRedirectConfig(r, listener) {
				continue
			}
			output = append(output, r)
			currentPriority++
		}
	}
	return output, nil
}

func (c *rulesController) getCurrentRules(listenerArn string) (results []*Rule, err error) {
	rules, err := c.elbv2.GetRules(listenerArn)
	if err != nil {
		return nil, err
	}

	for _, rule := range rules {
		if aws.BoolValue(rule.IsDefault) {
			// Ignore these, let the listener manage it
			continue
		}
		if len(rule.Actions) != 1 {
			return nil, fmt.Errorf("invalid amount of actions on rule for listener %v", listenerArn)
		}

		r := &Rule{Rule: *rule, Backend: extensions.IngressBackend{}}
		a := r.Actions[0]

		if aws.StringValue(a.Type) == elbv2.ActionTypeEnumForward {
			tagsOutput, err := c.elbv2.DescribeTags(&elbv2.DescribeTagsInput{ResourceArns: []*string{a.TargetGroupArn}})
			if err != nil {
				return nil, err
			}
			for _, tag := range tagsOutput.TagDescriptions[0].Tags {
				if aws.StringValue(tag.Key) == tags.ServiceName {
					r.Backend.ServiceName = aws.StringValue(tag.Value)
				}
				if aws.StringValue(tag.Key) == tags.ServicePort {
					r.Backend.ServicePort = intstr.FromString(aws.StringValue(tag.Value))
				}
			}
		} else {
			r.Backend.ServicePort = intstr.FromString(action.UseActionAnnotation)
		}

		results = append(results, r)
	}

	return results, nil
}

func backend(serviceName string, servicePort intstr.IntOrString) extensions.IngressBackend {
	return extensions.IngressBackend{
		ServiceName: serviceName,
		ServicePort: servicePort,
	}
}

func conditions(conditions ...*elbv2.RuleCondition) []*elbv2.RuleCondition {
	return conditions
}

func condition(field string, values ...string) *elbv2.RuleCondition {
	return &elbv2.RuleCondition{
		Field:  aws.String(field),
		Values: aws.StringSlice(values),
	}
}

func actions(a *elbv2.Action, t string) []*elbv2.Action {
	a.Type = aws.String(t)
	return []*elbv2.Action{a}
}

func priority(s *string) *int64 {
	if *s == "default" {
		return aws.Int64(0)
	}
	i, _ := strconv.ParseInt(*s, 10, 64)
	return aws.Int64(i)
}

func conflictsWithRedirectConfig(r *Rule, l *elbv2.Listener) bool {
	for _, action := range r.Actions {
		var host, path *string
		rc := action.RedirectConfig
		if rc == nil {
			continue
		}

		for _, c := range r.Conditions {
			if aws.StringValue(c.Field) == "host-header" {
				host = c.Values[0]
			}
			if aws.StringValue(c.Field) == "path-pattern" {
				path = c.Values[0]
			}
		}

		if host == nil && aws.StringValue(rc.Host) != "#{host}" {
			return false
		}
		if host != nil && aws.StringValue(rc.Host) != aws.StringValue(host) && aws.StringValue(rc.Host) != "#{host}" {
			return false
		}
		if path == nil && aws.StringValue(rc.Path) != "/#{path}" {
			return false
		}
		if path != nil && aws.StringValue(rc.Path) != aws.StringValue(path) && aws.StringValue(rc.Path) != "/#{path}" {
			return false
		}
		if aws.StringValue(rc.Port) != "#{port}" && aws.StringValue(rc.Port) != fmt.Sprintf("%v", aws.Int64Value(l.Port)) {
			return false
		}
		if aws.StringValue(rc.Query) != "#{query}" {
			return false
		}
		if aws.StringValue(rc.Protocol) != "#{protocol}" && aws.StringValue(rc.Protocol) != aws.StringValue(l.Protocol) {
			return false
		}
		return true
	}
	return false
}
