package build

import (
	"context"
	"fmt"
	extensions "k8s.io/api/extensions/v1beta1"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
)

// buildListenerRules will append rules into specified listener from specified ingress by port & protocol.
func (b *defaultBuilder) buildListenerRules(ctx context.Context, stack *LoadBalancingStack, groupID ingress.GroupID, ing *extensions.Ingress, port int64, protocol api.Protocol) ([]api.ListenerRule, error) {
	var lsRules []api.ListenerRule

	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}

		for _, path := range rule.HTTP.Paths {
			conditions := b.buildListenerRuleConditions(ctx, ing, rule, path)
			actions, err := b.buildListenerActions(ctx, stack, groupID, ing, path.Backend, protocol)
			if err != nil {
				return nil, err
			}
			if createsRedirectLoop(port, protocol, conditions, actions) {
				continue
			}
			lsRules = append(lsRules, api.ListenerRule{
				Conditions: conditions,
				Actions:    actions,
			})
		}
	}
	return lsRules, nil
}

func (b *defaultBuilder) buildListenerRuleConditions(ctx context.Context, ing *extensions.Ingress, rule extensions.IngressRule, path extensions.HTTPIngressPath) []api.ListenerRuleCondition {
	var conditions []api.ListenerRuleCondition
	if rule.Host != "" {
		conditions = append(conditions, buildHostHeaderCondition(rule.Host))
	}
	if path.Path != "" {
		conditions = append(conditions, buildPathPatternCondition(path.Path))
	}

	// AWS requires at least one condition per rule.
	if len(conditions) == 0 {
		conditions = append(conditions, buildPathPatternCondition("/*"))
	}
	return conditions
}

func buildHostHeaderCondition(host string) api.ListenerRuleCondition {
	return api.ListenerRuleCondition{
		Field: api.RuleConditionFieldHostHeader,
		HostHeader: &api.HostHeaderConditionConfig{
			Values: []string{host},
		},
	}
}

func buildPathPatternCondition(pathPtn string) api.ListenerRuleCondition {
	return api.ListenerRuleCondition{
		Field: api.RuleConditionFieldPathPattern,
		PathPattern: &api.PathPatternConditionConfig{
			Values: []string{pathPtn},
		},
	}
}

// createsRedirectLoop checks whether specified rule creates redirectionLoop.
// This is an old hack to support HTTP->HTTPS redirection feature as below:
// If an rule creates redirect loop on specific listener(e.g. redirect to HTTPS on HTTPS listener), it will be ignored.
func createsRedirectLoop(port int64, protocol api.Protocol, conditions []api.ListenerRuleCondition, actions []api.ListenerAction) bool {
	for _, action := range actions {
		rc := action.Redirect
		if rc == nil {
			continue
		}

		var host, path string
		for _, condition := range conditions {
			switch condition.Field {
			case api.RuleConditionFieldHostHeader:
				{
					host = condition.HostHeader.Values[0]
				}
			case api.RuleConditionFieldPathPattern:
				{
					path = condition.PathPattern.Values[0]
				}
			}
		}

		if rc.Port != RedirectOriginalPort && rc.Port != fmt.Sprintf("%d", port) {
			return false
		}
		if rc.Protocol != RedirectOriginalProtocol && rc.Protocol != string(protocol) {
			return false
		}
		if rc.Host != RedirectOriginalHost && rc.Host != host {
			return false
		}
		if rc.Path != RedirectOriginalPath && rc.Path != path {
			return false
		}
		if rc.Query != RedirectOriginalQuery {
			return false
		}
		return true
	}
	return false
}
