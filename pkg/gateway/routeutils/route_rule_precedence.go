package routeutils

import (
	"math"
	"sort"
	"strings"
	"time"

	v1 "sigs.k8s.io/gateway-api/apis/v1"
)

type RulePrecedence struct {
	CommonRulePrecedence CommonRulePrecedence
	HTTPMatch            *v1.HTTPRouteMatch
	GRPCMatch            *v1.GRPCRouteMatch

	// factors determining precedence
	HttpSpecificRulePrecedenceFactor *HttpSpecificRulePrecedenceFactor
	GrpcSpecificRulePrecedenceFactor *GrpcSpecificRulePrecedenceFactor
}

type GrpcSpecificRulePrecedenceFactor struct {
	PathType      int // 3=exact, 1=regex
	ServiceLength int // grpcRouteMatch Method - service characters number
	MethodLength  int // grpcRouteMatch Method - method characters number
	HeaderCount   int // headers count
}

type HttpSpecificRulePrecedenceFactor struct {
	PathType        int  // 3=exact, 2=prefix, 1=regex
	PathLength      int  // httpRouteMatch path length
	HasMethod       bool // httpRouteMatch Method
	HeaderCount     int  // httpRouteMatch headers count
	QueryParamCount int  // httpRouteMatch query params count
}

type CommonRulePrecedence struct {
	RouteDescriptor RouteDescriptor
	Rule            RouteRule

	// common rule precedence factors
	Hostnames            []string // raw hostnames from route, unsorted
	RouteNamespacedName  string
	RuleIndexInRoute     int // index of the rule in the route
	MatchIndexInRule     int // index of the match in the rule
	RouteCreateTimestamp time.Time
}

func SortAllRulesByPrecedence(routes []RouteDescriptor, port int32) []RulePrecedence {
	var allRoutes []RulePrecedence
	var httpRoutes []RulePrecedence
	var grpcRoutes []RulePrecedence

	for _, route := range routes {
		routeInfo := getCommonRouteInfo(route, port)
		for ruleIndex, rule := range route.GetAttachedRules() {
			rawRule := rule.GetRawRouteRule()
			switch r := rawRule.(type) {
			case *v1.HTTPRouteRule:
				for matchIndex, httpMatch := range r.Matches {
					common := routeInfo
					common.Rule = rule
					common.RuleIndexInRoute = ruleIndex
					common.MatchIndexInRule = matchIndex
					match := RulePrecedence{
						HTTPMatch:                        &httpMatch,
						HttpSpecificRulePrecedenceFactor: &HttpSpecificRulePrecedenceFactor{},
						CommonRulePrecedence:             common,
					}
					// set HttpSpecificRulePrecedenceFactor
					getHttpMatchPrecedenceInfo(&httpMatch, &match)
					httpRoutes = append(httpRoutes, match)
				}
				if len(r.Matches) == 0 {
					common := routeInfo
					common.Rule = rule
					common.RuleIndexInRoute = ruleIndex
					common.MatchIndexInRule = math.MaxInt
					match := RulePrecedence{
						HTTPMatch:                        &v1.HTTPRouteMatch{},
						HttpSpecificRulePrecedenceFactor: &HttpSpecificRulePrecedenceFactor{},
						CommonRulePrecedence:             common,
					}
					httpRoutes = append(httpRoutes, match)
				}
			case *v1.GRPCRouteRule:
				for matchIndex, grpcMatch := range r.Matches {
					common := routeInfo
					common.Rule = rule
					common.RuleIndexInRoute = ruleIndex
					common.MatchIndexInRule = matchIndex
					match := RulePrecedence{
						GRPCMatch:                        &grpcMatch,
						GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{},
						CommonRulePrecedence:             common,
					}
					// set GrpcSpecificRulePrecedenceFactor
					getGrpcMatchPrecedenceInfo(&grpcMatch, &match)
					grpcRoutes = append(grpcRoutes, match)
				}

				if len(r.Matches) == 0 {
					common := routeInfo
					common.Rule = rule
					common.RuleIndexInRoute = ruleIndex
					common.MatchIndexInRule = math.MaxInt
					match := RulePrecedence{
						GRPCMatch:                        &v1.GRPCRouteMatch{},
						GrpcSpecificRulePrecedenceFactor: &GrpcSpecificRulePrecedenceFactor{},
						CommonRulePrecedence:             common,
					}
					grpcRoutes = append(grpcRoutes, match)
				}
			}
		}
	}

	allRoutes = append(allRoutes, httpRoutes...)
	allRoutes = append(allRoutes, grpcRoutes...)

	// Sort all rules using a unified comparator that handles cross-kind
	// precedence based on specificity rather than route kind.
	sort.Slice(allRoutes, func(i, j int) bool {
		return compareRulePrecedenceUnified(allRoutes[i], allRoutes[j])
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
	// All compared hostnames tie up to the shorter list's length. A shorter list
	// (in the extreme, an empty list as used by a catch-all route) constrains
	// fewer hostnames and is therefore less specific, so it must have lower
	// precedence. Without this, an empty hostname list returns 0 ("equal") and
	// precedence falls through to path length / creation timestamp, mixing
	// incompatible criteria and making the comparator non-transitive.
	if len(hostnameListOne) != len(hostnameListTwo) {
		if len(hostnameListOne) > len(hostnameListTwo) {
			return -1 // one is more specific (more hostnames)
		}
		return 1 // two is more specific
	}
	// genuinely equal at hostname level
	return 0
}

func compareHttpRulePrecedence(ruleOne RulePrecedence, ruleTwo RulePrecedence) bool {
	precedence := getHostnameListPrecedenceOrder(ruleOne.CommonRulePrecedence.Hostnames, ruleTwo.CommonRulePrecedence.Hostnames)
	if precedence != 0 {
		return precedence < 0 // -1 means first hostname has higher precedence
	}
	// equal hostname precedence, sort by other factors
	// compare path match type (exact  > prefix > regex)
	if ruleOne.HttpSpecificRulePrecedenceFactor.PathType != ruleTwo.HttpSpecificRulePrecedenceFactor.PathType {
		return ruleOne.HttpSpecificRulePrecedenceFactor.PathType > ruleTwo.HttpSpecificRulePrecedenceFactor.PathType
	}
	// compare path length
	if ruleOne.HttpSpecificRulePrecedenceFactor.PathLength != ruleTwo.HttpSpecificRulePrecedenceFactor.PathLength {
		return ruleOne.HttpSpecificRulePrecedenceFactor.PathLength > ruleTwo.HttpSpecificRulePrecedenceFactor.PathLength
	}
	// compare has method
	if ruleOne.HttpSpecificRulePrecedenceFactor.HasMethod != ruleTwo.HttpSpecificRulePrecedenceFactor.HasMethod {
		return ruleOne.HttpSpecificRulePrecedenceFactor.HasMethod
	}
	// compare header count
	if ruleOne.HttpSpecificRulePrecedenceFactor.HeaderCount != ruleTwo.HttpSpecificRulePrecedenceFactor.HeaderCount {
		return ruleOne.HttpSpecificRulePrecedenceFactor.HeaderCount > ruleTwo.HttpSpecificRulePrecedenceFactor.HeaderCount
	}
	// compare query param count
	if ruleOne.HttpSpecificRulePrecedenceFactor.QueryParamCount != ruleTwo.HttpSpecificRulePrecedenceFactor.QueryParamCount {
		return ruleOne.HttpSpecificRulePrecedenceFactor.QueryParamCount > ruleTwo.HttpSpecificRulePrecedenceFactor.QueryParamCount
	}
	return compareCommonTieBreakers(ruleOne, ruleTwo)
}

func compareGrpcRulePrecedence(ruleOne RulePrecedence, ruleTwo RulePrecedence) bool {
	precedence := getHostnameListPrecedenceOrder(ruleOne.CommonRulePrecedence.Hostnames, ruleTwo.CommonRulePrecedence.Hostnames)
	if precedence != 0 {
		return precedence < 0 // -1 means first hostname has higher precedence
	}
	// equal hostname precedence, sort by other factors
	// compare path match type (exact  > regex)
	if ruleOne.GrpcSpecificRulePrecedenceFactor.PathType != ruleTwo.GrpcSpecificRulePrecedenceFactor.PathType {
		return ruleOne.GrpcSpecificRulePrecedenceFactor.PathType > ruleTwo.GrpcSpecificRulePrecedenceFactor.PathType
	}
	// compare service length
	if ruleOne.GrpcSpecificRulePrecedenceFactor.ServiceLength != ruleTwo.GrpcSpecificRulePrecedenceFactor.ServiceLength {
		return ruleOne.GrpcSpecificRulePrecedenceFactor.ServiceLength > ruleTwo.GrpcSpecificRulePrecedenceFactor.ServiceLength
	}
	// compare method length
	if ruleOne.GrpcSpecificRulePrecedenceFactor.MethodLength != ruleTwo.GrpcSpecificRulePrecedenceFactor.MethodLength {
		return ruleOne.GrpcSpecificRulePrecedenceFactor.MethodLength > ruleTwo.GrpcSpecificRulePrecedenceFactor.MethodLength
	}
	// compare header count
	if ruleOne.GrpcSpecificRulePrecedenceFactor.HeaderCount != ruleTwo.GrpcSpecificRulePrecedenceFactor.HeaderCount {
		return ruleOne.GrpcSpecificRulePrecedenceFactor.HeaderCount > ruleTwo.GrpcSpecificRulePrecedenceFactor.HeaderCount
	}
	return compareCommonTieBreakers(ruleOne, ruleTwo)
}

func compareCommonTieBreakers(ruleOne RulePrecedence, ruleTwo RulePrecedence) bool {
	// compare creation timestamp
	if !ruleOne.CommonRulePrecedence.RouteCreateTimestamp.Equal(ruleTwo.CommonRulePrecedence.RouteCreateTimestamp) {
		return ruleOne.CommonRulePrecedence.RouteCreateTimestamp.Before(ruleTwo.CommonRulePrecedence.RouteCreateTimestamp)
	}
	// compare namespaced name (namespace/name) in alphabetic order
	if ruleOne.CommonRulePrecedence.RouteNamespacedName != ruleTwo.CommonRulePrecedence.RouteNamespacedName {
		return ruleOne.CommonRulePrecedence.RouteNamespacedName < ruleTwo.CommonRulePrecedence.RouteNamespacedName
	}
	// compare rule index in route
	if ruleOne.CommonRulePrecedence.RuleIndexInRoute != ruleTwo.CommonRulePrecedence.RuleIndexInRoute {
		return ruleOne.CommonRulePrecedence.RuleIndexInRoute < ruleTwo.CommonRulePrecedence.RuleIndexInRoute
	}
	// compare match index within rule
	return ruleOne.CommonRulePrecedence.MatchIndexInRule < ruleTwo.CommonRulePrecedence.MatchIndexInRule
}

func getCommonRouteInfo(route RouteDescriptor, port int32) CommonRulePrecedence {
	routeNamespacedName := route.GetRouteNamespacedName().String()
	routeCreateTimestamp := route.GetRouteCreateTimestamp()
	// Use compatible hostnames computed during route attachment
	compatibleHostnamesByPort := route.GetCompatibleHostnamesByPort()[port]
	hostnames := make([]string, 0)
	for _, h := range compatibleHostnamesByPort {
		hostnames = append(hostnames, string(h))
	}
	// If no compatible hostnames, use route hostnames
	if len(hostnames) == 0 {
		for _, h := range route.GetHostnames() {
			hostnames = append(hostnames, string(h))
		}
	}
	return CommonRulePrecedence{
		RouteDescriptor:      route,
		Hostnames:            hostnames,
		RouteCreateTimestamp: routeCreateTimestamp,
		RouteNamespacedName:  routeNamespacedName,
	}
}

func getHttpMatchPrecedenceInfo(httpMatch *v1.HTTPRouteMatch, matchPrecedence *RulePrecedence) {
	matchPrecedence.HttpSpecificRulePrecedenceFactor.PathType = getHttpRoutePathType(httpMatch.Path)
	// httpMatch.Path.Value won't be nil, default is /
	matchPrecedence.HttpSpecificRulePrecedenceFactor.PathLength = len(*httpMatch.Path.Value)
	matchPrecedence.HttpSpecificRulePrecedenceFactor.HasMethod = httpMatch.Method != nil
	matchPrecedence.HttpSpecificRulePrecedenceFactor.HeaderCount = len(httpMatch.Headers)
	matchPrecedence.HttpSpecificRulePrecedenceFactor.QueryParamCount = len(httpMatch.QueryParams)

}

// getHttpRoutePathType returns path type
// the higher priority path type has higher value
// Exact = 3, Prefix = 2, RegularExpression = 1
func getHttpRoutePathType(path *v1.HTTPPathMatch) int {
	if path == nil {
		return 0
	}
	switch *path.Type {
	case v1.PathMatchExact:
		return 3
	case v1.PathMatchPathPrefix:
		return 2
	case v1.PathMatchRegularExpression:
		return 1
	default:
		return 0
	}
}

// getGrpcMatchPrecedenceInfo
func getGrpcMatchPrecedenceInfo(grpcMatch *v1.GRPCRouteMatch, matchPrecedence *RulePrecedence) {
	matchPrecedence.GrpcSpecificRulePrecedenceFactor.PathType = getGrpcRoutePathType(grpcMatch.Method)
	matchPrecedence.GrpcSpecificRulePrecedenceFactor.HeaderCount = len(grpcMatch.Headers)
	if grpcMatch.Method != nil {
		if grpcMatch.Method.Service != nil {
			matchPrecedence.GrpcSpecificRulePrecedenceFactor.ServiceLength = len(*grpcMatch.Method.Service)
		}
		if grpcMatch.Method.Method != nil {
			matchPrecedence.GrpcSpecificRulePrecedenceFactor.MethodLength = len(*grpcMatch.Method.Method)
		}
	}
}

// getGrpcRoutePathTypeAndLength returns path type for grpc
func getGrpcRoutePathType(method *v1.GRPCMethodMatch) int {
	if method == nil {
		return 0
	}
	switch *method.Type {
	case v1.GRPCMethodMatchExact:
		return 3
	case v1.GRPCMethodMatchRegularExpression:
		return 1
	default:
		return 0
	}
}

// compareRulePrecedenceUnified provides a single comparator that handles both
// same-kind and cross-kind rule precedence. For same-kind comparisons it
// delegates to the existing kind-specific comparators (which preserve
// intra-kind semantics like GRPC's service > method ordering). For cross-kind
// comparisons it normalizes both kinds to comparable specificity values.
func compareRulePrecedenceUnified(ruleOne, ruleTwo RulePrecedence) bool {
	oneIsHTTP := ruleOne.HttpSpecificRulePrecedenceFactor != nil
	twoIsHTTP := ruleTwo.HttpSpecificRulePrecedenceFactor != nil

	// Same kind: delegate to existing comparators to preserve intra-kind
	// semantics (e.g. GRPC service length > method length ordering).
	if oneIsHTTP && twoIsHTTP {
		return compareHttpRulePrecedence(ruleOne, ruleTwo)
	}
	if !oneIsHTTP && !twoIsHTTP {
		return compareGrpcRulePrecedence(ruleOne, ruleTwo)
	}

	// Cross-kind: use normalized specificity comparison.
	return compareCrossKindRulePrecedence(ruleOne, ruleTwo)
}

// compareCrossKindRulePrecedence compares an HTTPRoute rule against a GRPCRoute
// rule (or vice versa) using normalized specificity factors so that route kind
// alone does not determine priority.
//
// Cross-kind precedence ordering (most significant first):
//  1. Hostname specificity (exact > wildcard > none; more dots > fewer dots)
//  2. Path type (exact=3 > prefix=2 > regex=1 > none=0)
//  3. Effective path length (longer = more specific)
//  4. HTTP method constraint (hasMethod=true > false; GRPC routes are treated
//     as hasMethod=false since their method specificity is already captured in
//     path length via ServiceLength + MethodLength)
//  5. Header count (more headers = more specific)
//  6. Additional conditions (query params)
//  7. Common tiebreakers (creation timestamp, namespaced name, rule/match index)
func compareCrossKindRulePrecedence(ruleOne, ruleTwo RulePrecedence) bool {
	// 1. Hostname precedence
	precedence := getHostnameListPrecedenceOrder(ruleOne.CommonRulePrecedence.Hostnames, ruleTwo.CommonRulePrecedence.Hostnames)
	if precedence != 0 {
		return precedence < 0
	}

	// 2. Path type
	onePathType := ruleOne.normalizedPathType()
	twoPathType := ruleTwo.normalizedPathType()
	if onePathType != twoPathType {
		return onePathType > twoPathType
	}

	// 3. Effective path length
	onePathLen := ruleOne.effectivePathLength()
	twoPathLen := ruleTwo.effectivePathLength()
	if onePathLen != twoPathLen {
		return onePathLen > twoPathLen
	}

	// 4. HTTP method constraint (hasMethod)
	// GRPC routes have no equivalent of HTTP method matching (their method
	// specificity is captured in path length), so they are treated as false.
	oneHasMethod := ruleOne.normalizedHasMethod()
	twoHasMethod := ruleTwo.normalizedHasMethod()
	if oneHasMethod != twoHasMethod {
		return oneHasMethod
	}

	// 5. Header count
	oneHeaders := ruleOne.normalizedHeaderCount()
	twoHeaders := ruleTwo.normalizedHeaderCount()
	if oneHeaders != twoHeaders {
		return oneHeaders > twoHeaders
	}

	// 6. Additional conditions (query params)
	oneExtra := ruleOne.additionalConditionCount()
	twoExtra := ruleTwo.additionalConditionCount()
	if oneExtra != twoExtra {
		return oneExtra > twoExtra
	}

	// 7. Common tiebreakers (timestamp, name, indices)
	return compareCommonTieBreakers(ruleOne, ruleTwo)
}

// normalizedPathType returns a comparable path type value.
// Scale: 3=exact, 2=prefix, 1=regex, 0=none/catch-all.
func (r RulePrecedence) normalizedPathType() int {
	if r.HttpSpecificRulePrecedenceFactor != nil {
		return r.HttpSpecificRulePrecedenceFactor.PathType
	}
	if r.GrpcSpecificRulePrecedenceFactor != nil {
		return r.GrpcSpecificRulePrecedenceFactor.PathType
	}
	return 0
}

// effectivePathLength returns the effective path length for cross-kind comparison.
// HTTP: literal path length. GRPC: ServiceLength + MethodLength + separators.
func (r RulePrecedence) effectivePathLength() int {
	if r.HttpSpecificRulePrecedenceFactor != nil {
		return r.HttpSpecificRulePrecedenceFactor.PathLength
	}
	if r.GrpcSpecificRulePrecedenceFactor != nil {
		length := r.GrpcSpecificRulePrecedenceFactor.ServiceLength + r.GrpcSpecificRulePrecedenceFactor.MethodLength
		if r.GrpcSpecificRulePrecedenceFactor.ServiceLength > 0 {
			length++ // leading /
		}
		if r.GrpcSpecificRulePrecedenceFactor.MethodLength > 0 {
			length++ // separator /
		}
		return length
	}
	return 0
}

// normalizedHeaderCount returns the header count for either route kind.
func (r RulePrecedence) normalizedHeaderCount() int {
	if r.HttpSpecificRulePrecedenceFactor != nil {
		return r.HttpSpecificRulePrecedenceFactor.HeaderCount
	}
	if r.GrpcSpecificRulePrecedenceFactor != nil {
		return r.GrpcSpecificRulePrecedenceFactor.HeaderCount
	}
	return 0
}

// normalizedHasMethod returns whether the rule has an HTTP method constraint.
// GRPC routes have no equivalent of HTTP method matching — their method
// specificity is already captured in the effective path length (ServiceLength +
// MethodLength), so they always return false here.
func (r RulePrecedence) normalizedHasMethod() bool {
	if r.HttpSpecificRulePrecedenceFactor != nil {
		return r.HttpSpecificRulePrecedenceFactor.HasMethod
	}
	return false
}

// additionalConditionCount returns extra specificity factors beyond path,
// hasMethod, and headers. HTTP routes can constrain query params; GRPC routes
// have no equivalent, so they return 0 (less specific on this axis).
func (r RulePrecedence) additionalConditionCount() int {
	if r.HttpSpecificRulePrecedenceFactor != nil {
		return r.HttpSpecificRulePrecedenceFactor.QueryParamCount
	}
	return 0
}
