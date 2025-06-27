package routeutils

import (
	"github.com/pkg/errors"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strings"
)

// BuildHttpRuleConditions each match will be mapped to a ruleCondition, conditions within same match will be ANDed
func BuildHttpRuleConditions(rule RulePrecedence) ([]elbv2model.RuleCondition, error) {
	match := rule.HTTPMatch
	hostnamesStringList := rule.Hostnames
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
					Values:         []string{header.Value},
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

// TODO: implement it for GRPCRoute
func buildGrpcRouteRuleConditions(matches RouteRule) ([][]elbv2model.RuleCondition, error) {
	var conditions [][]elbv2model.RuleCondition
	return conditions, nil
}
