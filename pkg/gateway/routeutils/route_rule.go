package routeutils

import (
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1"
)

// RouteRule is a type agnostic representation of Routing Rules.
type RouteRule interface {
	GetRawRouteRule() interface{}
	GetBackends() []Backend
	GetListenerRuleConfig() *elbv2gw.ListenerRuleConfiguration
}
