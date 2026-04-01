package translate

import (
	"strings"

	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// conditionResult holds the Gateway API resources produced from translating conditions annotations.
type conditionResult struct {
	// AdditionalHostnames to add to the HTTPRoute's route-level hostnames (from host-header condition values).
	AdditionalHostnames []gwv1.Hostname
	// Matches is the expanded set of HTTPRouteMatch entries after applying conditions.
	Matches []gwv1.HTTPRouteMatch
	// ListenerRuleConditions are conditions that must go into a ListenerRuleConfiguration
	ListenerRuleConditions []gatewayv1beta1.ListenerRuleCondition
}

// translateConditions translates parsed ingress RuleConditions into Gateway API constructs.
// baseMatches is the existing set of matches for the rule (typically one match from the Ingress path).
// The function returns expanded matches (cross-product for OR semantics), additional hostnames,
// and LRC conditions for things Gateway API can't represent natively.
func translateConditions(conditions []ingress.RuleCondition, matches []gwv1.HTTPRouteMatch) *conditionResult {
	if len(conditions) == 0 {
		return nil
	}

	result := &conditionResult{}

	for _, cond := range conditions {
		switch cond.Field {
		case ingress.RuleConditionFieldHostHeader:
			if cond.HostHeaderConfig == nil {
				continue
			}
			// Pass host-header values through to AdditionalHostnames.
			// These get added to a separate HTTPRoute's hostnames (split route).
			// Complex wildcards or regex values that Gateway API can't represent will be
			// rejected by the K8s API server when the manifest is applied.
			for _, v := range cond.HostHeaderConfig.Values {
				result.AdditionalHostnames = append(result.AdditionalHostnames, gwv1.Hostname(v))
			}
			for _, v := range cond.HostHeaderConfig.RegexValues {
				result.AdditionalHostnames = append(result.AdditionalHostnames, gwv1.Hostname(v))
			}

		case ingress.RuleConditionFieldPathPattern:
			if cond.PathPatternConfig == nil {
				continue
			}
			if len(cond.PathPatternConfig.Values) > 0 {
				matches = expandMatchesWithPaths(matches, cond.PathPatternConfig.Values, gwv1.PathMatchExact)
			}
			if len(cond.PathPatternConfig.RegexValues) > 0 {
				matches = expandMatchesWithPaths(matches, cond.PathPatternConfig.RegexValues, gwv1.PathMatchRegularExpression)
			}

		case ingress.RuleConditionFieldHTTPHeader:
			if cond.HTTPHeaderConfig == nil {
				continue
			}
			headerName := gwv1.HTTPHeaderName(cond.HTTPHeaderConfig.HTTPHeaderName)
			if len(cond.HTTPHeaderConfig.Values) > 0 {
				matches = addHeadersToMatches(matches, headerName, cond.HTTPHeaderConfig.Values, gwv1.HeaderMatchExact)
			}
			if len(cond.HTTPHeaderConfig.RegexValues) > 0 {
				matches = addHeadersToMatches(matches, headerName, cond.HTTPHeaderConfig.RegexValues, gwv1.HeaderMatchRegularExpression)
			}

		case ingress.RuleConditionFieldHTTPRequestMethod:
			if cond.HTTPRequestMethodConfig == nil {
				continue
			}
			matches = expandMatchesWithMethods(matches, cond.HTTPRequestMethodConfig.Values)

		case ingress.RuleConditionFieldQueryString:
			if cond.QueryStringConfig == nil {
				continue
			}
			// Values within a single query-string condition are OR'd in ALB.
			// Gateway API QueryParams within a single match are AND'd by the controller.
			// So we expand matches (cross-product) for OR semantics within one condition.
			matches = expandMatchesWithQueryParams(matches, cond.QueryStringConfig.Values)

		case ingress.RuleConditionFieldSourceIP:
			if cond.SourceIPConfig == nil {
				continue
			}
			result.ListenerRuleConditions = append(result.ListenerRuleConditions, gatewayv1beta1.ListenerRuleCondition{
				Field: gatewayv1beta1.ListenerRuleConditionFieldSourceIP,
				SourceIPConfig: &gatewayv1beta1.SourceIPConditionConfig{
					Values: cond.SourceIPConfig.Values,
				},
			})
		}
	}

	result.Matches = matches
	return result
}

// expandMatchesWithPaths adds new matches for additional path values (OR with existing paths).
// The original matches are preserved, and each new path value creates a copy of each existing match
// with the path replaced. This produces the OR semantics: match original path OR any condition path.
func expandMatchesWithPaths(matches []gwv1.HTTPRouteMatch, pathValues []string, pathType gwv1.PathMatchType) []gwv1.HTTPRouteMatch {
	var additional []gwv1.HTTPRouteMatch
	for _, m := range matches {
		for _, pv := range pathValues {
			newMatch := m.DeepCopy()
			v := pv
			newMatch.Path = &gwv1.HTTPPathMatch{
				Type:  &pathType,
				Value: &v,
			}
			additional = append(additional, *newMatch)
		}
	}
	return append(matches, additional...)
}

// addHeadersToMatches adds a header condition to each match.
// Multiple values for the same header are joined with commas — the gateway controller's
// generateValuesFromMatchHeaderValue splits them back into separate ALB values (OR semantics).
// This avoids unnecessary match expansion while preserving OR behavior.
func addHeadersToMatches(matches []gwv1.HTTPRouteMatch, name gwv1.HTTPHeaderName, values []string, matchType gwv1.HeaderMatchType) []gwv1.HTTPRouteMatch {
	joinedValue := strings.Join(values, ",")
	for i := range matches {
		matches[i].Headers = append(matches[i].Headers, gwv1.HTTPHeaderMatch{
			Type:  &matchType,
			Name:  name,
			Value: joinedValue,
		})
	}
	return matches
}

// expandMatchesWithQueryParams creates the cross-product of existing matches with query param values (OR).
// Each key-value pair in the values slice produces a separate match because ALB OR's values within
// a single query-string condition, but the gateway controller AND's multiple QueryParams in a match.
func expandMatchesWithQueryParams(matches []gwv1.HTTPRouteMatch, values []ingress.QueryStringKeyValuePair) []gwv1.HTTPRouteMatch {
	var expanded []gwv1.HTTPRouteMatch
	for _, m := range matches {
		for _, qsKV := range values {
			newMatch := m.DeepCopy()
			exact := gwv1.QueryParamMatchExact
			newMatch.QueryParams = append(newMatch.QueryParams, gwv1.HTTPQueryParamMatch{
				Type:  &exact,
				Name:  gwv1.HTTPHeaderName(stringOrEmpty(qsKV.Key)),
				Value: qsKV.Value,
			})
			expanded = append(expanded, *newMatch)
		}
	}
	return expanded
}

// expandMatchesWithMethods creates the cross-product of existing matches with method values (OR).
func expandMatchesWithMethods(matches []gwv1.HTTPRouteMatch, methods []string) []gwv1.HTTPRouteMatch {
	var expanded []gwv1.HTTPRouteMatch
	for _, m := range matches {
		for _, method := range methods {
			newMatch := m.DeepCopy()
			httpMethod := gwv1.HTTPMethod(method)
			newMatch.Method = &httpMethod
			expanded = append(expanded, *newMatch)
		}
	}
	return expanded
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
