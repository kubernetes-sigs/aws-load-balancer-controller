package routeutils

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// TCPRoute and UDPRoute graduated to v1 in Gateway API 1.6. The v1alpha2
// variants remain structurally identical to v1 (their rule and status types
// are direct type conversions of the v1 types), so conversion is mechanical.
// These helpers let the controller keep a single internal (v1) representation
// while still supporting clusters that only serve v1alpha2.

// ConvertAlpha2TCPRouteToV1 converts a v1alpha2 TCPRoute to its v1 representation.
func ConvertAlpha2TCPRouteToV1(route *gwalpha2.TCPRoute) *gwv1.TCPRoute {
	if route == nil {
		return nil
	}
	rules := make([]gwv1.TCPRouteRule, 0, len(route.Spec.Rules))
	for _, rule := range route.Spec.Rules {
		rules = append(rules, gwv1.TCPRouteRule(rule))
	}
	return &gwv1.TCPRoute{
		TypeMeta:   route.TypeMeta,
		ObjectMeta: route.ObjectMeta,
		Spec: gwv1.TCPRouteSpec{
			CommonRouteSpec: route.Spec.CommonRouteSpec,
			Rules:           rules,
		},
		Status: gwv1.TCPRouteStatus(route.Status),
	}
}

// ConvertAlpha2UDPRouteToV1 converts a v1alpha2 UDPRoute to its v1 representation.
func ConvertAlpha2UDPRouteToV1(route *gwalpha2.UDPRoute) *gwv1.UDPRoute {
	if route == nil {
		return nil
	}
	rules := make([]gwv1.UDPRouteRule, 0, len(route.Spec.Rules))
	for _, rule := range route.Spec.Rules {
		rules = append(rules, gwv1.UDPRouteRule(rule))
	}
	return &gwv1.UDPRoute{
		TypeMeta:   route.TypeMeta,
		ObjectMeta: route.ObjectMeta,
		Spec: gwv1.UDPRouteSpec{
			CommonRouteSpec: route.Spec.CommonRouteSpec,
			Rules:           rules,
		},
		Status: gwv1.UDPRouteStatus(route.Status),
	}
}
