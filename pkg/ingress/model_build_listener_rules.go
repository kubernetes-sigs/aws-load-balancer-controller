package ingress

import (
	"context"
	"fmt"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sort"
	"strings"

	"github.com/pkg/errors"
	networking "k8s.io/api/networking/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func (t *defaultModelBuildTask) buildListenerRules(ctx context.Context, lsARN core.StringToken, port int32, protocol elbv2model.Protocol, ingList []ClassifiedIngress) error {
	if t.sslRedirectConfig != nil && protocol == elbv2model.ProtocolHTTP {
		return nil
	}

	var rules []Rule
	for _, ing := range ingList {
		for _, rule := range ing.Ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			paths, err := t.sortIngressPaths(rule.HTTP.Paths)
			if err != nil {
				return err
			}
			for _, path := range paths {
				enhancedBackend, err := t.enhancedBackendBuilder.Build(ctx, ing.Ing, path.Backend,
					WithLoadBackendServices(true, t.backendServices),
					WithLoadAuthConfig(true))
				if err != nil {
					return errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
				}
				conditions, err := t.buildRuleConditions(ctx, ing, rule, path, enhancedBackend)
				if err != nil {
					return errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
				}
				actions, err := t.buildActions(ctx, protocol, ing, enhancedBackend)
				if err != nil {
					return errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
				}
				transforms, err := t.buildTransforms(ctx, enhancedBackend)
				if err != nil {
					return errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
				}
				tags, err := t.buildListenerRuleTags(ctx, ing)
				if err != nil {
					return errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing.Ing))
				}
				rules = append(rules, Rule{
					Conditions: conditions,
					Actions:    actions,
					Transforms: transforms,
					Tags:       tags,
				})
			}
		}
	}
	optimizedRules, err := t.ruleOptimizer.Optimize(ctx, port, protocol, rules)
	if err != nil {
		return err
	}

	priority := int32(1)
	for _, rule := range optimizedRules {
		ruleResID := fmt.Sprintf("%v:%v", port, priority)
		_ = elbv2model.NewListenerRule(t.stack, ruleResID, elbv2model.ListenerRuleSpec{
			ListenerARN: lsARN,
			Priority:    priority,
			Conditions:  rule.Conditions,
			Actions:     rule.Actions,
			Transforms:  rule.Transforms,
			Tags:        rule.Tags,
		})
		priority += 1
	}

	return nil
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

func (t *defaultModelBuildTask) buildRuleConditions(ctx context.Context, ing ClassifiedIngress, rule networking.IngressRule,
	path networking.HTTPIngressPath, backend EnhancedBackend) ([]elbv2model.RuleCondition, error) {
	// Glob syntax with wildcards (`*` and `?`)
	var hostValues []string
	// Regex syntax
	var hostRegexValues []string
	if rule.Host != "" {
		hostValues = append(hostValues, rule.Host)
	}
	// Glob syntax with wildcards (`*` and `?`)
	var pathValues []string
	// Regex syntax
	var pathRegexValues []string
	if path.Path != "" {
		pathValuePatterns, pathRegexValuesPatterns, err := t.buildPathPatterns(ing, path.Path, path.PathType)
		if err != nil {
			return nil, err
		}
		pathValues = append(pathValues, pathValuePatterns...)
		pathRegexValues = append(pathRegexValues, pathRegexValuesPatterns...)
	}
	var conditions []elbv2model.RuleCondition
	for _, condition := range backend.Conditions {
		switch condition.Field {
		case RuleConditionFieldHostHeader:
			if condition.HostHeaderConfig == nil {
				return nil, errors.New("missing HostHeaderConfig")
			}
			if len(condition.HostHeaderConfig.Values) > 0 {
				hostValues = append(hostValues, condition.HostHeaderConfig.Values...)
			}
			if len(condition.HostHeaderConfig.RegexValues) > 0 {
				hostRegexValues = append(hostRegexValues, condition.HostHeaderConfig.RegexValues...)
			}
		case RuleConditionFieldPathPattern:
			if condition.PathPatternConfig == nil {
				return nil, errors.New("missing PathPatternConfig")
			}
			if len(condition.PathPatternConfig.Values) > 0 {
				pathValues = append(pathValues, condition.PathPatternConfig.Values...)
			}
			if len(condition.PathPatternConfig.RegexValues) > 0 {
				pathRegexValues = append(pathRegexValues, condition.PathPatternConfig.RegexValues...)
			}
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

	// Using client-side validation to enforce that host-name and path-pattern conditions use either Values or RegexValues, never both
	// This error is easy to make with LBC since these can be specified as conditions annotations or as host/path rules
	// but the resulting API error is hard to debug!
	if len(hostValues) != 0 && len(hostRegexValues) != 0 {
		return nil, errors.Errorf("host condition must specify exactly one of Values and RegexValues, got both Values %s and RegexValues %s", hostValues, hostRegexValues)
	}
	if len(pathValues) != 0 && len(pathRegexValues) != 0 {
		return nil, errors.Errorf("path condition must specify exactly one of Values and RegexValues, got both Values %s and RegexValues %s", pathValues, pathRegexValues)
	}

	if len(hostValues) != 0 {
		conditions = append(conditions, t.buildHostHeaderValuesCondition(ctx, hostValues))
	}
	if len(hostRegexValues) != 0 {
		conditions = append(conditions, t.buildHostHeaderRegexValuesCondition(ctx, hostRegexValues))
	}
	if len(pathValues) != 0 {
		conditions = append(conditions, t.buildPathValuesCondition(ctx, pathValues))
	}
	if len(pathRegexValues) != 0 {
		conditions = append(conditions, t.buildPathRegexValuesCondition(ctx, pathRegexValues))
	}
	if len(conditions) == 0 {
		conditions = append(conditions, t.buildPathValuesCondition(ctx, []string{"/*"}))
	}
	return conditions, nil
}

// buildPathPatterns will build ELBv2's path patterns for given path and pathType.
func (t *defaultModelBuildTask) buildPathPatterns(ing ClassifiedIngress, path string, pathType *networking.PathType) ([]string, []string, error) {
	normalizedPathType := networking.PathTypeImplementationSpecific
	if pathType != nil {
		normalizedPathType = *pathType
	}
	switch normalizedPathType {
	case networking.PathTypeImplementationSpecific:
		return t.buildPathPatternsForImplementationSpecificPathType(ing, path)
	case networking.PathTypeExact:
		return t.buildPathPatternsForExactPathType(path)
	case networking.PathTypePrefix:
		return t.buildPathPatternsForPrefixPathType(path)
	default:
		return nil, nil, errors.Errorf("unsupported pathType: %v", normalizedPathType)
	}
}

// buildPathPatternsForImplementationSpecificPathType will build path patterns for implementationSpecific pathType.
func (t *defaultModelBuildTask) buildPathPatternsForImplementationSpecificPathType(ing ClassifiedIngress, path string) ([]string, []string, error) {
	useRegexPathMatch, err := t.getUseRegexPathMatch(ing)

	if err != nil {
		return nil, nil, err
	}

	if useRegexPathMatch {
		// Strip first character (leading `/`)
		// because kubernetes requires paths to start with a `/`, but not required for regex
		return []string{}, []string{path[1:]}, nil
	}

	return []string{path}, []string{}, nil
}

// buildPathPatternsForExactPathType will build path patterns for exact pathType.
// exact path shouldn't contains any wildcards.
func (t *defaultModelBuildTask) buildPathPatternsForExactPathType(path string) ([]string, []string, error) {
	if strings.ContainsAny(path, "*?") {
		return nil, nil, errors.Errorf("exact path shouldn't contain wildcards: %v", path)
	}
	return []string{path}, []string{}, nil
}

// buildPathPatternsForPrefixPathType will build path patterns for prefix pathType.
// prefix path shouldn't contains any wildcards.
// with prefixType type, both "/foo" or "/foo/" should matches path like "/foo" or "/foo/" or "/foo/bar".
// for above case, we'll generate two path pattern: "/foo/" and "/foo/*".
// an special case is "/", which matches all paths, thus we generate the path pattern as "/*"
func (t *defaultModelBuildTask) buildPathPatternsForPrefixPathType(path string) ([]string, []string, error) {
	if path == "/" {
		return []string{"/*"}, []string{}, nil
	}
	if strings.ContainsAny(path, "*?") {
		return nil, nil, errors.Errorf("prefix path shouldn't contain wildcards: %v", path)
	}

	normalizedPath := strings.TrimSuffix(path, "/")
	return []string{normalizedPath, normalizedPath + "/*"}, []string{}, nil
}

func (t *defaultModelBuildTask) buildHTTPHeaderCondition(_ context.Context, condition RuleCondition) (elbv2model.RuleCondition, error) {
	if condition.HTTPHeaderConfig == nil {
		return elbv2model.RuleCondition{}, errors.New("missing HTTPHeaderConfig")
	}
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldHTTPHeader,
		HTTPHeaderConfig: &elbv2model.HTTPHeaderConditionConfig{
			HTTPHeaderName: condition.HTTPHeaderConfig.HTTPHeaderName,
			RegexValues:    condition.HTTPHeaderConfig.RegexValues,
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

func (t *defaultModelBuildTask) buildHostHeaderValuesCondition(_ context.Context, hostValues []string) elbv2model.RuleCondition {
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldHostHeader,
		HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
			Values: hostValues,
		},
	}
}

func (t *defaultModelBuildTask) buildHostHeaderRegexValuesCondition(_ context.Context, hostRegexValues []string) elbv2model.RuleCondition {
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldHostHeader,
		HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
			RegexValues: hostRegexValues,
		},
	}
}

func (t *defaultModelBuildTask) buildPathValuesCondition(_ context.Context, pathValues []string) elbv2model.RuleCondition {
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldPathPattern,
		PathPatternConfig: &elbv2model.PathPatternConditionConfig{
			Values: pathValues,
		},
	}
}

func (t *defaultModelBuildTask) buildPathRegexValuesCondition(_ context.Context, pathRegexValues []string) elbv2model.RuleCondition {
	return elbv2model.RuleCondition{
		Field: elbv2model.RuleConditionFieldPathPattern,
		PathPatternConfig: &elbv2model.PathPatternConditionConfig{
			RegexValues: pathRegexValues,
		},
	}
}

func (t *defaultModelBuildTask) buildListenerRuleTags(_ context.Context, ing ClassifiedIngress) (map[string]string, error) {
	ingTags, err := t.buildIngressResourceTags(ing)
	if err != nil {
		return nil, err
	}

	if t.featureGates.Enabled(config.EnableDefaultTagsLowPriority) {
		return algorithm.MergeStringMap(ingTags, t.defaultTags), nil
	}
	return algorithm.MergeStringMap(t.defaultTags, ingTags), nil
}

// Get whether to use regex path match for ImplementationSpecific path specs
func (t *defaultModelBuildTask) getUseRegexPathMatch(ing ClassifiedIngress) (bool, error) {
	var useRegexPathMatch bool
	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.IngressSuffixUseRegexPathMatch, &useRegexPathMatch, ing.Ing.Annotations)

	if err != nil {
		return false, err
	}

	if !exists {
		// Default to `false`, which is the existing behavior
		return false, nil
	}

	return useRegexPathMatch, nil
}

// buildTransforms builds the transforms for a listener rule from the enhanced backend
func (t *defaultModelBuildTask) buildTransforms(_ context.Context, backend EnhancedBackend) ([]elbv2model.Transform, error) {
	transforms := make([]elbv2model.Transform, 0)
	for _, transform := range backend.Transforms {
		switch transform.Type {
		case TransformTypeHostHeaderRewrite:
			if transform.HostHeaderRewriteConfig == nil {
				return nil, errors.New("missing hostHeaderRewriteConfig")
			}
			var rewrites []elbv2model.RewriteConfig
			for _, rewrite := range transform.HostHeaderRewriteConfig.Rewrites {
				rewrites = append(rewrites, elbv2model.RewriteConfig{
					Regex:   rewrite.Regex,
					Replace: rewrite.Replace,
				})
			}
			transforms = append(transforms, elbv2model.Transform{
				Type: elbv2model.TransformTypeHostHeaderRewrite,
				HostHeaderRewriteConfig: &elbv2model.RewriteConfigObject{
					Rewrites: rewrites,
				},
			})
		case TransformTypeUrlRewrite:
			if transform.UrlRewriteConfig == nil {
				return nil, errors.New("missing urlRewriteConfig")
			}
			var rewrites []elbv2model.RewriteConfig
			for _, rewrite := range transform.UrlRewriteConfig.Rewrites {
				rewrites = append(rewrites, elbv2model.RewriteConfig{
					Regex:   rewrite.Regex,
					Replace: rewrite.Replace,
				})
			}
			transforms = append(transforms, elbv2model.Transform{
				Type: elbv2model.TransformTypeUrlRewrite,
				UrlRewriteConfig: &elbv2model.RewriteConfigObject{
					Rewrites: rewrites,
				},
			})
		default:
			return nil, errors.Errorf("unknown transform type: %v", transform.Type)
		}
	}
	return transforms, nil
}
