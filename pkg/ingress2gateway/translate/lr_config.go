package translate

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	gwconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// buildListenerRuleConfiguration creates a skeleton ListenerRuleConfiguration with metadata.
func buildListenerRuleConfiguration(namespace, svcName string) *gatewayv1beta1.ListenerRuleConfiguration {
	return &gatewayv1beta1.ListenerRuleConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: utils.LBConfigAPIVersion,
			Kind:       gwconstants.ListenerRuleConfiguration,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.GetLRConfigName(namespace, svcName),
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
