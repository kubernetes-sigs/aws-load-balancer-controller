package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	"time"
)

/*
This class holds the representation of an TLS route.
Generally, outside consumers will use GetRawRouteRule to inspect the
TLS specific features of the route.
*/

/* Route Rule */

var _ RouteRule = &convertedTLSRouteRule{}

var defaultTLSRuleAccumulator = newAttachedRuleAccumulator[gwalpha2.TLSRouteRule](commonBackendLoader, listenerRuleConfigLoader)

type convertedTLSRouteRule struct {
	rule               *gwalpha2.TLSRouteRule
	backends           []Backend
	listenerRuleConfig *elbv2gw.ListenerRuleConfiguration
}

func convertTLSRouteRule(rule *gwalpha2.TLSRouteRule, backends []Backend) RouteRule {
	return &convertedTLSRouteRule{
		rule:               rule,
		backends:           backends,
		listenerRuleConfig: nil,
	}
}

func (t *convertedTLSRouteRule) GetRawRouteRule() interface{} {
	return t.rule
}

func (t *convertedTLSRouteRule) GetBackends() []Backend {
	return t.backends
}

func (t *convertedTLSRouteRule) GetListenerRuleConfig() *elbv2gw.ListenerRuleConfiguration {
	return nil
}

/* Route Description */

type tlsRouteDescription struct {
	route                     *gwalpha2.TLSRoute
	rules                     []RouteRule
	ruleAccumulator           attachedRuleAccumulator[gwalpha2.TLSRouteRule]
	compatibleHostnamesByPort map[int32][]gwv1.Hostname
	gatewayDefaultTGConfig    *elbv2gw.TargetGroupConfiguration
}

func (tlsRoute *tlsRouteDescription) GetAttachedRules() []RouteRule {
	return tlsRoute.rules
}

func (tlsRoute *tlsRouteDescription) setGatewayDefaultTGConfig(cfg *elbv2gw.TargetGroupConfiguration) {
	tlsRoute.gatewayDefaultTGConfig = cfg
}

func (tlsRoute *tlsRouteDescription) loadAttachedRules(ctx context.Context, k8sClient client.Client) (RouteDescriptor, []routeLoadError) {
	convertedRules, allErrors := tlsRoute.ruleAccumulator.accumulateRules(ctx, k8sClient, tlsRoute, tlsRoute.route.Spec.Rules, func(rule gwalpha2.TLSRouteRule) []gwv1.BackendRef {
		return rule.BackendRefs
	}, func(rule gwalpha2.TLSRouteRule) []gwv1.LocalObjectReference {
		return []gwv1.LocalObjectReference{}
	}, func(trr *gwalpha2.TLSRouteRule, backends []Backend, listenerRuleConfiguration *elbv2gw.ListenerRuleConfiguration) RouteRule {
		return convertTLSRouteRule(trr, backends)
	}, tlsRoute.gatewayDefaultTGConfig)
	tlsRoute.rules = convertedRules
	return tlsRoute, allErrors
}

func (tlsRoute *tlsRouteDescription) GetHostnames() []gwv1.Hostname {
	return tlsRoute.route.Spec.Hostnames
}

func (tlsRoute *tlsRouteDescription) GetParentRefs() []gwv1.ParentReference {
	return tlsRoute.route.Spec.ParentRefs
}

func (tlsRoute *tlsRouteDescription) GetRouteKind() RouteKind {
	return TLSRouteKind
}

func (tlsRoute *tlsRouteDescription) GetRouteGeneration() int64 {
	return tlsRoute.route.Generation
}

func convertTLSRoute(r gwalpha2.TLSRoute) *tlsRouteDescription {
	return &tlsRouteDescription{route: &r, ruleAccumulator: defaultTLSRuleAccumulator}
}

func (tlsRoute *tlsRouteDescription) GetRouteNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(tlsRoute.route)
}

func (tlsRoute *tlsRouteDescription) GetRouteIdentifier() string {
	return string(tlsRoute.GetRouteKind()) + "-" + tlsRoute.GetRouteNamespacedName().String()
}

func (tlsRoute *tlsRouteDescription) GetRawRoute() interface{} {
	return tlsRoute.route
}

func (tlsRoute *tlsRouteDescription) GetBackendRefs() []gwv1.BackendRef {
	backendRefs := make([]gwv1.BackendRef, 0)
	if tlsRoute.route.Spec.Rules != nil {
		for _, rule := range tlsRoute.route.Spec.Rules {
			backendRefs = append(backendRefs, rule.BackendRefs...)
		}
	}
	return backendRefs
}

func (tlsRoute *tlsRouteDescription) GetRouteListenerRuleConfigRefs() []gwv1.LocalObjectReference {
	return []gwv1.LocalObjectReference{}
}

func (tlsRoute *tlsRouteDescription) GetRouteCreateTimestamp() time.Time {
	return tlsRoute.route.CreationTimestamp.Time
}

func (tlsRoute *tlsRouteDescription) GetCompatibleHostnamesByPort() map[int32][]gwv1.Hostname {
	return tlsRoute.compatibleHostnamesByPort
}

func (tlsRoute *tlsRouteDescription) setCompatibleHostnamesByPort(hostnamesByPort map[int32][]gwv1.Hostname) {
	tlsRoute.compatibleHostnamesByPort = hostnamesByPort
}

var _ RouteDescriptor = &tlsRouteDescription{}

func ListTLSRoutes(context context.Context, k8sClient client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error) {
	routeList := &gwalpha2.TLSRouteList{}
	err := k8sClient.List(context, routeList, opts...)
	if err != nil {
		return nil, err
	}

	result := make([]preLoadRouteDescriptor, 0)

	for _, item := range routeList.Items {
		result = append(result, convertTLSRoute(item))
	}

	return result, nil
}
