package routeutils

import (
	"fmt"
	"github.com/pkg/errors"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strings"
)

// BuildHttpRuleConditions each match will be mapped to a ruleCondition, conditions within same match will be ANDed
func BuildHttpRuleConditions(rule RulePrecedence) ([]elbv2model.RuleCondition, error) {
	match := rule.HTTPMatch
	hostnamesStringList := rule.CommonRulePrecedence.Hostnames
	var conditions []elbv2model.RuleCondition
	if hostnamesStringList != nil && len(hostnamesStringList) > 0 {
		conditions = append(conditions, elbv2model.RuleCondition{
			Field: elbv2model.RuleConditionFieldHostHeader,
			HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
				Values: hostnamesStringList,
			},
		})
	}
	if match.Path != nil {
		pathCondition, err := buildHttpPathCondition(match.Path)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, pathCondition...)
	}
	if match.Headers != nil && len(match.Headers) > 0 {
		headerConditions := buildHttpHeaderCondition(match.Headers)
		conditions = append(conditions, headerConditions...)
	}
	if match.QueryParams != nil && len(match.QueryParams) > 0 {
		queryParamConditions := buildHttpQueryParamCondition(match.QueryParams)
		conditions = append(conditions, queryParamConditions...)
	}
	if match.Method != nil {
		methodCondition := buildHttpMethodCondition(match.Method)
		conditions = append(conditions, methodCondition...)
	}
	return conditions, nil
}

func buildHttpPathCondition(path *gwv1.HTTPPathMatch) ([]elbv2model.RuleCondition, error) {
	// Path Type will never be nil, default is prefixPath
	// Path Value will never be nil, default is "/"
	pathType := *path.Type
	pathValue := *path.Value
	var pathValues []string

	// prefix path shouldn't contain any wildcards (*?).
	// with prefixType type, the paths `/abc`, `/abc/`, and `/abc/def` would all match the prefix `/abc`, but the path `/abcd` would not.
	// therefore, for prefixType, we'll generate two path pattern: "/abc/" and "/abc/*".
	// a special case is "/", which matches all paths, thus we generate the path pattern as "/*"
	if pathType == gwv1.PathMatchPathPrefix {
		if strings.ContainsAny(pathValue, "*?") {
			return nil, errors.Errorf("prefix path shouldn't contain wildcards: %v", pathValue)
		}
		if pathValue == "/" {
			pathValues = append(pathValues, "/*")
		} else {
			trimmedPath := strings.TrimSuffix(pathValue, "/")
			pathValues = append(pathValues, trimmedPath, trimmedPath+"/*")
		}

	}

	// exact path shouldn't contain any wildcards.
	if pathType == gwv1.PathMatchExact {
		if strings.ContainsAny(pathValue, "*?") {
			return nil, errors.Errorf("exact path shouldn't contain wildcards: %v", path)
		}
		pathValues = append(pathValues, pathValue)
	}

	// for regex match, we do not need special processing, it will be taken as it is
	if pathType == gwv1.PathMatchRegularExpression {
		pathValues = append(pathValues, pathValue)
	}

	return []elbv2model.RuleCondition{
		{
			Field: elbv2model.RuleConditionFieldPathPattern,
			PathPatternConfig: &elbv2model.PathPatternConditionConfig{
				Values: pathValues,
			},
		},
	}, nil
}

func buildHttpHeaderCondition(headers []gwv1.HTTPHeaderMatch) []elbv2model.RuleCondition {
	var conditions []elbv2model.RuleCondition
	for _, header := range headers {
		headerCondition := []elbv2model.RuleCondition{
			{
				Field: elbv2model.RuleConditionFieldHTTPHeader,
				HTTPHeaderConfig: &elbv2model.HTTPHeaderConditionConfig{
					HTTPHeaderName: string(header.Name),
					// for a given HTTPHeaderName, ALB rule can accept a list of values. However, gateway route headers only accept one value per name, and name cannot duplicate.
					Values: []string{header.Value},
				},
			},
		}
		conditions = append(conditions, headerCondition...)
	}
	return conditions
}

func buildHttpQueryParamCondition(queryParams []gwv1.HTTPQueryParamMatch) []elbv2model.RuleCondition {
	var conditions []elbv2model.RuleCondition
	for _, query := range queryParams {
		keyName := string(query.Name)
		queryCondition := []elbv2model.RuleCondition{
			{
				Field: elbv2model.RuleConditionFieldQueryString,
				QueryStringConfig: &elbv2model.QueryStringConditionConfig{
					Values: []elbv2model.QueryStringKeyValuePair{
						{
							Key:   &keyName,
							Value: query.Value,
						},
					},
				},
			},
		}
		conditions = append(conditions, queryCondition...)
	}
	return conditions
}

func buildHttpMethodCondition(method *gwv1.HTTPMethod) []elbv2model.RuleCondition {
	return []elbv2model.RuleCondition{
		{
			Field: elbv2model.RuleConditionFieldHTTPRequestMethod,
			HTTPRequestMethodConfig: &elbv2model.HTTPRequestMethodConditionConfig{
				Values: []string{string(*method)},
			},
		},
	}
}

func BuildGrpcRuleConditions(rule RulePrecedence) ([]elbv2model.RuleCondition, error) {
	// handle host header
	hostnamesStringList := rule.CommonRulePrecedence.Hostnames
	var conditions []elbv2model.RuleCondition
	if hostnamesStringList != nil && len(hostnamesStringList) > 0 {
		conditions = append(conditions, elbv2model.RuleCondition{
			Field: elbv2model.RuleConditionFieldHostHeader,
			HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
				Values: hostnamesStringList,
			},
		})
	}

	match := rule.GRPCMatch

	if match == nil {
		// If Method field is not specified, all services and methods will match.
		conditions = append(conditions, elbv2model.RuleCondition{
			Field: elbv2model.RuleConditionFieldPathPattern,
			PathPatternConfig: &elbv2model.PathPatternConditionConfig{
				Values: []string{"/*"},
			},
		})
		return conditions, nil
	}

	// handle method match
	if match.Method == nil {
		conditions = append(conditions, elbv2model.RuleCondition{
			Field: elbv2model.RuleConditionFieldPathPattern,
			PathPatternConfig: &elbv2model.PathPatternConditionConfig{
				Values: []string{"/*"},
			},
		})
	} else {
		methodCondition, err := buildGrpcMethodCondition(match.Method)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, methodCondition...)
	}

	// handle header match
	if match.Headers != nil && len(match.Headers) > 0 {
		headerConditions := buildGrpcHeaderCondition(match.Headers)
		conditions = append(conditions, headerConditions...)
	}

	return conditions, nil
}

func buildGrpcHeaderCondition(headers []gwv1.GRPCHeaderMatch) []elbv2model.RuleCondition {
	var conditions []elbv2model.RuleCondition
	for _, header := range headers {
		headerCondition := []elbv2model.RuleCondition{
			{
				Field: elbv2model.RuleConditionFieldHTTPHeader,
				HTTPHeaderConfig: &elbv2model.HTTPHeaderConditionConfig{
					HTTPHeaderName: string(header.Name),
					// for a given HTTPHeaderName, ALB rule can accept a list of values. However, gateway route headers only accept one value per name, and name cannot duplicate.
					Values: []string{header.Value},
				},
			},
		}
		conditions = append(conditions, headerCondition...)
	}
	return conditions
}

// buildGrpcMethodCondition - we handle regex and exact type in same way since regex is taken as-is
// Exact method type will not accept wildcards - this is checked by kubebuilder validation
// Regular expression method type only works with wildcard * and ? for now
func buildGrpcMethodCondition(method *gwv1.GRPCMethodMatch) ([]elbv2model.RuleCondition, error) {
	var pathValue string
	if method.Service != nil && method.Method != nil {
		pathValue = fmt.Sprintf("/%s/%s", *method.Service, *method.Method)
	} else if method.Service != nil {
		pathValue = fmt.Sprintf("/%s/*", *method.Service)
	} else if method.Method != nil {
		pathValue = fmt.Sprintf("/*/%s", *method.Method)
	} else {
		return nil, errors.Errorf("Invalid grpc method match: %v", method)
	}

	return []elbv2model.RuleCondition{
		{
			Field: elbv2model.RuleConditionFieldPathPattern,
			PathPatternConfig: &elbv2model.PathPatternConditionConfig{
				Values: []string{pathValue},
			},
		},
	}, nil
}

// BuildHttpRuleActionsBasedOnFilter only request redirect is supported, header modification is limited due to ALB support level.
func BuildHttpRuleActionsBasedOnFilter(filters []gwv1.HTTPRouteFilter) ([]elbv2model.Action, error) {
	for _, filter := range filters {
		switch filter.Type {
		case gwv1.HTTPRouteFilterRequestRedirect:
			return buildHttpRedirectAction(filter.RequestRedirect)
		default:
			return nil, errors.Errorf("Unsupported filter type: %v. Only request redirect is supported. To specify header modification, please configure it through LoadBalancerConfiguration.", filter.Type)
		}
	}
	return nil, nil
}

// buildHttpRedirectAction configure filter attributes to RedirectActionConfig
// gateway api has no attribute to specify query
func buildHttpRedirectAction(filter *gwv1.HTTPRequestRedirectFilter) ([]elbv2model.Action, error) {
	isComponentSpecified := false
	var statusCode string
	if filter.StatusCode != nil {
		statusCodeStr := fmt.Sprintf("HTTP_%d", *filter.StatusCode)
		statusCode = statusCodeStr
	}

	var port *string
	if filter.Port != nil {
		portStr := fmt.Sprintf("%d", *filter.Port)
		port = &portStr
		isComponentSpecified = true
	}

	var protocol *string
	if filter.Scheme != nil {
		upperScheme := strings.ToUpper(*filter.Scheme)
		if upperScheme != "HTTP" && upperScheme != "HTTPS" {
			return nil, errors.Errorf("unsupported redirect scheme: %v", upperScheme)
		}
		protocol = &upperScheme
		isComponentSpecified = true
	}

	var path *string
	if filter.Path != nil {
		if filter.Path.ReplaceFullPath != nil {
			pathValue := *filter.Path.ReplaceFullPath
			if strings.ContainsAny(pathValue, "*?") {
				return nil, errors.Errorf("ReplaceFullPath shouldn't contain wildcards: %v", pathValue)
			}
			path = filter.Path.ReplaceFullPath
			isComponentSpecified = true
		} else if filter.Path.ReplacePrefixMatch != nil {
			pathValue := *filter.Path.ReplacePrefixMatch
			if strings.ContainsAny(pathValue, "*?") {
				return nil, errors.Errorf("ReplacePrefixMatch shouldn't contain wildcards: %v", pathValue)
			}
			processedPath := fmt.Sprintf("%s/*", pathValue)
			path = &processedPath
			isComponentSpecified = true
		}
	}

	var hostname *string
	if filter.Hostname != nil {
		hostname = (*string)(filter.Hostname)
		isComponentSpecified = true
	}

	if !isComponentSpecified {
		return nil, errors.Errorf("To avoid a redirect loop, you must modify at least one of the following components: protocol, port, hostname or path.")
	}

	action := elbv2model.Action{
		Type: elbv2model.ActionTypeRedirect,
		RedirectConfig: &elbv2model.RedirectActionConfig{
			Host:       hostname,
			Path:       path,
			Port:       port,
			Protocol:   protocol,
			StatusCode: statusCode,
		},
	}
	return []elbv2model.Action{action}, nil
}
