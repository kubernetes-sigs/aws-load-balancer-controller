package ingress

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	networking "k8s.io/api/networking/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

const maxConditionValuesCount = int(5)

func (t *defaultModelBuildTask) buildListenerRules(ctx context.Context, lsARN core.StringToken, port int64, protocol elbv2model.Protocol, ingList []ClassifiedIngress) error {
	if t.sslRedirectConfig != nil && protocol == elbv2model.ProtocolHTTP {
		return nil
	}

	var rules []Rule
	for _, ing := range ingList {
		rulesWithReplicateHosts, rulesWithUniqueHost, err := t.classifyRulesByHost(ing.Ing.Spec.Rules)
		if err != nil {
			return err
		}
		if len(rulesWithReplicateHosts) != 0 {
			mergedRules := t.mergePathsAcrossRules(rulesWithReplicateHosts)
			builtMergedRules, err := t.buildMergedListenerRules(ctx, protocol, ing, mergedRules)
			if err != nil {
				return err
			}
			rules = append(rules, builtMergedRules...)
		}
		if len(rulesWithUniqueHost) != 0 {
			builtUnmergedRules, err := t.buildUnmergedListenerRules(ctx, protocol, ing, rulesWithUniqueHost)
			if err != nil {
				return err
			}
			rules = append(rules, builtUnmergedRules...)
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
			Tags:        rule.Tags,
		})
		priority += 1
	}
	return nil
}

// classifyRulesByHost classifies rules based on whether the rule has a unique host
// only the rules with replicate hosts need merge
func (t *defaultModelBuildTask) classifyRulesByHost(rules []networking.IngressRule) ([]networking.IngressRule, []networking.IngressRule, error) {
	var rulesWithReplicateHosts []networking.IngressRule
	var rulesWithUniqueHost []networking.IngressRule
	hostToRulesMap := make(map[string][]networking.IngressRule)
	for _, rule := range rules {
		host := rule.Host
		_, exists := hostToRulesMap[host]
		if exists {
			hostToRulesMap[host] = append(hostToRulesMap[host], rule)
		} else {
			hostToRulesMap[host] = []networking.IngressRule{rule}
		}
	}
	for host, rules := range hostToRulesMap {
		if len(rules) == 1 {
			rulesWithUniqueHost = append(rulesWithUniqueHost, rules...)
		} else if len(rules) > 1 {
			rulesWithReplicateHosts = append(rulesWithReplicateHosts, rules...)
		} else {
			return nil, nil, errors.Errorf("no rules for Host %s", host)
		}
	}
	return rulesWithReplicateHosts, rulesWithUniqueHost, nil
}

// mergePathsAcrossRules generates new rules with paths merged
func (t *defaultModelBuildTask) mergePathsAcrossRules(rules []networking.IngressRule) []networking.IngressRule {
	// iterate the mergeRefMap and append all paths in a group to the rule of the min ruleIdx
	mergePathsRefMap := t.getMergeRuleRefMaps(rules)
	var mergedRule networking.IngressRule
	var mergedRules []networking.IngressRule
	for k, paths := range mergePathsRefMap {
		mergedRule = networking.IngressRule{
			Host: k[0],
			IngressRuleValue: networking.IngressRuleValue{
				HTTP: &networking.HTTPIngressRuleValue{
					Paths: paths,
				},
			},
		}
		mergedRules = append(mergedRules, mergedRule)
	}
	return mergedRules
}

// getMergeRuleRefMaps gets a map to help merge paths of rules by host, pathType and backend
// e.g. {(hostA, pathTypeA, serviceNameA, PortNameA, PortNumberA): [path1, path2,...],
//
//		 (hostB, pathTypeB, serviceNameB, PortNameB, PortNumberB): [path3, path4,...],
//	 	...}
func (t *defaultModelBuildTask) getMergeRuleRefMaps(rules []networking.IngressRule) map[[5]string][]networking.HTTPIngressPath {
	mergePathsRefMap := make(map[[5]string][]networking.HTTPIngressPath)
	//// pathToRuleMap stores {path: ruleIdx} relationship
	//pathToRuleMap := make(map[networking.HTTPIngressPath]int)

	for _, rule := range rules {
		if rule.HTTP == nil {
			continue
		}
		host := rule.Host
		for _, path := range rule.HTTP.Paths {
			//pathToRuleMap[path] = idx
			pathType := ""
			if path.PathType != nil {
				pathType = string(*path.PathType)
			}
			serviceName := path.Backend.Service.Name
			portName := path.Backend.Service.Port.Name
			portNumber := strconv.Itoa(int(path.Backend.Service.Port.Number))
			_, exist := mergePathsRefMap[[5]string{host, pathType, serviceName, portName, portNumber}]
			if !exist {
				mergePathsRefMap[[5]string{host, pathType, serviceName, portName, portNumber}] = []networking.HTTPIngressPath{path}
			} else {
				mergePathsRefMap[[5]string{host, pathType, serviceName, portName, portNumber}] = append(mergePathsRefMap[[5]string{host, pathType, serviceName, portName, portNumber}],
					path)
			}
		}
	}
	return mergePathsRefMap
}

// sortIngressPaths will sort the paths following the strategy:
// all exact match paths come first, no need to sort since exact match has to be unique
// followed by prefix paths, sort by lengths - longer paths get precedence
// followed by ImplementationSpecific paths or paths with no pathType specified, keep the original order
func (t *defaultModelBuildTask) sortIngressPaths(paths []networking.HTTPIngressPath) ([]networking.HTTPIngressPath, error) {
	exactPaths, prefixPaths, implementationSpecificPaths, err := t.classifyIngressPathsByType(paths)
	if err != nil {
		return nil, err
	}
	sortedPaths := exactPaths
	sort.SliceStable(prefixPaths, func(i, j int) bool {
		return len(prefixPaths[i].Path) > len(prefixPaths[j].Path)
	})
	sortedPaths = append(sortedPaths, prefixPaths...)
	sortedPaths = append(sortedPaths, implementationSpecificPaths...)
	return sortedPaths, nil
}

// classifyIngressPathsByType will classify the paths by type Exact, Prefix and ImplementationSpecific
func (t *defaultModelBuildTask) classifyIngressPathsByType(paths []networking.HTTPIngressPath) ([]networking.HTTPIngressPath, []networking.HTTPIngressPath, []networking.HTTPIngressPath, error) {
	var exactPaths []networking.HTTPIngressPath
	var prefixPaths []networking.HTTPIngressPath
	var implementationSpecificPaths []networking.HTTPIngressPath
	for _, path := range paths {
		if path.PathType != nil {
			switch *path.PathType {
			case networking.PathTypeExact:
				exactPaths = append(exactPaths, path)
			case networking.PathTypePrefix:
				prefixPaths = append(prefixPaths, path)
			case networking.PathTypeImplementationSpecific:
				implementationSpecificPaths = append(implementationSpecificPaths, path)
			default:
				return nil, nil, nil, errors.Errorf("unknown pathType for path %s", path.Path)
			}
		} else {
			implementationSpecificPaths = append(implementationSpecificPaths, path)
		}
	}
	return exactPaths, prefixPaths, implementationSpecificPaths, nil
}

func (t *defaultModelBuildTask) buildMergedListenerRules(ctx context.Context, protocol elbv2model.Protocol, ing ClassifiedIngress, rulesToMerge []networking.IngressRule) ([]Rule, error) {
	var rules []Rule
	for _, ruleToMerge := range rulesToMerge {
		if ruleToMerge.HTTP == nil {
			continue
		}
		host := ruleToMerge.Host
		// all the paths in one mergedRule should have the same backend and pathType
		mergedPaths := ruleToMerge.HTTP.Paths
		if len(mergedPaths) == 0 {
			continue
		}
		currPathType := networking.PathTypeImplementationSpecific
		if mergedPaths[0].PathType != nil {
			currPathType = *mergedPaths[0].PathType
		}
		currBackend := ruleToMerge.HTTP.Paths[0].Backend
		var paths []string
		for _, path := range mergedPaths {
			if path.Path != "" {
				paths = append(paths, path.Path)
			}
		}
		enhancedBackend, err := t.enhancedBackendBuilder.Build(ctx, ing.Ing, currBackend,
			WithLoadBackendServices(true, t.backendServices),
			WithLoadAuthConfig(true))
		conditions, err := t.buildRuleConditions(ctx, host, paths, enhancedBackend, currPathType)
		if err != nil {
			return nil, errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
		}
		if err != nil {
			return nil, errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
		}
		actions, err := t.buildActions(ctx, protocol, ing, enhancedBackend)
		if err != nil {
			return nil, errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
		}
		tags, err := t.buildListenerRuleTags(ctx, ing)
		if err != nil {
			return nil, errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
		}
		for _, condition := range conditions {
			rules = append(rules, Rule{
				Conditions: condition,
				Actions:    actions,
				Tags:       tags,
			})
		}
	}
	return rules, nil
}

func (t *defaultModelBuildTask) buildUnmergedListenerRules(ctx context.Context, protocol elbv2model.Protocol, ing ClassifiedIngress, rulesToBuild []networking.IngressRule) ([]Rule, error) {
	var rules []Rule
	for _, ruleToBuild := range rulesToBuild {
		if ruleToBuild.HTTP == nil {
			continue
		}
		paths, err := t.sortIngressPaths(ruleToBuild.HTTP.Paths)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			enhancedBackend, err := t.enhancedBackendBuilder.Build(ctx, ing.Ing, path.Backend,
				WithLoadBackendServices(true, t.backendServices),
				WithLoadAuthConfig(true))
			if err != nil {
				return nil, errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
			}
			pathType := networking.PathTypeImplementationSpecific
			if path.PathType != nil {
				pathType = *path.PathType
			}
			conditions, err := t.buildRuleConditions(ctx, ruleToBuild.Host, []string{path.Path}, enhancedBackend, pathType)
			if err != nil {
				return nil, errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
			}
			actions, err := t.buildActions(ctx, protocol, ing, enhancedBackend)
			if err != nil {
				return nil, errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
			}
			tags, err := t.buildListenerRuleTags(ctx, ing)
			if err != nil {
				return nil, errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
			}
			for _, condition := range conditions {
				rules = append(rules, Rule{
					Conditions: condition,
					Actions:    actions,
					Tags:       tags,
				})
			}

		}
	}
	return rules, nil
}

func (t *defaultModelBuildTask) buildRuleConditions(ctx context.Context, host string,
	pathsToBuild []string, backend EnhancedBackend, pathType networking.PathType) ([][]elbv2model.RuleCondition, error) {
	var hosts []string
	if host != "" {
		hosts = append(hosts, host)
	}
	var paths []string
	pathPatterns, err := t.buildPathPatterns(pathsToBuild, &pathType)
	if err != nil {
		return nil, err
	}
	paths = append(paths, pathPatterns...)
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
	if len(conditions) > 5 {
		return nil, errors.New("A rule can only have at most 5 condition values.")
	}
	// flow over the path values to new conditions if the total count of values exceeds 5
	var builtConditions [][]elbv2model.RuleCondition
	if len(paths) == 0 {
		if len(conditions) == 0 {
			conditions = append(conditions, t.buildPathPatternCondition(ctx, []string{"/*"}))
		}
		builtConditions = append(builtConditions, conditions)
	} else {
		step := maxConditionValuesCount - len(conditions)
		for i := 0; i < len(paths); i += step {
			end := algorithm.GetMin(i+step, len(paths))
			conditionsWithPathPattern := append(conditions, t.buildPathPatternCondition(ctx, paths[i:end]))
			builtConditions = append(builtConditions, conditionsWithPathPattern)
		}
	}
	return builtConditions, nil
}

func (t *defaultModelBuildTask) buildPathPatterns(paths []string, pathType *networking.PathType) ([]string, error) {
	normalizedPathType := networking.PathTypeImplementationSpecific
	if pathType != nil {
		normalizedPathType = *pathType
	}
	var builtPaths []string
	switch normalizedPathType {
	case networking.PathTypeImplementationSpecific:
		for _, path := range paths {
			builtPath, err := t.buildPathPatternsForImplementationSpecificPathType(path)
			if err != nil {
				return nil, err
			}
			builtPaths = append(builtPaths, builtPath...)
		}
		return builtPaths, nil
	case networking.PathTypeExact:
		for _, path := range paths {
			builtPath, err := t.buildPathPatternsForExactPathType(path)
			if err != nil {
				return nil, err
			}
			builtPaths = append(builtPaths, builtPath...)
		}
		return builtPaths, nil
	case networking.PathTypePrefix:
		for _, path := range paths {
			builtPath, err := t.buildPathPatternsForPrefixPathType(path)
			if err != nil {
				return nil, err
			}
			builtPaths = append(builtPaths, builtPath...)
		}
		return builtPaths, nil
	default:
		return nil, errors.Errorf("unsupported pathType: %v", normalizedPathType)
	}
}

// buildPathPatternsForImplementationSpecificPathType will build path patterns for implementationSpecific pathType.
func (t *defaultModelBuildTask) buildPathPatternsForImplementationSpecificPathType(path string) ([]string, error) {
	return []string{path}, nil
}

// buildPathPatternsForExactPathType will build path patterns for exact pathType.
// exact path shouldn't contain any wildcards.
func (t *defaultModelBuildTask) buildPathPatternsForExactPathType(path string) ([]string, error) {
	if strings.ContainsAny(path, "*?") {
		return nil, errors.Errorf("exact path shouldn't contain wildcards: %v", path)
	}
	return []string{path}, nil
}

// buildPathPatternsForPrefixPathType will build path patterns for prefix pathType.
// prefix path shouldn't contain any wildcards.
// with prefixType type, both "/foo" or "/foo/" should match path like "/foo" or "/foo/" or "/foo/bar".
// for above case, we'll generate two path pattern: "/foo/" and "/foo/*".
// an special case is "/", which matches all paths, thus we generate the path pattern as "/*"
func (t *defaultModelBuildTask) buildPathPatternsForPrefixPathType(path string) ([]string, error) {
	if path == "/" {
		return []string{"/*"}, nil
	}
	if strings.ContainsAny(path, "*?") {
		return nil, errors.Errorf("prefix path shouldn't contain wildcards: %v", path)
	}

	normalizedPath := strings.TrimSuffix(path, "/")
	return []string{normalizedPath, normalizedPath + "/*"}, nil
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

func (t *defaultModelBuildTask) buildListenerRuleTags(_ context.Context, ing ClassifiedIngress) (map[string]string, error) {
	ingTags, err := t.buildIngressResourceTags(ing)
	if err != nil {
		return nil, err
	}

	return algorithm.MergeStringMap(t.defaultTags, ingTags), nil
}
