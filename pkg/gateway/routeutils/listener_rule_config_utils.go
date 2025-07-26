package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func IsListenerRuleConfigInUse(ctx context.Context, listenerRuleConfig *elbv2gw.ListenerRuleConfiguration, k8sClient client.Client) (bool, error) {
	l7Routes, err := ListL7Routes(ctx, k8sClient)
	if err != nil {
		return false, err
	}
	filteredRoutesByListenerRuleCfg := FilterRoutesByListenerRuleCfg(l7Routes, listenerRuleConfig)

	return len(filteredRoutesByListenerRuleCfg) > 0, nil
}

// FilterRoutesByListenerRuleCfg filters a slice of routes based on ListenerRuleConfiguration reference.
// Returns a new slice containing only routes that reference the specified ListenerRuleConfiguration.
func FilterRoutesByListenerRuleCfg(routes []preLoadRouteDescriptor, ruleConfig *elbv2gw.ListenerRuleConfiguration) []preLoadRouteDescriptor {
	if ruleConfig == nil || len(routes) == 0 {
		return []preLoadRouteDescriptor{}
	}
	filteredRoutes := make([]preLoadRouteDescriptor, 0, len(routes))
	for _, route := range routes {
		if isListenerRuleConfigReferredByRoute(route, k8s.NamespacedName(ruleConfig)) {
			filteredRoutes = append(filteredRoutes, route)
		}
	}
	return filteredRoutes
}

// isListenerRuleConfigReferredByRoute checks if a route references a specific ruleConfig.
func isListenerRuleConfigReferredByRoute(route preLoadRouteDescriptor, ruleConfig types.NamespacedName) bool {
	for _, config := range route.GetListenerRuleConfigs() {
		namespace := route.GetRouteNamespacedName().Namespace
		if string(config.Name) == ruleConfig.Name && namespace == ruleConfig.Namespace {
			return true
		}
	}
	return false
}
