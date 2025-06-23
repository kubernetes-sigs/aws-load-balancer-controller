package routeutils

import (
	v1 "sigs.k8s.io/gateway-api/apis/v1"
	"sort"
	"strings"
	"time"
)

type RulePrecedence struct {
	RouteDescriptor  RouteDescriptor
	Rule             RouteRule
	RuleIndexInRoute int // index of the rule in the route
	MatchIndexInRule int // index of the match in the rule

	HTTPMatch *v1.HTTPRouteMatch
	GRPCMatch *v1.GRPCRouteMatch

	// factors determining precedence
	Hostnames            []string // raw hostnames from route, unsorted
	PathType             int      // 3=exact, 2=regex, 1=prefix
	PathLength           int
	HasMethod            bool
	HeaderCount          int
	QueryParamCount      int
	RouteCreateTimestamp time.Time
	RouteNamespacedName  string
}

func SortAllRulesByPrecedence(routes []RouteDescriptor) []RulePrecedence {
	var allRoutes []RulePrecedence

	for _, route := range routes {
		routeNamespacedName := route.GetRouteNamespacedName().String()
		routeCreateTimestamp := route.GetRouteCreateTimestamp()
		// get hostname in string array format
		hostnames := make([]string, len(route.GetHostnames()))
		for i, hostname := range route.GetHostnames() {
			hostnames[i] = string(hostname)
		}
		for ruleIndex, rule := range route.GetAttachedRules() {
			rawRule := rule.GetRawRouteRule()
			switch r := rawRule.(type) {
			// TODO: add handling for GRPC
			case *v1.HTTPRouteRule:
				for matchIndex, httpMatch := range r.Matches {
					match := RulePrecedence{
						RouteDescriptor:      route,
						Rule:                 rule,
						RuleIndexInRoute:     ruleIndex,
						MatchIndexInRule:     matchIndex,
						HTTPMatch:            &httpMatch,
						Hostnames:            hostnames,
						RouteCreateTimestamp: routeCreateTimestamp,
						RouteNamespacedName:  routeNamespacedName,
					}
					getHttpMatchPrecedenceInfo(&httpMatch, &match)
					allRoutes = append(allRoutes, match)
				}
			}

		}
	}
	// sort rules based on precedence
	sort.Slice(allRoutes, func(i, j int) bool {
		return comparePrecedence(allRoutes[i], allRoutes[j])
	})
	return allRoutes
}

// getHostnamePrecedenceOrder Hostname precedence ordering rule:
// 1. non-wildcard has higher precedence than wildcard
// 2. hostname with longer characters have higher precedence than those with shorter ones
// -1 means hostnameOne has higher precedence, 1 means hostnameTwo has higher precedence, 0 means equal
func getHostnamePrecedenceOrder(hostnameOne, hostnameTwo string) int {
	isHostnameOneWildcard := strings.HasPrefix(hostnameOne, "*.")
	isHostnameTwoWildcard := strings.HasPrefix(hostnameTwo, "*.")

	if !isHostnameOneWildcard && isHostnameTwoWildcard {
		return -1
	} else if isHostnameOneWildcard && !isHostnameTwoWildcard {
		return 1
	} else {
		dotsInHostnameOne := strings.Count(hostnameOne, ".")
		dotsInHostnameTwo := strings.Count(hostnameTwo, ".")
		if dotsInHostnameOne > dotsInHostnameTwo {
			return -1
		} else if dotsInHostnameOne < dotsInHostnameTwo {
			return 1
		}
		if len(hostnameOne) > len(hostnameTwo) {
			return -1
		} else if len(hostnameOne) < len(hostnameTwo) {
			return 1
		} else {
			return 0
		}
	}
}

// sortHostnameListByPrecedence Given a hostname list, sort it by precedence order
func sortHostnameListByPrecedence(hostnames []string) {
	// sort hostnames based on their precedence
	sort.Slice(hostnames, func(i, j int) bool {
		return getHostnamePrecedenceOrder(hostnames[i], hostnames[j]) < 0
	})
}

// getHostnameListPrecedenceOrder this function tries to tiebreak two routes based on hostname precedence
// When length of two hostname lists is not same and precedence order is not determined until the end, 0 will be return and tiebreak will continue on other attributes
func getHostnameListPrecedenceOrder(hostnameListOne, hostnameListTwo []string) int {
	// sort each hostname list by precedence
	sortHostnameListByPrecedence(hostnameListOne)
	sortHostnameListByPrecedence(hostnameListTwo)
	// compare each hostname list in order
	length := min(len(hostnameListOne), len(hostnameListTwo))
	for i := range length {
		precedence := getHostnamePrecedenceOrder(hostnameListOne[i], hostnameListTwo[i])
		if precedence != 0 {
			return precedence
		}
	}
	// can not complete tie breaking at hostname level
	return 0
}

func comparePrecedence(ruleOne RulePrecedence, ruleTwo RulePrecedence) bool {
	precedence := getHostnameListPrecedenceOrder(ruleOne.Hostnames, ruleTwo.Hostnames)
	if precedence != 0 {
		return precedence < 0 // -1 means first hostname has higher precedence
	}
	// equal hostname precedence, sort by other factors
	// compare path match type (exact  > regex > prefix)
	if ruleOne.PathType != ruleTwo.PathType {
		return ruleOne.PathType > ruleTwo.PathType
	}
	// compare path length
	if ruleOne.PathLength != ruleTwo.PathLength {
		return ruleOne.PathLength > ruleTwo.PathLength
	}
	// compare has method
	if ruleOne.HasMethod != ruleTwo.HasMethod {
		return ruleOne.HasMethod
	}
	// compare header count
	if ruleOne.HeaderCount != ruleTwo.HeaderCount {
		return ruleOne.HeaderCount > ruleTwo.HeaderCount
	}
	// compare query param count
	if ruleOne.QueryParamCount != ruleTwo.QueryParamCount {
		return ruleOne.QueryParamCount > ruleTwo.QueryParamCount
	}
	// compare creation timestamp
	if !ruleOne.RouteCreateTimestamp.Equal(ruleTwo.RouteCreateTimestamp) {
		return ruleOne.RouteCreateTimestamp.Before(ruleTwo.RouteCreateTimestamp)
	}
	// compare namespaced name (namespace/name) in alphabetic order
	if ruleOne.RouteNamespacedName != ruleTwo.RouteNamespacedName {
		return ruleOne.RouteNamespacedName < ruleTwo.RouteNamespacedName
	}
	// compare rule index in route
	if ruleOne.RuleIndexInRoute != ruleTwo.RuleIndexInRoute {
		return ruleOne.RuleIndexInRoute < ruleTwo.RuleIndexInRoute
	}
	// compare match index within rule
	return ruleOne.MatchIndexInRule < ruleTwo.MatchIndexInRule
}

func getHttpMatchPrecedenceInfo(httpMatch *v1.HTTPRouteMatch, matchPrecedence *RulePrecedence) {

	matchPrecedence.PathType, matchPrecedence.PathLength = getHttpRoutePathTypeAndLength(httpMatch.Path)
	matchPrecedence.HasMethod = httpMatch.Method != nil
	matchPrecedence.HeaderCount = len(httpMatch.Headers)
	matchPrecedence.QueryParamCount = len(httpMatch.QueryParams)

}

func getHttpRoutePathTypeAndLength(path *v1.HTTPPathMatch) (int, int) {
	switch *path.Type {
	case v1.PathMatchExact:
		return 3, len(*path.Value)
	case v1.PathMatchRegularExpression:
		return 2, len(*path.Value)
	default:
		return 1, len(*path.Value)
	}
}
