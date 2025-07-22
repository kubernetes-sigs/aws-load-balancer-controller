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
This class holds the representation of an UDP route.
Generally, outside consumers will use GetRawRouteRule to inspect the
UDP specific features of the route.
*/

/* Route Rule */

var _ RouteRule = &convertedUDPRouteRule{}

var defaultUDPRuleAccumulator = newAttachedRuleAccumulator[gwalpha2.UDPRouteRule](commonBackendLoader)

type convertedUDPRouteRule struct {
	rule     *gwalpha2.UDPRouteRule
	backends []Backend
}

func convertUDPRouteRule(rule *gwalpha2.UDPRouteRule, backends []Backend) RouteRule {
	return &convertedUDPRouteRule{
		rule:     rule,
		backends: backends,
	}
}

func (t *convertedUDPRouteRule) GetRawRouteRule() interface{} {
	return t.rule
}

func (t *convertedUDPRouteRule) GetSectionName() *gwv1.SectionName {
	return t.rule.Name
}

func (t *convertedUDPRouteRule) GetBackends() []Backend {
	return t.backends
}

/* Route Description */

type udpRouteDescription struct {
	route           *gwalpha2.UDPRoute
	rules           []RouteRule
	ruleAccumulator attachedRuleAccumulator[gwalpha2.UDPRouteRule]
}

func (udpRoute *udpRouteDescription) GetAttachedRules() []RouteRule {
	return udpRoute.rules
}

func (udpRoute *udpRouteDescription) loadAttachedRules(ctx context.Context, k8sClient client.Client) (RouteDescriptor, []routeLoadError) {
	convertedRules, allErrors := udpRoute.ruleAccumulator.accumulateRules(ctx, k8sClient, udpRoute, udpRoute.route.Spec.Rules, func(rule gwalpha2.UDPRouteRule) []gwv1.BackendRef {
		return rule.BackendRefs
	}, func(urr *gwalpha2.UDPRouteRule, backends []Backend) RouteRule {
		return convertUDPRouteRule(urr, backends)
	})
	udpRoute.rules = convertedRules
	return udpRoute, allErrors
}

func (udpRoute *udpRouteDescription) GetHostnames() []gwv1.Hostname {
	return []gwv1.Hostname{}
}

func (udpRoute *udpRouteDescription) GetParentRefs() []gwv1.ParentReference {
	return udpRoute.route.Spec.ParentRefs
}

func (udpRoute *udpRouteDescription) GetRouteKind() RouteKind {
	return UDPRouteKind
}

func (udpRoute *udpRouteDescription) GetRouteGeneration() int64 {
	return udpRoute.route.Generation
}

func convertUDPRoute(r gwalpha2.UDPRoute) *udpRouteDescription {
	return &udpRouteDescription{route: &r, ruleAccumulator: defaultUDPRuleAccumulator}
}

func (udpRoute *udpRouteDescription) GetRouteNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(udpRoute.route)
}

func (udpRoute *udpRouteDescription) GetRawRoute() interface{} {
	return udpRoute.route
}

func (udpRoute *udpRouteDescription) GetBackendRefs() []gwv1.BackendRef {
	backendRefs := make([]gwv1.BackendRef, 0)
	if udpRoute.route.Spec.Rules != nil {
		for _, rule := range udpRoute.route.Spec.Rules {
			backendRefs = append(backendRefs, rule.BackendRefs...)
		}
	}
	return backendRefs
}
func (udpRoute *udpRouteDescription) GetListenerRuleConfigs() []gwv1.LocalObjectReference {
	return []gwv1.LocalObjectReference{}
}

func (udpRoute *udpRouteDescription) GetRouteCreateTimestamp() time.Time {
	return udpRoute.route.CreationTimestamp.Time
}

var _ RouteDescriptor = &udpRouteDescription{}

func ListUDPRoutes(context context.Context, k8sClient client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error) {
	routeList := &gwalpha2.UDPRouteList{}
	err := k8sClient.List(context, routeList, opts...)
	if err != nil {
		return nil, err
	}

	result := make([]preLoadRouteDescriptor, 0)

	for _, item := range routeList.Items {
		result = append(result, convertUDPRoute(item))
	}

	return result, nil
}
