package routeutils

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/gateway/crddetect"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

/*
This class holds the representation of an TCP route.
Generally, outside consumers will use GetRawRouteRule to inspect the
TCP specific features of the route.
*/

/* Route Rule */

var _ RouteRule = &convertedTCPRouteRule{}

var defaultTCPRuleAccumulator = newAttachedRuleAccumulator[gwv1.TCPRouteRule](commonBackendLoader, listenerRuleConfigLoader)

type convertedTCPRouteRule struct {
	rule               *gwv1.TCPRouteRule
	backends           []Backend
	listenerRuleConfig *elbv2gw.ListenerRuleConfiguration
}

func convertTCPRouteRule(rule *gwv1.TCPRouteRule, backends []Backend) RouteRule {
	return &convertedTCPRouteRule{
		rule:               rule,
		backends:           backends,
		listenerRuleConfig: nil,
	}
}

func (t *convertedTCPRouteRule) GetRawRouteRule() interface{} {
	return t.rule
}

func (t *convertedTCPRouteRule) GetBackends() []Backend {
	return t.backends
}

func (t *convertedTCPRouteRule) GetListenerRuleConfig() *elbv2gw.ListenerRuleConfiguration {
	return nil
}

/* Route Description */

type tcpRouteDescription struct {
	route                     *gwv1.TCPRoute
	rules                     []RouteRule
	ruleAccumulator           attachedRuleAccumulator[gwv1.TCPRouteRule]
	compatibleHostnamesByPort map[int32][]gwv1.Hostname
}

func (tcpRoute *tcpRouteDescription) GetAttachedRules() []RouteRule {
	return tcpRoute.rules
}

func (tcpRoute *tcpRouteDescription) loadAttachedRules(ctx context.Context, k8sClient client.Client, gatewayDefaultTGConfig *elbv2gw.TargetGroupConfiguration) (RouteDescriptor, []routeLoadError) {
	convertedRules, allErrors := tcpRoute.ruleAccumulator.accumulateRules(ctx, k8sClient, tcpRoute, tcpRoute.route.Spec.Rules, func(rule gwv1.TCPRouteRule) []gwv1.BackendRef {
		return rule.BackendRefs
	}, func(rule gwv1.TCPRouteRule) []gwv1.LocalObjectReference {
		return []gwv1.LocalObjectReference{}
	}, func(trr *gwv1.TCPRouteRule, backends []Backend, listenerRuleConfiguration *elbv2gw.ListenerRuleConfiguration) RouteRule {
		return convertTCPRouteRule(trr, backends)
	}, gatewayDefaultTGConfig)
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

func (tcpRoute *tcpRouteDescription) GetRouteIdentifier() string {
	return string(tcpRoute.GetRouteKind()) + "-" + tcpRoute.GetRouteNamespacedName().String()
}

func convertTCPRoute(r gwv1.TCPRoute) *tcpRouteDescription {
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

func (tcpRoute *tcpRouteDescription) GetRouteListenerRuleConfigRefs() []gwv1.LocalObjectReference {
	return []gwv1.LocalObjectReference{}
}

func (tcpRoute *tcpRouteDescription) GetRouteGeneration() int64 {
	return tcpRoute.route.Generation
}

func (tcpRoute *tcpRouteDescription) GetRouteCreateTimestamp() time.Time {
	return tcpRoute.route.CreationTimestamp.Time
}

func (tcpRoute *tcpRouteDescription) GetCompatibleHostnamesByPort() map[int32][]gwv1.Hostname {
	return tcpRoute.compatibleHostnamesByPort
}

func (tcpRoute *tcpRouteDescription) setCompatibleHostnamesByPort(hostnamesByPort map[int32][]gwv1.Hostname) {
	tcpRoute.compatibleHostnamesByPort = hostnamesByPort
}

var _ RouteDescriptor = &tcpRouteDescription{}

// Can we use an indexer here to query more efficiently?

// ListTCPRoutes lists TCPRoutes using the group version resolved at startup
// and returns them as preLoadRouteDescriptors over the v1 representation.
func ListTCPRoutes(context context.Context, versions crddetect.RouteVersions, k8sClient client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error) {
	routes, err := ListRawTCPRoutes(context, versions, k8sClient, opts...)
	if err != nil {
		return nil, err
	}

	result := make([]preLoadRouteDescriptor, 0, len(routes))

	for _, item := range routes {
		result = append(result, convertTCPRoute(item))
	}
	return result, nil
}

// ListRawTCPRoutes lists TCPRoutes in their v1 representation, querying the
// API server at the group version resolved at startup. On clusters serving
// only v1alpha2 (Gateway API < 1.6) the items are converted at this boundary.
func ListRawTCPRoutes(context context.Context, versions crddetect.RouteVersions, k8sClient client.Client, opts ...client.ListOption) ([]gwv1.TCPRoute, error) {
	if versions.IsTCPRouteV1() {
		routeList := &gwv1.TCPRouteList{}
		if err := k8sClient.List(context, routeList, opts...); err != nil {
			return nil, err
		}
		return routeList.Items, nil
	}

	alphaRouteList := &gwalpha2.TCPRouteList{}
	if err := k8sClient.List(context, alphaRouteList, opts...); err != nil {
		return nil, err
	}
	items := make([]gwv1.TCPRoute, 0, len(alphaRouteList.Items))
	for i := range alphaRouteList.Items {
		items = append(items, *ConvertAlpha2TCPRouteToV1(&alphaRouteList.Items[i]))
	}
	return items, nil
}
