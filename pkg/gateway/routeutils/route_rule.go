package routeutils

import (
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// RouteRule is a type agnostic representation of Routing Rules.
type RouteRule interface {
	GetRawRouteRule() interface{}
	GetSectionName() *gwv1.SectionName
	GetBackends() []Backend
	GetListenerRuleConfig() *elbv2gw.ListenerRuleConfiguration
}
