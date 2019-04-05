package deploy

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
	"sort"
	"strconv"
)

func (a *loadBalancerActuator) reconcileListenerRules(ctx context.Context, listener api.Listener, lsArn string) error {
	currentRules, err := a.getCurrentListenerRules(ctx, lsArn)
	if err != nil {
		return err
	}
	desiredRules, err := a.buildELBV2ListenerRules(ctx, listener.Rules)
	if err != nil {
		return err
	}
	add, modify, remove := a.computeListenerRuleChangeSet(ctx, currentRules, desiredRules)

	for _, rule := range add {
		logging.FromContext(ctx).Info("creating listener rule", "lsArn", lsArn, "priority", aws.StringValue(rule.Priority))
		priority, _ := strconv.ParseInt(aws.StringValue(rule.Priority), 10, 64)
		in := &elbv2.CreateRuleInput{
			ListenerArn: aws.String(lsArn),
			Actions:     rule.Actions,
			Conditions:  rule.Conditions,
			Priority:    aws.Int64(priority),
		}
		if _, err := a.cloud.ELBV2().CreateRuleWithContext(ctx, in); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("created listener rule", "lsArn", lsArn, "priority", aws.StringValue(rule.Priority))
	}

	for _, rule := range modify {
		logging.FromContext(ctx).Info("modifying listener rule", "lsArn", lsArn, "priority", aws.StringValue(rule.Priority))
		in := &elbv2.ModifyRuleInput{
			RuleArn:    rule.RuleArn,
			Actions:    rule.Actions,
			Conditions: rule.Conditions,
		}

		if _, err := a.cloud.ELBV2().ModifyRuleWithContext(ctx, in); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("modified listener rule", "lsArn", lsArn, "priority", aws.StringValue(rule.Priority))
	}

	for _, rule := range remove {
		logging.FromContext(ctx).Info("removing listener rule", "lsArn", lsArn, "priority", aws.StringValue(rule.Priority))
		in := &elbv2.DeleteRuleInput{RuleArn: rule.RuleArn}
		if _, err := a.cloud.ELBV2().DeleteRuleWithContext(ctx, in); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("removed listener rule", "lsArn", lsArn, "priority", aws.StringValue(rule.Priority))
	}
	return nil
}

func (a *loadBalancerActuator) computeListenerRuleChangeSet(ctx context.Context, current []*elbv2.Rule, desired []*elbv2.Rule) (add []*elbv2.Rule, modify []*elbv2.Rule, remove []*elbv2.Rule) {
	currentMap := make(map[string]*elbv2.Rule, len(current))
	desiredMap := make(map[string]*elbv2.Rule, len(desired))

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

		normalizeELBV2ListenerRuleConditions(currentRule.Conditions)
		normalizeELBV2ListenerRuleConditions(desiredRule.Conditions)
		normalizeELBV2ListenerActions(currentRule.Actions)
		normalizeELBV2ListenerActions(desiredRule.Actions)
		if !awsutil.DeepEqual(currentRule, desiredRule) {
			modify = append(modify, desiredRule)
			logging.FromContext(ctx).Info("rule differs", "current", currentRule, "desired", desiredRule)
		}
	}
	return add, modify, remove
}

func (a *loadBalancerActuator) getCurrentListenerRules(ctx context.Context, lsArn string) ([]*elbv2.Rule, error) {
	resp, err := a.cloud.ELBV2().DescribeRulesAsList(ctx, &elbv2.DescribeRulesInput{
		ListenerArn: aws.String(lsArn),
	})
	if err != nil {
		return nil, err
	}

	var rules []*elbv2.Rule
	for _, rule := range resp {
		if aws.BoolValue(rule.IsDefault) {
			// Ignore these, let the listener manage it
			continue
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

func (a *loadBalancerActuator) buildELBV2ListenerRules(ctx context.Context, rules []api.ListenerRule) ([]*elbv2.Rule, error) {
	result := make([]*elbv2.Rule, 0, len(rules))
	for index, rule := range rules {
		elbv2Rule, err := a.buildELBV2ListenerRule(ctx, rule)
		if err != nil {
			return nil, err
		}
		priority := index + 1
		elbv2Rule.Priority = aws.String(strconv.Itoa(priority))
		elbv2Rule.IsDefault = aws.Bool(false)
		result = append(result, elbv2Rule)
	}
	return result, nil
}

func (a *loadBalancerActuator) buildELBV2ListenerRule(ctx context.Context, rule api.ListenerRule) (*elbv2.Rule, error) {
	conditions, err := a.buildELBV2ListenerRuleConditions(ctx, rule.Conditions)
	if err != nil {
		return nil, err
	}
	actions, err := a.buildELBV2ListenerActions(ctx, rule.Actions)
	if err != nil {
		return nil, err
	}
	return &elbv2.Rule{
		Conditions: conditions,
		Actions:    actions,
	}, nil
}

func (a *loadBalancerActuator) buildELBV2ListenerRuleConditions(ctx context.Context, conditions []api.ListenerRuleCondition) ([]*elbv2.RuleCondition, error) {
	result := make([]*elbv2.RuleCondition, 0, len(conditions))
	for _, condition := range conditions {
		elbv2Condition, err := a.buildELBV2ListenerRuleCondition(ctx, condition)
		if err != nil {
			return nil, err
		}
		result = append(result, elbv2Condition)
	}
	return result, nil
}

func (a *loadBalancerActuator) buildELBV2ListenerRuleCondition(ctx context.Context, condition api.ListenerRuleCondition) (*elbv2.RuleCondition, error) {
	switch condition.Field {
	case api.RuleConditionFieldHostHeader:
		{
			return &elbv2.RuleCondition{
				Field: aws.String(api.RuleConditionFieldHostHeader.String()),
				HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
					Values: aws.StringSlice(condition.HostHeader.Values),
				},
			}, nil
		}
	case api.RuleConditionFieldPathPattern:
		{
			return &elbv2.RuleCondition{
				Field: aws.String(api.RuleConditionFieldPathPattern.String()),
				PathPatternConfig: &elbv2.PathPatternConditionConfig{
					Values: aws.StringSlice(condition.PathPattern.Values),
				},
			}, nil
		}
	}
	return nil, errors.Errorf("unknown condition type: %v", condition.Field)
}

func normalizeELBV2ListenerRuleConditions(conditions []*elbv2.RuleCondition) {
	for _, cond := range conditions {
		sort.Slice(cond.Values, func(i, j int) bool { return aws.StringValue(cond.Values[i]) < aws.StringValue(cond.Values[j]) })
		if cond.HostHeaderConfig != nil {
			sort.Slice(cond.HostHeaderConfig.Values, func(i, j int) bool {
				return aws.StringValue(cond.HostHeaderConfig.Values[i]) < aws.StringValue(cond.HostHeaderConfig.Values[j])
			})
		}
		if cond.PathPatternConfig != nil {
			sort.Slice(cond.PathPatternConfig.Values, func(i, j int) bool {
				return aws.StringValue(cond.PathPatternConfig.Values[i]) < aws.StringValue(cond.PathPatternConfig.Values[j])
			})
		}

		cond.Values = nil
	}

	sort.Slice(conditions, func(i, j int) bool {
		return aws.StringValue(conditions[i].Field) < aws.StringValue(conditions[j].Field)
	})
}

func normalizeELBV2ListenerActions(actions []*elbv2.Action) {
	sort.Slice(actions, func(i, j int) bool {
		return aws.Int64Value(actions[i].Order) < aws.Int64Value(actions[j].Order)
	})
}
