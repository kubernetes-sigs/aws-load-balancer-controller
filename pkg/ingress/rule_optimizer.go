package ingress

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

type Rule struct {
	Conditions []elbv2model.RuleCondition
	Actions    []elbv2model.Action
	Tags       map[string]string
}

// RuleOptimizer will optimize the listener Rules for a single Listener.
type RuleOptimizer interface {
	Optimize(ctx context.Context, port int64, protocol elbv2model.Protocol, rules []Rule) ([]Rule, error)
}

// NewDefaultRuleOptimizer constructs new defaultRuleOptimizer.
func NewDefaultRuleOptimizer(logger logr.Logger) *defaultRuleOptimizer {
	return &defaultRuleOptimizer{
		logger: logger,
	}
}

var _ RuleOptimizer = &defaultRuleOptimizer{}

// default implementation for RuleOptimizer.
// This rule optimizer exists in order to be backwards-compatible with our hacky "actions.ssl-redirect" annotation.
// when users used `actions.ssl-redirect` annotation to define a redirect rule on Ingresses with both HTTP and HTTPS listener.
// we'll omit the redirect rule on HTTPS listener as it would have caused infinite redirect loop.
// also, we omit additional rules after the redirect rule on the HTTP listener to reduce elbv2 rule usage.
//
// The semantic of this rule optimizer is intended to be generic while supports above use case.
//   * It will omit any redirect rules that would result in a infinite redirect loop.
//   * it will omit any rules that take priority by a redirect rule with a super set of conditions
//  	(ideally this could applies to other action type as well, but we only consider redirect action for now)
type defaultRuleOptimizer struct {
	logger logr.Logger
}

func (o *defaultRuleOptimizer) Optimize(_ context.Context, port int64, protocol elbv2model.Protocol, rules []Rule) ([]Rule, error) {
	optimizedRules := o.omitInfiniteRedirectRules(port, protocol, rules)
	optimizedRules = o.omitOvershadowedRulesAfterRedirectRules(optimizedRules)
	return optimizedRules, nil
}

func (o *defaultRuleOptimizer) omitInfiniteRedirectRules(port int64, protocol elbv2model.Protocol, rules []Rule) []Rule {
	var optimizedRules []Rule
	for _, rule := range rules {
		if isInfiniteRedirectRule(port, protocol, rule) {
			continue
		}
		optimizedRules = append(optimizedRules, rule)
	}
	return optimizedRules
}

func (o *defaultRuleOptimizer) omitOvershadowedRulesAfterRedirectRules(rules []Rule) []Rule {
	var optimizedRules []Rule
	for _, rule := range rules {
		ruleIsOvershadowed := false
		for _, existingRule := range optimizedRules {
			// we only consider a existing redirect rule for now.
			if redirectActionCFG := findRedirectActionConfig(existingRule.Actions); redirectActionCFG == nil {
				continue
			}

			if isSupersetConditions(existingRule.Conditions, rule.Conditions) {
				ruleIsOvershadowed = true
				break
			}
		}

		if !ruleIsOvershadowed {
			optimizedRules = append(optimizedRules, rule)
		}
	}
	return optimizedRules
}

// isInfiniteRedirectRule checks whether specified rule will cause a infinite redirect loop.
func isInfiniteRedirectRule(port int64, protocol elbv2model.Protocol, rule Rule) bool {
	redirectActionCFG := findRedirectActionConfig(rule.Actions)
	if redirectActionCFG == nil {
		return false
	}

	ruleHosts := sets.NewString()
	rulePaths := sets.NewString()
	for _, condition := range rule.Conditions {
		switch {
		case condition.Field == elbv2model.RuleConditionFieldHostHeader && condition.HostHeaderConfig != nil:
			ruleHosts.Insert(condition.HostHeaderConfig.Values...)
		case condition.Field == elbv2model.RuleConditionFieldPathPattern && condition.PathPatternConfig != nil:
			rulePaths.Insert(condition.PathPatternConfig.Values...)
		}
	}

	if redirectActionCFG.Host != nil {
		redirectHost := awssdk.StringValue(redirectActionCFG.Host)
		if redirectHost != "#{host}" && !ruleHosts.Has(redirectHost) {
			return false
		}
	}
	if redirectActionCFG.Path != nil {
		redirectPath := awssdk.StringValue(redirectActionCFG.Path)
		if redirectPath != "/#{path}" && !rulePaths.Has(redirectPath) {
			return false
		}
	}
	if redirectActionCFG.Port != nil {
		redirectPort := awssdk.StringValue(redirectActionCFG.Port)
		rulePort := fmt.Sprintf("%v", port)
		if redirectPort != "#{port}" && redirectPort != rulePort {
			return false
		}
	}
	if redirectActionCFG.Protocol != nil {
		redirectProtocol := awssdk.StringValue(redirectActionCFG.Protocol)
		if redirectProtocol != "#{protocol}" && redirectProtocol != string(protocol) {
			return false
		}
	}
	if redirectActionCFG.Query != nil {
		redirectQuery := awssdk.StringValue(redirectActionCFG.Query)
		if redirectQuery != "#{query}" {
			return false
		}
	}

	return true
}

// isSupersetConditions checks whether lhsConditions is a Superset of rhsConditions.
func isSupersetConditions(lhsConditions []elbv2model.RuleCondition, rhsConditions []elbv2model.RuleCondition) bool {
	lhsHosts := sets.NewString()
	lhsPaths := sets.NewString()
	for _, condition := range lhsConditions {
		switch {
		case condition.Field == elbv2model.RuleConditionFieldHostHeader && condition.HostHeaderConfig != nil:
			lhsHosts.Insert(condition.HostHeaderConfig.Values...)
		case condition.Field == elbv2model.RuleConditionFieldPathPattern && condition.PathPatternConfig != nil:
			lhsPaths.Insert(condition.PathPatternConfig.Values...)
		default:
			// if there are any other conditions, then we treat it as not superset.
			return false
		}
	}

	rhsHosts := sets.NewString()
	rhsPaths := sets.NewString()
	for _, condition := range rhsConditions {
		switch {
		case condition.Field == elbv2model.RuleConditionFieldHostHeader && condition.HostHeaderConfig != nil:
			rhsHosts.Insert(condition.HostHeaderConfig.Values...)
		case condition.Field == elbv2model.RuleConditionFieldPathPattern && condition.PathPatternConfig != nil:
			rhsPaths.Insert(condition.PathPatternConfig.Values...)
		}
	}

	hostIsSuperset := len(lhsHosts) == 0 || (len(rhsHosts) != 0 && lhsHosts.IsSuperset(rhsHosts))
	pathsIsSuperset := len(lhsPaths) == 0 || lhsPaths.Has("/*") || (len(rhsPaths) != 0 && lhsPaths.IsSuperset(rhsPaths))
	return hostIsSuperset && pathsIsSuperset
}

// findRedirectActionConfig finds redirectActionConfig from list of actions if any.
func findRedirectActionConfig(actions []elbv2model.Action) *elbv2model.RedirectActionConfig {
	var redirectActionCFG *elbv2model.RedirectActionConfig
	for _, action := range actions {
		if action.Type == elbv2model.ActionTypeRedirect {
			redirectActionCFG = action.RedirectConfig
			break
		}
	}
	return redirectActionCFG
}
