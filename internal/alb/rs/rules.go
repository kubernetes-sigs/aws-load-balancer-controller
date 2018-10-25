package rs

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
)

// Controller provides functionality to manage rules
type Controller interface {
	// Reconcile ensures the listener rules in AWS match the rules configured in the Ingress resource.
	Reconcile(ctx context.Context, listener *elbv2.Listener, ingress *extensions.Ingress, ingressAnnos *annotations.Ingress, tgGroup tg.TargetGroupGroup) error
}

// NewController constructs a new rules controller
func NewController(cloud aws.CloudAPI) Controller {
	c := &defaultController{
		cloud: cloud,
	}
	c.getCurrentRulesFunc = c.getCurrentRules
	c.getDesiredRulesFunc = c.getDesiredRules
	return c
}

type defaultController struct {
	cloud               aws.CloudAPI
	getCurrentRulesFunc func(string) ([]elbv2.Rule, error)
	getDesiredRulesFunc func(*elbv2.Listener, *extensions.Ingress, *annotations.Ingress, tg.TargetGroupGroup) ([]elbv2.Rule, error)
}

// Reconcile modifies AWS resources to match the rules defined in the Ingress
func (c *defaultController) Reconcile(ctx context.Context, listener *elbv2.Listener, ingress *extensions.Ingress, ingressAnnos *annotations.Ingress, tgGroup tg.TargetGroupGroup) error {
	desired, err := c.getDesiredRulesFunc(listener, ingress, ingressAnnos, tgGroup)
	if err != nil {
		return err
	}
	lsArn := aws.StringValue(listener.ListenerArn)
	current, err := c.getCurrentRulesFunc(lsArn)
	if err != nil {
		return err
	}
	additions, modifies, removals := rulesChangeSets(current, desired)

	for _, rule := range additions {
		albctx.GetLogger(ctx).Infof("creating rule %v on %v", aws.StringValue(rule.Priority), lsArn)
		priority, _ := strconv.ParseInt(aws.StringValue(rule.Priority), 10, 64)
		in := &elbv2.CreateRuleInput{
			ListenerArn: aws.String(lsArn),
			Actions:     rule.Actions,
			Conditions:  rule.Conditions,
			Priority:    aws.Int64(priority),
		}

		if _, err := c.cloud.CreateRule(in); err != nil {
			msg := fmt.Sprintf("failed creating rule %v on %v due to %v", aws.StringValue(rule.Priority), lsArn, err)
			albctx.GetLogger(ctx).Errorf(msg)
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", msg)
			return fmt.Errorf(msg)
		}

		msg := fmt.Sprintf("rule %v created with conditions %v", aws.StringValue(rule.Priority), log.Prettify(rule.Conditions))
		albctx.GetLogger(ctx).Infof(msg)
		albctx.GetEventf(ctx)(api.EventTypeNormal, "CREATE", msg)
	}

	for _, rule := range modifies {
		albctx.GetLogger(ctx).Infof("modifying rule %v on %v", aws.StringValue(rule.Priority), lsArn)
		in := &elbv2.ModifyRuleInput{
			Actions:    rule.Actions,
			Conditions: rule.Conditions,
			RuleArn:    rule.RuleArn,
		}

		if _, err := c.cloud.ModifyRule(in); err != nil {
			msg := fmt.Sprintf("failed modifying rule %v on %v due to %v", aws.StringValue(rule.Priority), lsArn, err)
			albctx.GetLogger(ctx).Errorf(msg)
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", msg)
			return fmt.Errorf(msg)
		}

		msg := fmt.Sprintf("rule %v modified with conditions %v", aws.StringValue(rule.Priority), log.Prettify(rule.Conditions))
		albctx.GetEventf(ctx)(api.EventTypeNormal, "MODIFY", msg)
		albctx.GetLogger(ctx).Infof(msg)
	}

	for _, rule := range removals {
		albctx.GetLogger(ctx).Infof("deleting rule %v on %v", aws.StringValue(rule.Priority), lsArn)

		in := &elbv2.DeleteRuleInput{RuleArn: rule.RuleArn}
		if _, err := c.cloud.DeleteRule(in); err != nil {
			msg := fmt.Sprintf("failed deleting rule %v on %v due to %v", aws.StringValue(rule.Priority), lsArn, err)
			albctx.GetLogger(ctx).Errorf(msg)
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", msg)
			return fmt.Errorf(msg)
		}

		msg := fmt.Sprintf("rule %v deleted with conditions %v", aws.StringValue(rule.Priority), log.Prettify(rule.Conditions))
		albctx.GetEventf(ctx)(api.EventTypeNormal, "DELETE", msg)
		albctx.GetLogger(ctx).Infof(msg)
	}
	return nil
}

func (c *defaultController) getDesiredRules(listener *elbv2.Listener, ingress *extensions.Ingress, ingressAnnos *annotations.Ingress, tgGroup tg.TargetGroupGroup) ([]elbv2.Rule, error) {
	var output []elbv2.Rule

	currentPriority := 1
	for _, ingressRule := range ingress.Spec.Rules {
		// Ingress spec allows empty HTTP, and we will 'route all traffic to the default backend'(which relies on default action of listeners)
		if ingressRule.HTTP == nil {
			continue
		}

		for _, path := range ingressRule.HTTP.Paths {
			elbRule := elbv2.Rule{
				IsDefault: aws.Bool(false),
				Priority:  aws.String(strconv.Itoa(currentPriority)),
			}

			// Handle the annotation based actions
			if action.Use(path.Backend.ServicePort.String()) {
				action, err := ingressAnnos.Action.GetAction(path.Backend.ServiceName)
				if err != nil {
					return nil, err
				}
				elbRule.Actions = []*elbv2.Action{action}
			} else {
				targetGroup, ok := tgGroup.TGByBackend[path.Backend]
				if !ok {
					return nil, fmt.Errorf("unable to locate a target group for backend %v:%v",
						path.Backend.ServiceName, path.Backend.ServicePort.String())
				}
				elbRule.Actions = []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: aws.String(targetGroup.Arn),
					},
				}
			}

			if ingressRule.Host != "" {
				elbRule.Conditions = append(elbRule.Conditions, condition("host-header", ingressRule.Host))
			}

			if path.Path != "" {
				elbRule.Conditions = append(elbRule.Conditions, condition("path-pattern", path.Path))
			}

			if createsRedirectLoop(listener, elbRule) {
				continue
			}
			output = append(output, elbRule)
			currentPriority++
		}
	}
	return output, nil
}

func (c *defaultController) getCurrentRules(listenerArn string) (results []elbv2.Rule, err error) {
	rules, err := c.cloud.GetRules(listenerArn)
	if err != nil {
		return nil, err
	}

	for _, rule := range rules {
		if aws.BoolValue(rule.IsDefault) {
			// Ignore these, let the listener manage it
			continue
		}
		results = append(results, *rule)
	}

	return results, nil
}

// rulesChangeSets compares desired to current, returning a list of rules to add, modify and remove from current to match desired
func rulesChangeSets(current, desired []elbv2.Rule) (add []elbv2.Rule, modify []elbv2.Rule, remove []elbv2.Rule) {
	currentMap := make(map[string]elbv2.Rule, len(current))
	desiredMap := make(map[string]elbv2.Rule, len(desired))
	for _, i := range current {
		currentMap[aws.StringValue(i.Priority)] = i
	}
	for _, i := range desired {
		desiredMap[aws.StringValue(i.Priority)] = i
	}
	currentKeys := sets.StringKeySet(currentMap)
	desiredKeys := sets.StringKeySet(desiredMap)
	for key := range desiredKeys.Difference(currentKeys) {
		desiredRule := desiredMap[key]
		sortConditions(desiredRule.Conditions)
		add = append(add, desiredRule)
	}
	for key := range currentKeys.Difference(desiredKeys) {
		remove = append(remove, currentMap[key])
	}
	for key := range currentKeys.Intersection(desiredKeys) {
		currentRule := currentMap[key]
		desiredRule := desiredMap[key]
		desiredRule.RuleArn = currentRule.RuleArn
		sortConditions(currentRule.Conditions)
		sortConditions(desiredRule.Conditions)
		if !reflect.DeepEqual(currentRule, desiredRule) {
			modify = append(modify, desiredRule)
		}
	}
	return add, modify, remove
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

func condition(field string, values ...string) *elbv2.RuleCondition {
	return &elbv2.RuleCondition{
		Field:  aws.String(field),
		Values: aws.StringSlice(values),
	}
}

// createsRedirectLoop checks whether specified rule creates redirectionLoop for listener of protocol & port
func createsRedirectLoop(listener *elbv2.Listener, r elbv2.Rule) bool {
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
		if aws.StringValue(rc.Port) != "#{port}" && aws.StringValue(rc.Port) != fmt.Sprintf("%v", aws.Int64Value(listener.Port)) {
			return false
		}
		if aws.StringValue(rc.Query) != "#{query}" {
			return false
		}
		if aws.StringValue(rc.Protocol) != "#{protocol}" && aws.StringValue(rc.Protocol) != aws.StringValue(listener.Protocol) {
			return false
		}
		return true
	}
	return false
}
