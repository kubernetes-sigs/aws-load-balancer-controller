package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"time"
)

/*
This class holds the representation of a GRPC route.
Generally, outside consumers will use GetRawRouteRule to inspect the
GRPC specific features of the route.
*/

/* Route Rule */

var _ RouteRule = &convertedGRPCRouteRule{}

var defaultGRPCRuleAccumulator = newAttachedRuleAccumulator[gwv1.GRPCRouteRule](commonBackendLoader, listenerRuleConfigLoader)

type convertedGRPCRouteRule struct {
	rule               *gwv1.GRPCRouteRule
	backends           []Backend
	listenerRuleConfig *elbv2gw.ListenerRuleConfiguration
}

func (t *convertedGRPCRouteRule) GetRawRouteRule() interface{} {
	return t.rule
}

func convertGRPCRouteRule(rule *gwv1.GRPCRouteRule, backends []Backend, listenerRuleConfig *elbv2gw.ListenerRuleConfiguration) RouteRule {
	return &convertedGRPCRouteRule{
		rule:               rule,
		backends:           backends,
		listenerRuleConfig: listenerRuleConfig,
	}
}

func (t *convertedGRPCRouteRule) GetBackends() []Backend {
	return t.backends
}

func (t *convertedGRPCRouteRule) GetListenerRuleConfig() *elbv2gw.ListenerRuleConfiguration {
	return t.listenerRuleConfig
}

/* Route Description */

type grpcRouteDescription struct {
	route                     *gwv1.GRPCRoute
	rules                     []RouteRule
	ruleAccumulator           attachedRuleAccumulator[gwv1.GRPCRouteRule]
	compatibleHostnamesByPort map[int32][]gwv1.Hostname
	gatewayDefaultTGConfig    *elbv2gw.TargetGroupConfiguration
}

func (grpcRoute *grpcRouteDescription) setGatewayDefaultTGConfig(cfg *elbv2gw.TargetGroupConfiguration) {
	grpcRoute.gatewayDefaultTGConfig = cfg
}

func (grpcRoute *grpcRouteDescription) loadAttachedRules(ctx context.Context, k8sClient client.Client) (RouteDescriptor, []routeLoadError) {
	convertedRules, allErrors := grpcRoute.ruleAccumulator.accumulateRules(ctx, k8sClient, grpcRoute, grpcRoute.route.Spec.Rules,
		func(rule gwv1.GRPCRouteRule) []gwv1.BackendRef {
			refs := make([]gwv1.BackendRef, 0, len(rule.BackendRefs))
			for _, grpcRef := range rule.BackendRefs {
				refs = append(refs, grpcRef.BackendRef)
			}
			return refs
		}, func(rule gwv1.GRPCRouteRule) []gwv1.LocalObjectReference {
			return getListenerRuleConfigForRuleGeneric(rule.Filters, func(filter gwv1.GRPCRouteFilter) bool {
				return filter.Type == gwv1.GRPCRouteFilterExtensionRef
			}, func(filter gwv1.GRPCRouteFilter) *gwv1.LocalObjectReference {
				return filter.ExtensionRef
			})
		}, func(grr *gwv1.GRPCRouteRule, backends []Backend, listenerRuleConfiguration *elbv2gw.ListenerRuleConfiguration) RouteRule {
			return convertGRPCRouteRule(grr, backends, listenerRuleConfiguration)
		}, grpcRoute.gatewayDefaultTGConfig)
	grpcRoute.rules = convertedRules
	return grpcRoute, allErrors
}

func (grpcRoute *grpcRouteDescription) GetHostnames() []gwv1.Hostname {
	return grpcRoute.route.Spec.Hostnames
}

func (grpcRoute *grpcRouteDescription) GetAttachedRules() []RouteRule {
	return grpcRoute.rules
}

func (grpcRoute *grpcRouteDescription) GetParentRefs() []gwv1.ParentReference {
	return grpcRoute.route.Spec.ParentRefs
}

func (grpcRoute *grpcRouteDescription) GetRouteKind() RouteKind {
	return GRPCRouteKind
}

func (grpcRoute *grpcRouteDescription) GetRouteNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(grpcRoute.route)
}

func (grpcRoute *grpcRouteDescription) GetRouteIdentifier() string {
	return string(grpcRoute.GetRouteKind()) + "-" + grpcRoute.GetRouteNamespacedName().String()
}

func convertGRPCRoute(r gwv1.GRPCRoute) *grpcRouteDescription {
	return &grpcRouteDescription{route: &r, ruleAccumulator: defaultGRPCRuleAccumulator}
}

func (grpcRoute *grpcRouteDescription) GetRawRoute() interface{} {
	return grpcRoute.route
}

func (grpcRoute *grpcRouteDescription) GetBackendRefs() []gwv1.BackendRef {
	backendRefs := make([]gwv1.BackendRef, 0)
	if grpcRoute.route.Spec.Rules != nil {
		for _, rule := range grpcRoute.route.Spec.Rules {
			for _, grpcBackendRef := range rule.BackendRefs {
				backendRefs = append(backendRefs, grpcBackendRef.BackendRef)
			}
		}
	}
	return backendRefs
}

// GetListenerRuleConfigs returns all ListenerRuleConfiguration references from
// ExtensionRef filters in the GRPCRoute
func (grpcRoute *grpcRouteDescription) GetRouteListenerRuleConfigRefs() []gwv1.LocalObjectReference {
	listenerRuleConfigs := make([]gwv1.LocalObjectReference, 0)
	if grpcRoute.route.Spec.Rules != nil {
		for _, rule := range grpcRoute.route.Spec.Rules {
			cfgList := getListenerRuleConfigForRuleGeneric(rule.Filters,
				func(filter gwv1.GRPCRouteFilter) bool {
					return filter.Type == gwv1.GRPCRouteFilterExtensionRef
				}, func(filter gwv1.GRPCRouteFilter) *gwv1.LocalObjectReference {
					return filter.ExtensionRef
				})
			listenerRuleConfigs = append(listenerRuleConfigs, cfgList...)
		}
	}
	return listenerRuleConfigs
}

func (grpcRoute *grpcRouteDescription) GetRouteGeneration() int64 {
	return grpcRoute.route.Generation
}

func (grpcRoute *grpcRouteDescription) GetRouteCreateTimestamp() time.Time {
	return grpcRoute.route.CreationTimestamp.Time
}

func (grpcRoute *grpcRouteDescription) GetCompatibleHostnamesByPort() map[int32][]gwv1.Hostname {
	return grpcRoute.compatibleHostnamesByPort
}

func (grpcRoute *grpcRouteDescription) setCompatibleHostnamesByPort(hostnamesByPort map[int32][]gwv1.Hostname) {
	grpcRoute.compatibleHostnamesByPort = hostnamesByPort
}

var _ RouteDescriptor = &grpcRouteDescription{}

// Can we use an indexer here to query more efficiently?

func ListGRPCRoutes(context context.Context, k8sClient client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error) {
	routeList := &gwv1.GRPCRouteList{}
	err := k8sClient.List(context, routeList, opts...)
	if err != nil {
		return nil, err
	}

	result := make([]preLoadRouteDescriptor, 0)

	for _, item := range routeList.Items {
		result = append(result, convertGRPCRoute(item))
	}

	return result, nil
}
