package ls

import (
	"context"
	"fmt"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	"sort"
	"strconv"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// RulesController provides functionality to manage rules on listeners
type RulesController interface {
	// Reconcile ensures the listener rules in AWS match the rules configured in the Ingress resource.
	Reconcile(ctx context.Context, listener *elbv2.Listener, service *corev1.Service, serviceAnnos *annotations.Service, tgGroup tg.TargetGroupGroup) error
}

// NewRulesController constructs RulesController
func NewRulesController(cloud aws.CloudAPI) RulesController {
	return &rulesController{
		cloud:      cloud,
	}
}

type rulesController struct {
	cloud      aws.CloudAPI
}

// Reconcile modifies AWS resources to match the rules defined in the Ingress
func (c *rulesController) Reconcile(ctx context.Context, listener *elbv2.Listener, service *corev1.Service, serviceAnnos *annotations.Service, tgGroup tg.TargetGroupGroup) error {
	desired, err := c.getDesiredRules(ctx, listener, service, serviceAnnos, tgGroup)
	if err != nil {
		return err
	}
	lsArn := aws.StringValue(listener.ListenerArn)
	current, err := c.getCurrentRules(ctx, lsArn)
	if err != nil {
		return err
	}
	return c.reconcileRules(ctx, lsArn, current, desired)
}

func (c *rulesController) reconcileRules(ctx context.Context, lsArn string, current []elbv2.Rule, desired []elbv2.Rule) error {
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

		if _, err := c.cloud.CreateRuleWithContext(ctx, in); err != nil {
			msg := fmt.Sprintf("failed creating rule %v on %v due to %v", aws.StringValue(rule.Priority), lsArn, err)
			albctx.GetLogger(ctx).Errorf(msg)
			albctx.GetEventf(ctx)(corev1.EventTypeWarning, "ERROR", msg)
			return fmt.Errorf(msg)
		}

		msg := fmt.Sprintf("rule %v created with conditions %v", aws.StringValue(rule.Priority), log.Prettify(rule.Conditions))
		albctx.GetLogger(ctx).Infof(msg)
		albctx.GetEventf(ctx)(corev1.EventTypeNormal, "CREATE", msg)
	}

	for _, rule := range modifies {
		albctx.GetLogger(ctx).Infof("modifying rule %v on %v", aws.StringValue(rule.Priority), lsArn)
		in := &elbv2.ModifyRuleInput{
			Actions:    rule.Actions,
			Conditions: rule.Conditions,
			RuleArn:    rule.RuleArn,
		}

		if _, err := c.cloud.ModifyRuleWithContext(ctx, in); err != nil {
			msg := fmt.Sprintf("failed modifying rule %v on %v due to %v", aws.StringValue(rule.Priority), lsArn, err)
			albctx.GetLogger(ctx).Errorf(msg)
			albctx.GetEventf(ctx)(corev1.EventTypeWarning, "ERROR", msg)
			return fmt.Errorf(msg)
		}

		msg := fmt.Sprintf("rule %v modified with conditions %v", aws.StringValue(rule.Priority), log.Prettify(rule.Conditions))
		albctx.GetEventf(ctx)(corev1.EventTypeNormal, "MODIFY", msg)
		albctx.GetLogger(ctx).Infof(msg)
	}

	for _, rule := range removals {
		albctx.GetLogger(ctx).Infof("deleting rule %v on %v", aws.StringValue(rule.Priority), lsArn)

		in := &elbv2.DeleteRuleInput{RuleArn: rule.RuleArn}
		if _, err := c.cloud.DeleteRuleWithContext(ctx, in); err != nil {
			msg := fmt.Sprintf("failed deleting rule %v on %v due to %v", aws.StringValue(rule.Priority), lsArn, err)
			albctx.GetLogger(ctx).Errorf(msg)
			albctx.GetEventf(ctx)(corev1.EventTypeWarning, "ERROR", msg)
			return fmt.Errorf(msg)
		}

		msg := fmt.Sprintf("rule %v deleted with conditions %v", aws.StringValue(rule.Priority), log.Prettify(rule.Conditions))
		albctx.GetEventf(ctx)(corev1.EventTypeNormal, "DELETE", msg)
		albctx.GetLogger(ctx).Infof(msg)
	}
	return nil
}

func (c *rulesController) getDesiredRules(ctx context.Context, listener *elbv2.Listener, service *corev1.Service, serviceAnnos *annotations.Service, tgGroup tg.TargetGroupGroup) ([]elbv2.Rule, error) {
	var output []elbv2.Rule
	backend := extensions.IngressBackend{
		ServiceName: service.GetName(),
	}

	nextPriority := 1
	for _, port := range service.Spec.Ports {
		if port.Name != "" {
			backend.ServicePort = intstr.FromString(port.Name)
		} else {
			backend.ServicePort = intstr.FromInt(int(port.Port))
		}

		elbActions, err := buildActions(ctx, serviceAnnos, backend, tgGroup)
		if err != nil {
			return nil, err
		}
		elbRule := elbv2.Rule{
			IsDefault: aws.Bool(false),
			Priority:  aws.String(strconv.Itoa(nextPriority)),
			Actions:   elbActions,
		}
		output = append(output, elbRule)
		nextPriority++
	}

	return output, nil
}

func (c *rulesController) getCurrentRules(ctx context.Context, listenerArn string) ([]elbv2.Rule, error) {
	rules, err := c.cloud.GetRules(ctx, listenerArn)
	if err != nil {
		return nil, err
	}

	var output []elbv2.Rule

	for _, rule := range rules {
		if aws.BoolValue(rule.IsDefault) {
			// Ignore these, let the listener manage it
			continue
		}
		output = append(output, *rule)
	}

	return output, nil
}

// buildActions will build listener rule actions for specific authCfg and backend
func buildActions(ctx context.Context, serviceAnnos *annotations.Service, backend extensions.IngressBackend, tgGroup tg.TargetGroupGroup) ([]*elbv2.Action, error) {
	var actions []*elbv2.Action

	// Handle backend actions
	if action.Use(backend.ServicePort.String()) {
		// backend is based on annotation
		backendAction, err := serviceAnnos.Action.GetAction(backend.ServiceName)
		if err != nil {
			return nil, err
		}
		actions = append(actions, &backendAction)
	} else {
		// backend is based on service
		targetGroup, ok := tgGroup.TGByBackend[backend]
		if !ok {
			return nil, fmt.Errorf("unable to find targetGroup for backend %v:%v",
				backend.ServiceName, backend.ServicePort.String())
		}
		backendAction := elbv2.Action{
			Type:           aws.String(elbv2.ActionTypeEnumForward),
			TargetGroupArn: aws.String(targetGroup.Arn),
		}
		actions = append(actions, &backendAction)
	}

	for index, action := range actions {
		action.Order = aws.Int64(int64(index) + 1)
	}
	return actions, nil
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
		add = append(add, desiredMap[key])
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
		sortActions(currentRule.Actions)
		sortActions(desiredRule.Actions)
		if !reflect.DeepEqual(currentRule, desiredRule) {
			modify = append(modify, desiredRule)
		}
	}
	return add, modify, remove
}

func condition(field string, values ...string) *elbv2.RuleCondition {
	return &elbv2.RuleCondition{
		Field:  aws.String(field),
		Values: aws.StringSlice(values),
	}
}

func sortConditions(conditions []*elbv2.RuleCondition) {
	for _, cond := range conditions {
		sort.Slice(cond.Values, func(i, j int) bool { return aws.StringValue(cond.Values[i]) < aws.StringValue(cond.Values[j]) })
	}

	sort.Slice(conditions, func(i, j int) bool {
		return aws.StringValue(conditions[i].Field) < aws.StringValue(conditions[j].Field)
	})
}

func sortActions(actions []*elbv2.Action) {
	sort.Slice(actions, func(i, j int) bool {
		return aws.Int64Value(actions[i].Order) < aws.Int64Value(actions[j].Order)
	})
}
