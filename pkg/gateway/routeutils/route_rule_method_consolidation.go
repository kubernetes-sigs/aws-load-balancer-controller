package routeutils

import (
	"fmt"
	"sort"
	"strings"

	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// maxConditionValuesPerRule is the ALB quota "Condition Values per Rule" (5, not
// adjustable). An http-request-method condition accepts a value list, so matches
// that differ only by method can share one rule — but the merged value list still
// has to fit in the rule's value budget together with the values the other
// conditions of the same rule consume.
const maxConditionValuesPerRule = 5

// consolidateHttpMethodMatches merges HTTP matches of the same route rule that
// are identical in every respect except their method into a single entry whose
// http-request-method condition carries every method. The Gateway API
// HTTPRouteMatch.Method field holds a single method, so serving N methods on one
// path requires N matches, which would otherwise become N ALB rules and eat into
// the (default 100) rules per load balancer quota.
//
// Only matches that set a method participate: a match without a method means
// "any method", so folding it into a method-bearing group would narrow it.
//
// Merged entries keep the lowest MatchIndexInRule of their group. Every member
// has the same path type/length, header count, query param count and
// HasMethod=true, so the group's precedence factors equal any member's and the
// merged entry sorts exactly where its members did.
//
// Groups whose methods do not fit the rule's remaining value budget are split
// into chunks of the largest allowed size; a budget of one value yields the
// unmerged entries, i.e. today's behavior.
func consolidateHttpMethodMatches(entries []RulePrecedence) []RulePrecedence {
	groupMembers := make(map[string][]RulePrecedence, len(entries))
	groupOrder := make([]string, 0, len(entries))

	for _, entry := range entries {
		key, mergeable := httpMethodMergeKey(entry)
		if !mergeable {
			continue
		}
		if _, exists := groupMembers[key]; !exists {
			groupOrder = append(groupOrder, key)
		}
		groupMembers[key] = append(groupMembers[key], entry)
	}

	// Nothing to do unless at least one group has more than one member.
	merged := false
	for _, key := range groupOrder {
		if len(groupMembers[key]) > 1 {
			merged = true
			break
		}
	}
	if !merged {
		return entries
	}

	mergedByKey := make(map[string][]RulePrecedence, len(groupOrder))
	for _, key := range groupOrder {
		mergedByKey[key] = mergeHttpMethodGroup(groupMembers[key])
	}

	consolidated := make([]RulePrecedence, 0, len(entries))
	for _, entry := range entries {
		key, mergeable := httpMethodMergeKey(entry)
		if !mergeable {
			consolidated = append(consolidated, entry)
			continue
		}
		// Emit the whole group in place of its first member and drop the rest.
		if group, pending := mergedByKey[key]; pending {
			consolidated = append(consolidated, group...)
			delete(mergedByKey, key)
		}
	}
	return consolidated
}

// mergeHttpMethodGroup turns a group of matches that differ only by method into
// as few entries as the rule's condition value budget allows.
func mergeHttpMethodGroup(group []RulePrecedence) []RulePrecedence {
	if len(group) == 1 {
		return group
	}

	members := make([]RulePrecedence, len(group))
	copy(members, group)
	sort.SliceStable(members, func(i, j int) bool {
		return members[i].CommonRulePrecedence.MatchIndexInRule < members[j].CommonRulePrecedence.MatchIndexInRule
	})

	methods := make([]string, 0, len(members))
	seen := make(map[string]bool, len(members))
	for _, member := range members {
		method := string(*member.HTTPMatch.Method)
		if seen[method] {
			continue
		}
		seen[method] = true
		methods = append(methods, method)
	}

	// The methods share one condition, so they get whatever is left of the
	// rule's value budget after the other conditions of the rule took theirs.
	chunkSize := maxConditionValuesPerRule - countNonMethodConditionValues(members[0])
	if chunkSize < 2 {
		// No room to merge: keep the individual matches untouched.
		return group
	}

	entries := make([]RulePrecedence, 0, (len(methods)+chunkSize-1)/chunkSize)
	for start := 0; start < len(methods); start += chunkSize {
		end := start + chunkSize
		if end > len(methods) {
			end = len(methods)
		}
		// The lowest match index of the chunk keeps ordering deterministic and
		// keeps the chunks in the order their matches were declared.
		entry := members[start]
		entry.HTTPMethods = methods[start:end]
		entries = append(entries, entry)
	}
	return entries
}

// httpMethodMergeKey identifies matches that may be merged with each other: same
// route rule, same hostname unit and identical path, header and query param
// matches, with only the method left to differ. Matches without a method are not
// mergeable ("any method" would be narrowed by the merge).
func httpMethodMergeKey(entry RulePrecedence) (string, bool) {
	match := entry.HTTPMatch
	if match == nil || match.Method == nil {
		return "", false
	}
	common := entry.CommonRulePrecedence
	var key strings.Builder
	fmt.Fprintf(&key, "route=%s#rule=%d#host=%q", common.RouteNamespacedName, common.RuleIndexInRoute, common.Hostname)
	key.WriteString("#path=")
	if match.Path != nil {
		if match.Path.Type != nil {
			key.WriteString(string(*match.Path.Type))
		}
		if match.Path.Value != nil {
			fmt.Fprintf(&key, ":%q", *match.Path.Value)
		}
	}
	// Header and query param matches are keyed in their declared order: it is
	// the order of the generated conditions, so members of a group produce the
	// very same condition list.
	for _, header := range match.Headers {
		headerType := gwv1.HeaderMatchExact
		if header.Type != nil {
			headerType = *header.Type
		}
		fmt.Fprintf(&key, "#header=%q:%s:%q", header.Name, headerType, header.Value)
	}
	for _, query := range match.QueryParams {
		queryType := gwv1.QueryParamMatchExact
		if query.Type != nil {
			queryType = *query.Type
		}
		fmt.Fprintf(&key, "#query=%q:%s:%q", query.Name, queryType, query.Value)
	}
	// Conditions coming from the ListenerRuleConfiguration CRD may be scoped to
	// specific match indexes, so matches that get different ones must not merge.
	key.WriteString("#sourceIP=")
	key.WriteString(sourceIpConditionKey(entry))
	return key.String(), true
}

// sourceIpConditionKey serializes the source IP conditions the
// ListenerRuleConfiguration CRD contributes to this particular match.
func sourceIpConditionKey(entry RulePrecedence) string {
	rule := entry.CommonRulePrecedence.Rule
	if rule == nil || rule.GetListenerRuleConfig() == nil {
		return ""
	}
	var key strings.Builder
	for _, condition := range applicableSourceIpConditions(entry) {
		fmt.Fprintf(&key, "[%s]", strings.Join(condition.SourceIPConfig.Values, ","))
	}
	return key.String()
}

// applicableSourceIpConditions returns the source IP conditions of the rule's
// ListenerRuleConfiguration that apply to this match.
func applicableSourceIpConditions(entry RulePrecedence) []elbv2gw.ListenerRuleCondition {
	rule := entry.CommonRulePrecedence.Rule
	if rule == nil || rule.GetListenerRuleConfig() == nil {
		return nil
	}
	matchIndex := entry.CommonRulePrecedence.MatchIndexInRule
	var conditions []elbv2gw.ListenerRuleCondition
	for _, condition := range rule.GetListenerRuleConfig().Spec.Conditions {
		if condition.Field != elbv2gw.ListenerRuleConditionFieldSourceIP || condition.SourceIPConfig == nil {
			continue
		}
		if condition.MatchIndexes == nil {
			conditions = append(conditions, condition)
			continue
		}
		for _, index := range *condition.MatchIndexes {
			if index == matchIndex {
				conditions = append(conditions, condition)
				break
			}
		}
	}
	return conditions
}

// countNonMethodConditionValues counts the condition values the entry consumes
// besides the http-request-method ones, mirroring what the condition builders
// emit, so a merged method list can be sized against the ALB per rule value
// quota.
func countNonMethodConditionValues(entry RulePrecedence) int {
	count := 0
	if entry.CommonRulePrecedence.Hostname != "" {
		count++
	}
	match := entry.HTTPMatch
	if match.Path != nil && match.Path.Type != nil {
		switch *match.Path.Type {
		case gwv1.PathMatchPathPrefix:
			// "/" becomes the single pattern "/*", any other prefix becomes
			// both the exact path and its subtree pattern.
			if match.Path.Value != nil && *match.Path.Value == "/" {
				count++
			} else {
				count += 2
			}
		default:
			count++
		}
	}
	for _, header := range match.Headers {
		if header.Type != nil && *header.Type == gwv1.HeaderMatchRegularExpression {
			count++
			continue
		}
		count += len(generateValuesFromMatchHeaderValue(header.Value))
	}
	count += len(match.QueryParams)
	for _, condition := range applicableSourceIpConditions(entry) {
		count += len(condition.SourceIPConfig.Values)
	}
	return count
}
