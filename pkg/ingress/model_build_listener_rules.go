package ingress

import (
	"context"
	"errors"
	"fmt"
	networking "k8s.io/api/networking/v1beta1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
)

func (b *defaultModelBuilder) buildListenerRules(ctx context.Context, stack core.Stack, ingGroupID GroupID, port int64, protocol elbv2model.Protocol, lsARN core.StringToken, tgByID map[string]*elbv2model.TargetGroup, ingList []*networking.Ingress) error {
	priority := int64(1)
	for _, ing := range ingList {
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				enhancedBackend, err := b.enhancedBackendBuilder.Build(ctx, ing, path.Backend)
				if err != nil {
					return err
				}
				conditions, err := b.buildConditions(ctx, port, protocol, rule, path, enhancedBackend)
				if err != nil {
					return err
				}
				actions, err := b.buildActions(ctx, stack, ingGroupID, tgByID, protocol, ing, enhancedBackend)
				if err != nil {
					return err
				}
				ruleResID := fmt.Sprintf("%v", priority)
				_ = elbv2model.NewListenerRule(stack, ruleResID, elbv2model.ListenerRuleSpec{
					ListenerARN: lsARN,
					Priority:    priority,
					Actions:     actions,
					Conditions:  conditions,
				})
				priority += 1
			}
		}
	}
	return nil
}

func (b *defaultModelBuilder) buildConditions(ctx context.Context, port int64, protocol elbv2model.Protocol,
	rule networking.IngressRule, path networking.HTTPIngressPath, backend EnhancedBackend) ([]elbv2model.RuleCondition, error) {
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
			httpHeaderCondition, err := b.buildHTTPHeaderCondition(ctx, condition)
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, httpHeaderCondition)
		case RuleConditionFieldHTTPRequestMethod:
			httpRequestMethodCondition, err := b.buildHTTPRequestMethodCondition(ctx, condition)
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, httpRequestMethodCondition)
		case RuleConditionFieldQueryString:
			queryStringCondition, err := b.buildQueryStringCondition(ctx, condition)
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, queryStringCondition)
		case RuleConditionFieldSourceIP:
			sourceIPCondition, err := b.buildSourceIPCondition(ctx, condition)
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, sourceIPCondition)
		}
	}
	if len(hosts) != 0 {
		conditions = append(conditions, b.buildHostHeaderCondition(ctx, hosts))
	}
	if len(paths) != 0 {
		conditions = append(conditions, b.buildPathPatternCondition(ctx, paths))
	}
	if len(conditions) == 0 {
		conditions = append(conditions, b.buildPathPatternCondition(ctx, []string{"/*"}))
	}
	return conditions, nil
}

func (b *defaultModelBuilder) buildHTTPHeaderCondition(ctx context.Context, condition RuleCondition) (elbv2model.RuleCondition, error) {
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

func (b *defaultModelBuilder) buildHTTPRequestMethodCondition(ctx context.Context, condition RuleCondition) (elbv2model.RuleCondition, error) {
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

func (b *defaultModelBuilder) buildQueryStringCondition(ctx context.Context, condition RuleCondition) (elbv2model.RuleCondition, error) {
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

func (b *defaultModelBuilder) buildSourceIPCondition(ctx context.Context, condition RuleCondition) (elbv2model.RuleCondition, error) {
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

func (b *defaultModelBuilder) buildHostHeaderCondition(ctx context.Context, hosts []string) elbv2model.RuleCondition {
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldHostHeader,
		HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
			Values: hosts,
		},
	}
}

func (b *defaultModelBuilder) buildPathPatternCondition(ctx context.Context, paths []string) elbv2model.RuleCondition {
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldPathPattern,
		PathPatternConfig: &elbv2model.PathPatternConditionConfig{
			Values: paths,
		},
	}
}
