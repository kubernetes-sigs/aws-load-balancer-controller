package routeutils

import (
	"context"
	corev1 "k8s.io/api/core/v1"
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
	for _, config := range route.GetRouteListenerRuleConfigRefs() {
		namespace := route.GetRouteNamespacedName().Namespace
		if string(config.Name) == ruleConfig.Name && namespace == ruleConfig.Namespace {
			return true
		}
	}
	return false
}

// FilterListenerRuleConfigBySecret filters listenerruleconfigs based on secret reference.
// Returns a list of ListenerRuleConfiguration objects that reference the given secret.
func FilterListenerRuleConfigBySecret(ctx context.Context, k8sClient client.Client, secret *corev1.Secret) ([]*elbv2gw.ListenerRuleConfiguration, error) {
	if secret == nil {
		return nil, nil
	}

	// List all ListenerRuleConfiguration objects in same namespace as secret
	listenerRuleCfgList := &elbv2gw.ListenerRuleConfigurationList{}
	listOpts := []client.ListOption{
		client.InNamespace(secret.Namespace), // namespace-scoped search
	}
	if err := k8sClient.List(ctx, listenerRuleCfgList, listOpts...); err != nil {
		return nil, err
	}

	var matchingConfigs []*elbv2gw.ListenerRuleConfiguration
	secretKey := k8s.NamespacedName(secret)

	// Iterate through each ListenerRuleConfiguration
	for i := range listenerRuleCfgList.Items {
		listenerRuleCfg := &listenerRuleCfgList.Items[i]

		if isListenerRuleConfigReferencingSecret(listenerRuleCfg, secretKey) {
			matchingConfigs = append(matchingConfigs, listenerRuleCfg)
		}
	}

	return matchingConfigs, nil
}

// isListenerRuleConfigReferencingSecret checks if a ListenerRuleConfiguration references a specific secret.
func isListenerRuleConfigReferencingSecret(listenerRuleCfg *elbv2gw.ListenerRuleConfiguration, secretKey types.NamespacedName) bool {
	if listenerRuleCfg.Spec.Actions == nil {
		return false
	}

	for _, action := range listenerRuleCfg.Spec.Actions {
		// Check authenticate-oidc actions for secret references
		if action.Type == elbv2gw.ActionTypeAuthenticateOIDC && action.AuthenticateOIDCConfig != nil && action.AuthenticateOIDCConfig.Secret != nil {
			referencedSecretName := action.AuthenticateOIDCConfig.Secret.Name
			referencedSecretNamespace := listenerRuleCfg.Namespace

			// Check if this secret reference matches the provided secret
			if referencedSecretName == secretKey.Name && referencedSecretNamespace == secretKey.Namespace {
				return true
			}
		}
	}
	return false
}
