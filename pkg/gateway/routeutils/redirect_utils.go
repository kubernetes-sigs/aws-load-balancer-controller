package routeutils

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// IsRedirectOnlyRule checks if an HTTPRoute rule contains only RequestRedirect filters
// and has no BackendRefs, making it a redirect-only rule that should not create target groups.
//
// Redirect-only rules are an optimization for simple HTTP redirects that don't require
// backend services. They create ALB redirect actions without consuming target group quotas.
//
// Returns true if:
// - The rule is an HTTPRoute rule
// - The rule has at least one RequestRedirect filter
// - The rule has no backends (BackendRefs)
//
// Returns false for:
// - Non-HTTPRoute rules (TCP, UDP, TLS, GRPC)
// - Rules with backends (regardless of redirect filters)
// - Rules without redirect filters
func IsRedirectOnlyRule(rule RouteRule) bool {
	// Check if the rule has any backends
	if len(rule.GetBackends()) > 0 {
		return false
	}

	// Check if this is an HTTPRoute rule with redirect filters
	rawRule := rule.GetRawRouteRule()
	httpRule, ok := rawRule.(*gwv1.HTTPRouteRule)
	if !ok {
		return false
	}

	// Check if the rule has RequestRedirect filters
	hasRedirectFilter := false
	for _, filter := range httpRule.Filters {
		if filter.Type == gwv1.HTTPRouteFilterRequestRedirect {
			hasRedirectFilter = true
			break
		}
	}

	return hasRedirectFilter
}

// HasRequestRedirectFilter checks if an HTTPRoute rule contains RequestRedirect filters.
//
// This function is used to identify rules that have redirect behavior, regardless of
// whether they also have backends. It's useful for:
// - Determining if redirect actions need to be built
// - Validating rule configurations
// - Logging and debugging redirect behavior
//
// Returns true if the rule is an HTTPRoute rule with at least one RequestRedirect filter.
func HasRequestRedirectFilter(rule RouteRule) bool {
	rawRule := rule.GetRawRouteRule()
	httpRule, ok := rawRule.(*gwv1.HTTPRouteRule)
	if !ok {
		return false
	}

	for _, filter := range httpRule.Filters {
		if filter.Type == gwv1.HTTPRouteFilterRequestRedirect {
			return true
		}
	}

	return false
}