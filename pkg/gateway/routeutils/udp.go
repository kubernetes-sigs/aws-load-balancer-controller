package routeutils

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/crddetect"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

/*
This class holds the representation of an UDP route.
Generally, outside consumers will use GetRawRouteRule to inspect the
UDP specific features of the route.
*/

/* Route Rule */

var _ RouteRule = &convertedUDPRouteRule{}

var defaultUDPRuleAccumulator = newAttachedRuleAccumulator[gwv1.UDPRouteRule](commonBackendLoader, listenerRuleConfigLoader)

type convertedUDPRouteRule struct {
	rule               *gwv1.UDPRouteRule
	backends           []Backend
	listenerRuleConfig *elbv2gw.ListenerRuleConfiguration
}

func convertUDPRouteRule(rule *gwv1.UDPRouteRule, backends []Backend) RouteRule {
	return &convertedUDPRouteRule{
		rule:               rule,
		backends:           backends,
		listenerRuleConfig: nil,
	}
}

func (t *convertedUDPRouteRule) GetRawRouteRule() interface{} {
	return t.rule
}

func (t *convertedUDPRouteRule) GetBackends() []Backend {
	return t.backends
}

func (t *convertedUDPRouteRule) GetListenerRuleConfig() *elbv2gw.ListenerRuleConfiguration {
	return nil
}

/* Route Description */

type udpRouteDescription struct {
	route                     *gwv1.UDPRoute
	rules                     []RouteRule
	ruleAccumulator           attachedRuleAccumulator[gwv1.UDPRouteRule]
	compatibleHostnamesByPort map[int32][]gwv1.Hostname
}

func (udpRoute *udpRouteDescription) GetAttachedRules() []RouteRule {
	return udpRoute.rules
}

func (udpRoute *udpRouteDescription) loadAttachedRules(ctx context.Context, k8sClient client.Client, gatewayDefaultTGConfig *elbv2gw.TargetGroupConfiguration) (RouteDescriptor, []routeLoadError) {
	convertedRules, allErrors := udpRoute.ruleAccumulator.accumulateRules(ctx, k8sClient, udpRoute, udpRoute.route.Spec.Rules, func(rule gwv1.UDPRouteRule) []gwv1.BackendRef {
		return rule.BackendRefs
	}, func(rule gwv1.UDPRouteRule) []gwv1.LocalObjectReference {
		return []gwv1.LocalObjectReference{}
	}, func(urr *gwv1.UDPRouteRule, backends []Backend, listenerRuleConfiguration *elbv2gw.ListenerRuleConfiguration) RouteRule {
		return convertUDPRouteRule(urr, backends)
	}, gatewayDefaultTGConfig)

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

func convertUDPRoute(r gwv1.UDPRoute) *udpRouteDescription {
	return &udpRouteDescription{route: &r, ruleAccumulator: defaultUDPRuleAccumulator}
}

func (udpRoute *udpRouteDescription) GetRouteNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(udpRoute.route)
}

func (udpRoute *udpRouteDescription) GetRouteIdentifier() string {
	return string(udpRoute.GetRouteKind()) + "-" + udpRoute.GetRouteNamespacedName().String()
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
func (udpRoute *udpRouteDescription) GetRouteListenerRuleConfigRefs() []gwv1.LocalObjectReference {
	return []gwv1.LocalObjectReference{}
}

func (udpRoute *udpRouteDescription) GetRouteCreateTimestamp() time.Time {
	return udpRoute.route.CreationTimestamp.Time
}

func (udpRoute *udpRouteDescription) GetCompatibleHostnamesByPort() map[int32][]gwv1.Hostname {
	return udpRoute.compatibleHostnamesByPort
}

func (udpRoute *udpRouteDescription) setCompatibleHostnamesByPort(hostnamesByPort map[int32][]gwv1.Hostname) {
	udpRoute.compatibleHostnamesByPort = hostnamesByPort
}

var _ RouteDescriptor = &udpRouteDescription{}

// ListUDPRoutes lists UDPRoutes using the group version resolved at startup
// and returns them as preLoadRouteDescriptors over the v1 representation.
func ListUDPRoutes(context context.Context, versions crddetect.RouteVersions, k8sClient client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error) {
	routes, err := listRawUDPRoutes(context, versions, k8sClient, opts...)
	if err != nil {
		return nil, err
	}

	result := make([]preLoadRouteDescriptor, 0, len(routes))

	for _, item := range routes {
		result = append(result, convertUDPRoute(item))
	}

	return result, nil
}

// listRawUDPRoutes lists UDPRoutes in their v1 representation, querying the
// API server at the group version resolved at startup. On clusters serving
// only v1alpha2 (Gateway API < 1.6) the items are converted at this boundary.
func listRawUDPRoutes(context context.Context, versions crddetect.RouteVersions, k8sClient client.Client, opts ...client.ListOption) ([]gwv1.UDPRoute, error) {
	if versions.IsUDPRouteV1() {
		routeList := &gwv1.UDPRouteList{}
		if err := k8sClient.List(context, routeList, opts...); err != nil {
			return nil, err
		}
		return routeList.Items, nil
	}

	alphaRouteList := &gwalpha2.UDPRouteList{}
	if err := k8sClient.List(context, alphaRouteList, opts...); err != nil {
		return nil, err
	}
	items := make([]gwv1.UDPRoute, 0, len(alphaRouteList.Items))
	for i := range alphaRouteList.Items {
		items = append(items, *ConvertAlpha2UDPRouteToV1(&alphaRouteList.Items[i]))
	}
	return items, nil
}
