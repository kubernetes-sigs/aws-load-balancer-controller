package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	"time"
)

/*
This class holds the representation of an TCP route.
Generally, outside consumers will use GetRawRouteRule to inspect the
TCP specific features of the route.
*/

/* Route Rule */

var _ RouteRule = &convertedTCPRouteRule{}

var defaultTCPRuleAccumulator = newAttachedRuleAccumulator[gwalpha2.TCPRouteRule](commonBackendLoader)

type convertedTCPRouteRule struct {
	rule     *gwalpha2.TCPRouteRule
	backends []Backend
}

func convertTCPRouteRule(rule *gwalpha2.TCPRouteRule, backends []Backend) RouteRule {
	return &convertedTCPRouteRule{
		rule:     rule,
		backends: backends,
	}
}

func (t *convertedTCPRouteRule) GetRawRouteRule() interface{} {
	return t.rule
}

func (t *convertedTCPRouteRule) GetSectionName() *gwv1.SectionName {
	return t.rule.Name
}

func (t *convertedTCPRouteRule) GetBackends() []Backend {
	return t.backends
}

/* Route Description */

type tcpRouteDescription struct {
	route           *gwalpha2.TCPRoute
	rules           []RouteRule
	ruleAccumulator attachedRuleAccumulator[gwalpha2.TCPRouteRule]
}

func (tcpRoute *tcpRouteDescription) GetAttachedRules() []RouteRule {
	return tcpRoute.rules
}

func (tcpRoute *tcpRouteDescription) loadAttachedRules(ctx context.Context, k8sClient client.Client) (RouteDescriptor, []routeLoadError) {
	convertedRules, allErrors := tcpRoute.ruleAccumulator.accumulateRules(ctx, k8sClient, tcpRoute, tcpRoute.route.Spec.Rules, func(rule gwalpha2.TCPRouteRule) []gwv1.BackendRef {
		return rule.BackendRefs
	}, func(trr *gwalpha2.TCPRouteRule, backends []Backend) RouteRule {
		return convertTCPRouteRule(trr, backends)
	})
	tcpRoute.rules = convertedRules
	return tcpRoute, allErrors
}

func (tcpRoute *tcpRouteDescription) GetHostnames() []gwv1.Hostname {
	return []gwv1.Hostname{}
}

func (tcpRoute *tcpRouteDescription) GetRouteKind() RouteKind {
	return TCPRouteKind
}

func (tcpRoute *tcpRouteDescription) GetRouteNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(tcpRoute.route)
}

func convertTCPRoute(r gwalpha2.TCPRoute) *tcpRouteDescription {
	return &tcpRouteDescription{route: &r, ruleAccumulator: defaultTCPRuleAccumulator}
}

func (tcpRoute *tcpRouteDescription) GetRawRoute() interface{} {
	return tcpRoute.route
}

func (tcpRoute *tcpRouteDescription) GetParentRefs() []gwv1.ParentReference {
	return tcpRoute.route.Spec.ParentRefs
}

func (tcpRoute *tcpRouteDescription) GetBackendRefs() []gwv1.BackendRef {
	backendRefs := make([]gwv1.BackendRef, 0)
	if tcpRoute.route.Spec.Rules != nil {
		for _, rule := range tcpRoute.route.Spec.Rules {
			backendRefs = append(backendRefs, rule.BackendRefs...)
		}
	}
	return backendRefs
}

func (tcpRoute *tcpRouteDescription) GetRouteGeneration() int64 {
	return tcpRoute.route.Generation
}

func (tcpRoute *tcpRouteDescription) GetRouteCreateTimestamp() time.Time {
	return tcpRoute.route.CreationTimestamp.Time
}

var _ RouteDescriptor = &tcpRouteDescription{}

// Can we use an indexer here to query more efficiently?

func ListTCPRoutes(context context.Context, k8sClient client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error) {
	routeList := &gwalpha2.TCPRouteList{}
	err := k8sClient.List(context, routeList, opts...)
	if err != nil {
		return nil, err
	}

	result := make([]preLoadRouteDescriptor, 0)

	for _, item := range routeList.Items {
		result = append(result, convertTCPRoute(item))
	}
	return result, nil
}
