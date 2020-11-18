package ingress

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	networking "k8s.io/api/networking/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func (t *defaultModelBuildTask) buildListenerRules(ctx context.Context, lsARN core.StringToken, port int64, protocol elbv2model.Protocol, ingList []*networking.Ingress) error {
	var rules []Rule
	for _, ing := range ingList {
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				enhancedBackend, err := t.enhancedBackendBuilder.Build(ctx, ing, path.Backend)
				if err != nil {
					return errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing))
				}
				conditions, err := t.buildRuleConditions(ctx, rule, path, enhancedBackend)
				if err != nil {
					return errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing))
				}
				actions, err := t.buildActions(ctx, protocol, ing, enhancedBackend)
				if err != nil {
					return errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing))
				}
				rules = append(rules, Rule{
					Conditions: conditions,
					Actions:    actions,
				})
			}
		}
	}
	optimizedRules, err := t.ruleOptimizer.Optimize(ctx, port, protocol, rules)
	if err != nil {
		return err
	}

	priority := int64(1)
	for _, rule := range optimizedRules {
		ruleResID := fmt.Sprintf("%v:%v", port, priority)
		_ = elbv2model.NewListenerRule(t.stack, ruleResID, elbv2model.ListenerRuleSpec{
			ListenerARN: lsARN,
			Priority:    priority,
			Conditions:  rule.Conditions,
			Actions:     rule.Actions,
		})
		priority += 1
	}

	return nil
}

func (t *defaultModelBuildTask) buildRuleConditions(ctx context.Context, rule networking.IngressRule,
	path networking.HTTPIngressPath, backend EnhancedBackend) ([]elbv2model.RuleCondition, error) {
	var hosts []string
	if rule.Host != "" {
		hosts = append(hosts, rule.Host)
	}
	var paths []string
	if path.Path != "" {
		paths = append(paths, path.Path)
	}
	var conditions []elbv2model.RuleCondition
	for _, condition := range backend.Conditions {
		switch condition.Field {
		case RuleConditionFieldHostHeader:
			if condition.HostHeaderConfig == nil {
				return nil, errors.New("missing HostHeaderConfig")
			}
			hosts = append(hosts, condition.HostHeaderConfig.Values...)
		case RuleConditionFieldPathPattern:
			if condition.PathPatternConfig == nil {
				return nil, errors.New("missing PathPatternConfig")
			}
			paths = append(paths, condition.PathPatternConfig.Values...)
		case RuleConditionFieldHTTPHeader:
			httpHeaderCondition, err := t.buildHTTPHeaderCondition(ctx, condition)
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, httpHeaderCondition)
		case RuleConditionFieldHTTPRequestMethod:
			httpRequestMethodCondition, err := t.buildHTTPRequestMethodCondition(ctx, condition)
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, httpRequestMethodCondition)
		case RuleConditionFieldQueryString:
			queryStringCondition, err := t.buildQueryStringCondition(ctx, condition)
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, queryStringCondition)
		case RuleConditionFieldSourceIP:
			sourceIPCondition, err := t.buildSourceIPCondition(ctx, condition)
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, sourceIPCondition)
		}
	}
	if len(hosts) != 0 {
		conditions = append(conditions, t.buildHostHeaderCondition(ctx, hosts))
	}
	if len(paths) != 0 {
		conditions = append(conditions, t.buildPathPatternCondition(ctx, paths))
	}
	if len(conditions) == 0 {
		conditions = append(conditions, t.buildPathPatternCondition(ctx, []string{"/*"}))
	}
	return conditions, nil
}

func (t *defaultModelBuildTask) buildHTTPHeaderCondition(_ context.Context, condition RuleCondition) (elbv2model.RuleCondition, error) {
	if condition.HTTPHeaderConfig == nil {
		return elbv2model.RuleCondition{}, errors.New("missing HTTPHeaderConfig")
	}
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldHTTPHeader,
		HTTPHeaderConfig: &elbv2model.HTTPHeaderConditionConfig{
			HTTPHeaderName: condition.HTTPHeaderConfig.HTTPHeaderName,
			Values:         condition.HTTPHeaderConfig.Values,
		},
	}, nil
}

func (t *defaultModelBuildTask) buildHTTPRequestMethodCondition(_ context.Context, condition RuleCondition) (elbv2model.RuleCondition, error) {
	if condition.HTTPRequestMethodConfig == nil {
		return elbv2model.RuleCondition{}, errors.New("missing HTTPRequestMethodConfig")
	}
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldHTTPRequestMethod,
		HTTPRequestMethodConfig: &elbv2model.HTTPRequestMethodConditionConfig{
			Values: condition.HTTPRequestMethodConfig.Values,
		},
	}, nil
}

func (t *defaultModelBuildTask) buildQueryStringCondition(_ context.Context, condition RuleCondition) (elbv2model.RuleCondition, error) {
	if condition.QueryStringConfig == nil {
		return elbv2model.RuleCondition{}, errors.New("missing QueryStringConfig")
	}
	var values []elbv2model.QueryStringKeyValuePair
	for _, value := range condition.QueryStringConfig.Values {
		values = append(values, elbv2model.QueryStringKeyValuePair{
			Key:   value.Key,
			Value: value.Value,
		})
	}
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldQueryString,
		QueryStringConfig: &elbv2model.QueryStringConditionConfig{
			Values: values,
		},
	}, nil
}

func (t *defaultModelBuildTask) buildSourceIPCondition(_ context.Context, condition RuleCondition) (elbv2model.RuleCondition, error) {
	if condition.SourceIPConfig == nil {
		return elbv2model.RuleCondition{}, errors.New("missing SourceIPConfig")
	}
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldSourceIP,
		SourceIPConfig: &elbv2model.SourceIPConditionConfig{
			Values: condition.SourceIPConfig.Values,
		},
	}, nil
}

func (t *defaultModelBuildTask) buildHostHeaderCondition(_ context.Context, hosts []string) elbv2model.RuleCondition {
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldHostHeader,
		HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
			Values: hosts,
		},
	}
}

func (t *defaultModelBuildTask) buildPathPatternCondition(_ context.Context, paths []string) elbv2model.RuleCondition {
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldPathPattern,
		PathPatternConfig: &elbv2model.PathPatternConditionConfig{
			Values: paths,
		},
	}
}
