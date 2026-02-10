package routeutils

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
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
		return []elbv2model.RuleCondition{
			{
				Field: elbv2model.RuleConditionFieldPathPattern,
				PathPatternConfig: &elbv2model.PathPatternConditionConfig{
					RegexValues: append(pathValues, pathValue),
				},
			},
		}, nil
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
					Values:         generateValuesFromMatchHeaderValue(header.Value),
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
					Values:         generateValuesFromMatchHeaderValue(header.Value),
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

func buildSourceIpCondition(condition elbv2gw.ListenerRuleCondition) []elbv2model.RuleCondition {
	return []elbv2model.RuleCondition{
		{
			Field: elbv2model.RuleConditionField(condition.Field),
			SourceIPConfig: &elbv2model.SourceIPConditionConfig{
				Values: condition.SourceIPConfig.Values,
			},
		},
	}
}

// BuildSourceIpInCondition : takes source ip configuration from listener rule configuration CRD, then AND it to condition list
func BuildSourceIpInCondition(ruleWithPrecedence RulePrecedence, conditionsList []elbv2model.RuleCondition) []elbv2model.RuleCondition {
	rule := ruleWithPrecedence.CommonRulePrecedence.Rule
	matchIndex := ruleWithPrecedence.CommonRulePrecedence.MatchIndexInRule
	if rule.GetListenerRuleConfig() != nil {
		conditionsFromRuleConfig := rule.GetListenerRuleConfig().Spec.Conditions
		for _, condition := range conditionsFromRuleConfig {
			switch condition.Field {
			case elbv2gw.ListenerRuleConditionFieldSourceIP:
				sourceIpCondition := buildSourceIpCondition(condition)
				if condition.MatchIndexes == nil {
					conditionsList = append(conditionsList, sourceIpCondition...)
				} else {
					for _, index := range *condition.MatchIndexes {
						if index == matchIndex {
							conditionsList = append(conditionsList, sourceIpCondition...)
						}
					}
				}
			}
		}
	}
	return conditionsList
}

// generateValuesFromMatchHeaderValue takes in header value from route match
// returns list of values
// for a given HTTPHeaderName/GRPCHeaderName, ALB rule can accept a list of values. However, gateway route headers only accept one value per name, and name cannot duplicate.
func generateValuesFromMatchHeaderValue(headerValue string) []string {
	var values []string
	var current strings.Builder

	for i := 0; i < len(headerValue); i++ {
		if headerValue[i] == '\\' && i+1 < len(headerValue) {
			// Escape sequence - add the escaped character literally
			current.WriteByte(headerValue[i+1])
			i++ // skip the escaped character
		} else if headerValue[i] == ',' {
			// Unescaped comma - split here
			values = append(values, current.String())
			current.Reset()
		} else {
			// Regular character
			current.WriteByte(headerValue[i])
		}
	}

	values = append(values, current.String())
	return values
}
