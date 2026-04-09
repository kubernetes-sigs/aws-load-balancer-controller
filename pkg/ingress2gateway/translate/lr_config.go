package translate

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	gwconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// buildListenerRuleConfiguration creates a skeleton ListenerRuleConfiguration with metadata.
func buildListenerRuleConfiguration(namespace, ingName, svcName string) *gatewayv1beta1.ListenerRuleConfiguration {
	return &gatewayv1beta1.ListenerRuleConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: utils.LBConfigAPIVersion,
			Kind:       gwconstants.ListenerRuleConfiguration,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.GetLRConfigName(namespace, ingName, svcName),
			Namespace: namespace,
		},
	}
}

// extensionRefFilter creates an HTTPRouteFilter that references a ListenerRuleConfiguration via ExtensionRef.
func extensionRefFilter(lrcName string) gwv1.HTTPRouteFilter {
	group := gwv1.Group(utils.LBCGatewayAPIGroup)
	kind := gwv1.Kind(gwconstants.ListenerRuleConfiguration)
	return gwv1.HTTPRouteFilter{
		Type: gwv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gwv1.LocalObjectReference{
			Group: group,
			Kind:  kind,
			Name:  gwv1.ObjectName(lrcName),
		},
	}
}

// findOrCreateLRC finds an existing LRC for the given ingName+svcName in the list, or creates a new one and appends it.
func findOrCreateLRC(lrcs *[]gatewayv1beta1.ListenerRuleConfiguration, namespace, ingName, svcName string) *gatewayv1beta1.ListenerRuleConfiguration {
	expectedName := utils.GetLRConfigName(namespace, ingName, svcName)
	for i := range *lrcs {
		if (*lrcs)[i].Name == expectedName {
			return &(*lrcs)[i]
		}
	}
	*lrcs = append(*lrcs, *buildListenerRuleConfiguration(namespace, ingName, svcName))
	return &(*lrcs)[len(*lrcs)-1]
}

// routeRuleHasExtensionRef checks if a route rule already has an ExtensionRef filter pointing to the given LRC name.
func routeRuleHasExtensionRef(rule gwv1.HTTPRouteRule, lrcName string) bool {
	for _, f := range rule.Filters {
		if f.Type == gwv1.HTTPRouteFilterExtensionRef && f.ExtensionRef != nil && string(f.ExtensionRef.Name) == lrcName {
			return true
		}
	}
	return false
}
