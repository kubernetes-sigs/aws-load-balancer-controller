package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"time"
)

/*
This class holds the representation of an HTTP route.
Generally, outside consumers will use GetRawRouteRule to inspect the
HTTP specific features of the route.
*/

/* Route Rule */

var _ RouteRule = &convertedHTTPRouteRule{}

var defaultHTTPRuleAccumulator = newAttachedRuleAccumulator[gwv1.HTTPRouteRule](commonBackendLoader)

type convertedHTTPRouteRule struct {
	rule     *gwv1.HTTPRouteRule
	backends []Backend
}

func convertHTTPRouteRule(rule *gwv1.HTTPRouteRule, backends []Backend) RouteRule {
	return &convertedHTTPRouteRule{
		rule:     rule,
		backends: backends,
	}
}

func (t *convertedHTTPRouteRule) GetRawRouteRule() interface{} {
	return t.rule
}

func (t *convertedHTTPRouteRule) GetSectionName() *gwv1.SectionName {
	return t.rule.Name
}

func (t *convertedHTTPRouteRule) GetBackends() []Backend {
	return t.backends
}

/* Route Description */

type httpRouteDescription struct {
	route           *gwv1.HTTPRoute
	rules           []RouteRule
	ruleAccumulator attachedRuleAccumulator[gwv1.HTTPRouteRule]
}

func (httpRoute *httpRouteDescription) GetAttachedRules() []RouteRule {
	return httpRoute.rules
}

func (httpRoute *httpRouteDescription) loadAttachedRules(ctx context.Context, k8sClient client.Client) (RouteDescriptor, []routeLoadError) {
	convertedRules, allErrors := httpRoute.ruleAccumulator.accumulateRules(ctx, k8sClient, httpRoute, httpRoute.route.Spec.Rules, func(rule gwv1.HTTPRouteRule) []gwv1.BackendRef {
		refs := make([]gwv1.BackendRef, 0, len(rule.BackendRefs))
		for _, httpRef := range rule.BackendRefs {
			refs = append(refs, httpRef.BackendRef)
		}
		return refs
	}, func(hrr *gwv1.HTTPRouteRule, backends []Backend) RouteRule {
		return convertHTTPRouteRule(hrr, backends)
	})
	httpRoute.rules = convertedRules
	return httpRoute, allErrors
}

func (httpRoute *httpRouteDescription) GetHostnames() []gwv1.Hostname {
	return httpRoute.route.Spec.Hostnames
}

func (httpRoute *httpRouteDescription) GetParentRefs() []gwv1.ParentReference {
	return httpRoute.route.Spec.ParentRefs
}

func (httpRoute *httpRouteDescription) GetRouteKind() RouteKind {
	return HTTPRouteKind
}

func (httpRoute *httpRouteDescription) GetRouteGeneration() int64 {
	return httpRoute.route.Generation
}

func (httpRoute *httpRouteDescription) GetRouteNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(httpRoute.route)
}

func (httpRoute *httpRouteDescription) GetBackendRefs() []gwv1.BackendRef {
	backendRefs := make([]gwv1.BackendRef, 0)
	if httpRoute.route.Spec.Rules != nil {
		for _, rule := range httpRoute.route.Spec.Rules {
			for _, httpBackendRef := range rule.BackendRefs {
				backendRefs = append(backendRefs, httpBackendRef.BackendRef)
			}
		}
	}
	return backendRefs
}

func (httpRoute *httpRouteDescription) GetRouteCreateTimestamp() time.Time {
	return httpRoute.route.CreationTimestamp.Time
}

func convertHTTPRoute(r gwv1.HTTPRoute) *httpRouteDescription {
	return &httpRouteDescription{route: &r, ruleAccumulator: defaultHTTPRuleAccumulator}
}

func (httpRoute *httpRouteDescription) GetRawRoute() interface{} {
	return httpRoute.route
}

var _ RouteDescriptor = &httpRouteDescription{}

// Can we use an indexer here to query more efficiently?

func ListHTTPRoutes(context context.Context, k8sClient client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error) {
	routeList := &gwv1.HTTPRouteList{}
	err := k8sClient.List(context, routeList, opts...)
	if err != nil {
		return nil, err
	}

	result := make([]preLoadRouteDescriptor, 0)

	for _, item := range routeList.Items {
		result = append(result, convertHTTPRoute(item))
	}

	return result, nil
}
