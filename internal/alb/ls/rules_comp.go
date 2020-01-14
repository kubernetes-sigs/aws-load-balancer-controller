package ls

import (
	"reflect"
	"sort"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/conditions"
)

// ruleMatches checks whether current rule matches desired rule.
func ruleMatches(desired elbv2.Rule, current elbv2.Rule) bool {
	return actionsMatches(desired.Actions, current.Actions) &&
		conditionsMatches(desired.Conditions, current.Conditions)
}

// actionsMatches checks whether current actions matches desired actions
// the actions is compared based on their order.
func actionsMatches(desired []*elbv2.Action, current []*elbv2.Action) bool {
	sortedDesired := sortedActions(desired)
	sortedCurrent := sortedActions(current)
	if len(sortedDesired) != len(sortedCurrent) {
		return false
	}
	for i := range sortedDesired {
		if !actionMatches(sortedDesired[i], sortedCurrent[i]) {
			return false
		}
	}
	return true
}

// conditionsMatches checks whether current conditions matches desired conditions
func conditionsMatches(desired []*elbv2.RuleCondition, current []*elbv2.RuleCondition) bool {
	return sliceMatches(desired, current, func(i interface{}, j interface{}) bool {
		return conditionMatches(i.(*elbv2.RuleCondition), j.(*elbv2.RuleCondition))
	})
}

// sortedActions returns sorted actions based on action order.
func sortedActions(actions []*elbv2.Action) []*elbv2.Action {
	actionsClone := make([]*elbv2.Action, len(actions))
	copy(actionsClone, actions)
	sort.Slice(actionsClone, func(i, j int) bool {
		return aws.Int64Value(actionsClone[i].Order) < aws.Int64Value(actionsClone[j].Order)
	})
	return actionsClone
}

// actionMatches checks whether current action matches desired action
func actionMatches(desired *elbv2.Action, current *elbv2.Action) bool {
	if aws.StringValue(desired.Type) != aws.StringValue(current.Type) {
		return false
	}
	switch aws.StringValue(desired.Type) {
	case elbv2.ActionTypeEnumAuthenticateOidc:
		return reflect.DeepEqual(desired.AuthenticateOidcConfig, current.AuthenticateOidcConfig)
	case elbv2.ActionTypeEnumAuthenticateCognito:
		return reflect.DeepEqual(desired.AuthenticateCognitoConfig, current.AuthenticateCognitoConfig)
	case elbv2.ActionTypeEnumRedirect:
		return reflect.DeepEqual(desired.RedirectConfig, current.RedirectConfig)
	case elbv2.ActionTypeEnumFixedResponse:
		return reflect.DeepEqual(desired.FixedResponseConfig, current.FixedResponseConfig)
	case elbv2.ActionTypeEnumForward:
		return reflect.DeepEqual(desired.ForwardConfig, current.ForwardConfig)
	}
	return false
}

func conditionMatches(desired *elbv2.RuleCondition, current *elbv2.RuleCondition) bool {
	if aws.StringValue(desired.Field) != aws.StringValue(current.Field) {
		return false
	}
	switch aws.StringValue(desired.Field) {
	case conditions.FieldHostHeader:
		return hostHeaderConditionConfigMatches(desired.HostHeaderConfig, current.HostHeaderConfig)
	case conditions.FieldPathPattern:
		return pathPatternConditionConfigMatches(desired.PathPatternConfig, current.PathPatternConfig)
	case conditions.FieldHTTPHeader:
		return httpHeaderConditionConfigMatches(desired.HttpHeaderConfig, current.HttpHeaderConfig)
	case conditions.FieldHTTPRequestMethod:
		return httpRequestMethodConditionConfigMatches(desired.HttpRequestMethodConfig, current.HttpRequestMethodConfig)
	case conditions.FieldQueryString:
		return queryStringConditionConfigMatches(desired.QueryStringConfig, current.QueryStringConfig)
	case conditions.FieldSourceIP:
		return sourceIpConditionConfigMatches(desired.SourceIpConfig, current.SourceIpConfig)
	}
	return false
}

func hostHeaderConditionConfigMatches(desired *elbv2.HostHeaderConditionConfig, current *elbv2.HostHeaderConditionConfig) bool {
	if desired == nil || current == nil {
		return desired == current
	}
	return sliceMatches(desired.Values, current.Values, reflect.DeepEqual)
}

func pathPatternConditionConfigMatches(desired *elbv2.PathPatternConditionConfig, current *elbv2.PathPatternConditionConfig) bool {
	if desired == nil || current == nil {
		return desired == current
	}
	return sliceMatches(desired.Values, current.Values, reflect.DeepEqual)
}

func httpHeaderConditionConfigMatches(desired *elbv2.HttpHeaderConditionConfig, current *elbv2.HttpHeaderConditionConfig) bool {
	if desired == nil || current == nil {
		return desired == current
	}
	return (aws.StringValue(desired.HttpHeaderName) == aws.StringValue(current.HttpHeaderName)) &&
		sliceMatches(desired.Values, current.Values, reflect.DeepEqual)
}

func httpRequestMethodConditionConfigMatches(desired *elbv2.HttpRequestMethodConditionConfig, current *elbv2.HttpRequestMethodConditionConfig) bool {
	if desired == nil || current == nil {
		return desired == current
	}
	return sliceMatches(desired.Values, current.Values, reflect.DeepEqual)
}

func queryStringConditionConfigMatches(desired *elbv2.QueryStringConditionConfig, current *elbv2.QueryStringConditionConfig) bool {
	if desired == nil || current == nil {
		return desired == current
	}
	return sliceMatches(desired.Values, current.Values, reflect.DeepEqual)
}

func sourceIpConditionConfigMatches(desired *elbv2.SourceIpConditionConfig, current *elbv2.SourceIpConditionConfig) bool {
	if desired == nil || current == nil {
		return desired == current
	}
	return sliceMatches(desired.Values, current.Values, reflect.DeepEqual)
}

// stringSliceMatches checks whether current slice matches desired slice, without comparing order.
func stringSliceMatches(desired []*string, current []*string) bool {
	if len(desired) != len(current) {
		return false
	}
	visited := make([]bool, len(current))
	for _, elemI := range desired {
		found := false
		for j, elemJ := range current {
			if visited[j] {
				continue
			}
			if aws.StringValue(elemI) == aws.StringValue(elemJ) {
				visited[j] = true
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func sliceMatches(listA interface{}, listB interface{}, comparator func(interface{}, interface{}) bool) bool {
	valueA := reflect.ValueOf(listA)
	valueB := reflect.ValueOf(listB)
	lenA := valueA.Len()
	lenB := valueB.Len()
	if lenA != lenB {
		return false
	}
	visited := make([]bool, lenB)
	for i := 0; i < lenA; i++ {
		elem := valueA.Index(i).Interface()
		found := false
		for j := 0; j < lenB; j++ {
			if visited[j] {
				continue
			}
			if comparator(elem, valueB.Index(j).Interface()) {
				visited[j] = true
				found = true
			}
		}
		if !found {
			return false
		}
	}

	return true
}
