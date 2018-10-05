package rs

import (
	"context"
	"fmt"
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
	Reconcile(context.Context, *Rules) error
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
	getDesiredRulesFunc func(*extensions.Ingress, tg.TargetGroups) ([]*Rule, error)
}

// Reconcile modifies AWS resources to match the rules defined in the Ingress
func (c *rulesController) Reconcile(ctx context.Context, rules *Rules) error {
	desired, err := c.getDesiredRulesFunc(rules.Ingress, rules.TargetGroups)
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

		if _, err := albelbv2.ELBV2svc.CreateRule(in); err != nil {
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

		if _, err := albelbv2.ELBV2svc.ModifyRule(in); err != nil {
			msg := fmt.Sprintf("Error modifying rule %s: %s", aws.StringValue(rule.Priority), err.Error())
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
		if _, err := albelbv2.ELBV2svc.DeleteRule(in); err != nil {
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

		if c == nil && d != nil {
			add = append(add, d)
		}
		if c != nil && d != nil {
			d.RuleArn = c.RuleArn
			modify = append(modify, d)
		}
		if c != nil && d == nil {
			remove = append(remove, c)
		}
	}

	return
}

func (c *rulesController) getDesiredRules(ingress *extensions.Ingress, targetGroups tg.TargetGroups) ([]*Rule, error) {
	var output []*Rule

	currentPriority := 1
	for _, rule := range ingress.Spec.Rules {
		if len(rule.HTTP.Paths) == 0 {
			return nil, fmt.Errorf("ingress doesn't have any paths defined")
		}

		for _, path := range rule.HTTP.Paths {
			r := &Rule{
				Rule: elbv2.Rule{
					Priority: aws.String(fmt.Sprintf("%v", currentPriority)),
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

// TODO: Determine value
// func (r OldRule) valid(listenerPort int64, listenerProtocol *string) bool {
// 	if r.rs.desired.Actions[0].RedirectConfig != nil {
// 		var host, path *string
// 		rc := r.rs.desired.Actions[0].RedirectConfig
//
// 		for _, c := range r.rs.desired.Conditions {
// 			if *c.Field == "host-header" {
// 				host = c.Values[0]
// 			}
// 			if *c.Field == "path-pattern" {
// 				path = c.Values[0]
// 			}
// 		}
//
// 		if host == nil && *rc.Host != "#{host}" {
// 			return true
// 		}
// 		if host != nil && *rc.Host != *host && *rc.Host != "#{host}" {
// 			return true
// 		}
// 		if path == nil && *rc.Path != "/#{path}" {
// 			return true
// 		}
// 		if path != nil && *rc.Path != *path && *rc.Path != "/#{path}" {
// 			return true
// 		}
// 		if *rc.Port != "#{port}" && *rc.Port != fmt.Sprintf("%v", listenerPort) {
// 			return true
// 		}
// 		if *rc.Query != "#{query}" {
// 			return true
// 		}
// 		if listenerProtocol != nil && *rc.Protocol != "#{protocol}" && *rc.Protocol != *listenerProtocol {
// 			return true
// 		}
// 		return false
// 	}
// 	return true
// }
