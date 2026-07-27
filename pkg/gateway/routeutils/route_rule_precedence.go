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

	// PrecedenceFactor holds the specificity signals used to order this match.
	// A single struct is used for both HTTPRoute and GRPCRoute rules so that one
	// comparator can order every rule with a single uniform key — which is a
	// strict weak ordering, as sort.Slice requires.
	PrecedenceFactor *RulePrecedenceFactor
}

// RulePrecedenceFactor captures the per-match specificity signals for either
// route kind. The two length fields are populated differently per kind so that
// one uniform comparison key preserves each kind's Gateway API precedence:
//
//	         | PathLength (primary) | SecondaryLength | HasMethod          | QueryParamCount
//	HTTPRoute| path length          | 0               | method match set   | query param count
//	GRPCRoute| service length       | method length   | false              | 0
type RulePrecedenceFactor struct {
	PathType        int  // HTTP: 3=exact,2=prefix,1=regex,0=none. GRPC: 3=exact,1=regex,0=none.
	PathLength      int  // primary match length: HTTP path length / GRPC service length
	SecondaryLength int  // secondary match length: GRPC method length (0 for HTTP)
	HasMethod       bool // HTTP method match present (always false for GRPC)
	HeaderCount     int  // header match count
	QueryParamCount int  // HTTP query param count (always 0 for GRPC)
}

type CommonRulePrecedence struct {
	RouteDescriptor RouteDescriptor
	Rule            RouteRule

	// common rule precedence factors
	Hostname             string // the single (compatible) hostname this rule was split to; "" = catch-all (no host-header condition)
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
		// Split the route's (compatible) hostnames so each generated rule carries a
		// single hostname. This makes hostname precedence per-hostname (a scalar)
		// instead of collapsing a multi-hostname route down to one representative.
		// It is both more faithful to the Gateway API (precedence is defined per
		// matching hostname) and removes the representative bias where an unrelated
		// hostname could flip a route's ordering (e.g. a wildcard shadowing another
		// route's exact host). Each hostname unit becomes its own ALB rule/priority.
		hostnameUnits := splitHostnamesForPrecedence(getRouteHostnames(route, port))

		for ruleIndex, rule := range route.GetAttachedRules() {
			rawRule := rule.GetRawRouteRule()
			switch r := rawRule.(type) {
			case *v1.HTTPRouteRule:
				for matchIndex := range r.Matches {
					httpMatch := r.Matches[matchIndex]
					for _, hostname := range hostnameUnits {
						common := routeInfo
						common.Hostname = hostname
						common.Rule = rule
						common.RuleIndexInRoute = ruleIndex
						common.MatchIndexInRule = matchIndex
						match := RulePrecedence{
							HTTPMatch:            &httpMatch,
							PrecedenceFactor:     &RulePrecedenceFactor{},
							CommonRulePrecedence: common,
						}
						// populate PrecedenceFactor from the HTTP match
						getHttpMatchPrecedenceInfo(&httpMatch, &match)
						httpRoutes = append(httpRoutes, match)
					}
				}
				if len(r.Matches) == 0 {
					for _, hostname := range hostnameUnits {
						common := routeInfo
						common.Hostname = hostname
						common.Rule = rule
						common.RuleIndexInRoute = ruleIndex
						common.MatchIndexInRule = math.MaxInt
						match := RulePrecedence{
							HTTPMatch:            &v1.HTTPRouteMatch{},
							PrecedenceFactor:     &RulePrecedenceFactor{},
							CommonRulePrecedence: common,
						}
						httpRoutes = append(httpRoutes, match)
					}
				}
			case *v1.GRPCRouteRule:
				for matchIndex := range r.Matches {
					grpcMatch := r.Matches[matchIndex]
					for _, hostname := range hostnameUnits {
						common := routeInfo
						common.Hostname = hostname
						common.Rule = rule
						common.RuleIndexInRoute = ruleIndex
						common.MatchIndexInRule = matchIndex
						match := RulePrecedence{
							GRPCMatch:            &grpcMatch,
							PrecedenceFactor:     &RulePrecedenceFactor{},
							CommonRulePrecedence: common,
						}
						// populate PrecedenceFactor from the GRPC match
						getGrpcMatchPrecedenceInfo(&grpcMatch, &match)
						grpcRoutes = append(grpcRoutes, match)
					}
				}

				if len(r.Matches) == 0 {
					for _, hostname := range hostnameUnits {
						common := routeInfo
						common.Hostname = hostname
						common.Rule = rule
						common.RuleIndexInRoute = ruleIndex
						common.MatchIndexInRule = math.MaxInt
						match := RulePrecedence{
							GRPCMatch:            &v1.GRPCRouteMatch{},
							PrecedenceFactor:     &RulePrecedenceFactor{},
							CommonRulePrecedence: common,
						}
						grpcRoutes = append(grpcRoutes, match)
					}
				}
			}
		}
	}

	allRoutes = append(allRoutes, httpRoutes...)
	allRoutes = append(allRoutes, grpcRoutes...)

	// Sort every rule — HTTP or GRPC — with a single unified comparator. Using
	// one uniform key for all pairs (rather than separate same-kind and
	// cross-kind comparators) guarantees a strict weak ordering, which
	// sort.Slice requires.
	sort.Slice(allRoutes, func(i, j int) bool {
		return compareRulePrecedenceUnified(allRoutes[i], allRoutes[j])
	})

	return allRoutes
}

// splitHostnamesForPrecedence returns one entry per hostname the route serves on
// the port, so each generated ALB rule carries a single hostname and is assigned
// its own priority. A route with no hostnames yields a single catch-all entry
// ("" -> no host-header condition), preserving the behavior for hostname-less
// routes.
func splitHostnamesForPrecedence(hostnames []string) []string {
	if len(hostnames) == 0 {
		return []string{""}
	}
	return hostnames
}

// getHostnamePrecedenceOrder Hostname precedence ordering rule:
// 0. an empty hostname is a catch-all (matches every host) and is the least specific
// 1. non-wildcard has higher precedence than wildcard
// 2. hostname with longer characters have higher precedence than those with shorter ones
// -1 means hostnameOne has higher precedence, 1 means hostnameTwo has higher precedence, 0 means equal
func getHostnamePrecedenceOrder(hostnameOne, hostnameTwo string) int {
	// An empty hostname is a catch-all and therefore the least specific — it must
	// lose to any concrete hostname, including a wildcard. Handle it before the
	// wildcard check below, which would otherwise treat "" as a non-wildcard and
	// let it beat a wildcard.
	if hostnameOne == "" || hostnameTwo == "" {
		switch {
		case hostnameOne == "" && hostnameTwo == "":
			return 0
		case hostnameOne == "":
			return 1 // hostnameTwo (concrete) is more specific
		default:
			return -1 // hostnameOne (concrete) is more specific
		}
	}

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

// compareRulePrecedenceUnified is the single comparator used to order every
// rule — HTTP or GRPC — by one uniform specificity key. Returns true when
// ruleOne has higher precedence than ruleTwo (i.e. should sort earlier and be
// assigned a lower/first ALB priority).
//
// Using one key for all pairs (rather than separate same-kind and cross-kind
// comparators) guarantees a strict weak ordering, which sort.Slice requires.
// The earlier design compared same-kind GRPC rules by (serviceLength,
// methodLength) as separate keys while comparing cross-kind rules by their sum,
// which was non-transitive and produced an undefined, input-order-dependent ALB
// rule priority.
//
// Key, most significant first:
//  1. Hostname specificity (most-specific matching hostname)
//  2. Path match type (Exact=3 > Prefix=2 > Regex=1 > none=0)
//  3. Primary match length   (HTTP: path length;    GRPC: service length)
//  4. Secondary match length  (HTTP: 0;             GRPC: method length)
//  5. HTTP method constraint  (HTTP: hasMethod;     GRPC: always false)
//  6. Header match count
//  7. Query param count       (HTTP: query params;  GRPC: always 0)
//  8. Common tiebreakers (creation timestamp, namespaced name, rule/match index)
//
// Because each kind's "foreign" fields are constant within that kind
// (SecondaryLength is always 0 for HTTP, HasMethod always false and
// QueryParamCount always 0 for GRPC), this reproduces the previous intra-kind
// HTTP and GRPC orderings exactly while making cross-kind ordering consistent.
// In particular GRPC service and method remain separate keys (steps 3 and 4),
// preserving the Gateway API GRPC precedence (service characters, then method
// characters).
func compareRulePrecedenceUnified(ruleOne, ruleTwo RulePrecedence) bool {
	precedence := getHostnamePrecedenceOrder(ruleOne.CommonRulePrecedence.Hostname, ruleTwo.CommonRulePrecedence.Hostname)
	if precedence != 0 {
		return precedence < 0 // -1 means first hostname has higher precedence
	}

	one := ruleOne.PrecedenceFactor
	two := ruleTwo.PrecedenceFactor

	// path match type (exact > prefix > regex > none)
	if one.PathType != two.PathType {
		return one.PathType > two.PathType
	}
	// primary match length (HTTP path length / GRPC service length)
	if one.PathLength != two.PathLength {
		return one.PathLength > two.PathLength
	}
	// secondary match length (GRPC method length; 0 for HTTP)
	if one.SecondaryLength != two.SecondaryLength {
		return one.SecondaryLength > two.SecondaryLength
	}
	// HTTP method constraint (always false for GRPC)
	if one.HasMethod != two.HasMethod {
		return one.HasMethod
	}
	// header match count
	if one.HeaderCount != two.HeaderCount {
		return one.HeaderCount > two.HeaderCount
	}
	// query param count (0 for GRPC)
	if one.QueryParamCount != two.QueryParamCount {
		return one.QueryParamCount > two.QueryParamCount
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
	if ruleOne.CommonRulePrecedence.MatchIndexInRule != ruleTwo.CommonRulePrecedence.MatchIndexInRule {
		return ruleOne.CommonRulePrecedence.MatchIndexInRule < ruleTwo.CommonRulePrecedence.MatchIndexInRule
	}
	// Final deterministic tiebreaker. After per-hostname splitting, two units can
	// differ only by an equally specific hostname (e.g. example.com vs
	// example.net). These match disjoint requests, so their relative ALB priority
	// is functionally irrelevant — but ordering by the hostname string keeps the
	// sort output stable/deterministic (so sort.Slice doesn't reorder equivalent
	// elements across reconciles). This never fires for distinct routes, which are
	// separated earlier by namespaced name.
	return ruleOne.CommonRulePrecedence.Hostname < ruleTwo.CommonRulePrecedence.Hostname
}

func getCommonRouteInfo(route RouteDescriptor, port int32) CommonRulePrecedence {
	return CommonRulePrecedence{
		RouteDescriptor:      route,
		RouteCreateTimestamp: route.GetRouteCreateTimestamp(),
		RouteNamespacedName:  route.GetRouteNamespacedName().String(),
	}
}

// getRouteHostnames returns the hostnames a route serves on the given listener
// port: the compatible hostnames computed during attachment (listener ∩ route,
// narrowed to the more specific side), or the route's own hostnames when the
// listener has no hostname. An empty result means the route is a catch-all on
// this port.
func getRouteHostnames(route RouteDescriptor, port int32) []string {
	compatible := route.GetCompatibleHostnamesByPort()[port]
	hostnames := make([]string, 0, len(compatible))
	for _, h := range compatible {
		hostnames = append(hostnames, string(h))
	}
	// If no compatible hostnames, use route hostnames.
	if len(hostnames) == 0 {
		for _, h := range route.GetHostnames() {
			hostnames = append(hostnames, string(h))
		}
	}
	return hostnames
}

func getHttpMatchPrecedenceInfo(httpMatch *v1.HTTPRouteMatch, matchPrecedence *RulePrecedence) {
	matchPrecedence.PrecedenceFactor.PathType = getHttpRoutePathType(httpMatch.Path)
	// httpMatch.Path.Value won't be nil, default is /
	matchPrecedence.PrecedenceFactor.PathLength = len(*httpMatch.Path.Value)
	matchPrecedence.PrecedenceFactor.HasMethod = httpMatch.Method != nil
	matchPrecedence.PrecedenceFactor.HeaderCount = len(httpMatch.Headers)
	matchPrecedence.PrecedenceFactor.QueryParamCount = len(httpMatch.QueryParams)
	// SecondaryLength stays 0 for HTTP.
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

// getGrpcMatchPrecedenceInfo populates the unified PrecedenceFactor from a GRPC
// match: service length is the primary length, method length the secondary,
// preserving the Gateway API GRPC precedence (service characters, then method
// characters). HasMethod stays false and QueryParamCount stays 0 for GRPC.
func getGrpcMatchPrecedenceInfo(grpcMatch *v1.GRPCRouteMatch, matchPrecedence *RulePrecedence) {
	matchPrecedence.PrecedenceFactor.PathType = getGrpcRoutePathType(grpcMatch.Method)
	matchPrecedence.PrecedenceFactor.HeaderCount = len(grpcMatch.Headers)
	if grpcMatch.Method != nil {
		if grpcMatch.Method.Service != nil {
			matchPrecedence.PrecedenceFactor.PathLength = len(*grpcMatch.Method.Service)
		}
		if grpcMatch.Method.Method != nil {
			matchPrecedence.PrecedenceFactor.SecondaryLength = len(*grpcMatch.Method.Method)
		}
	}
}

// getGrpcRoutePathType returns path type for grpc
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
