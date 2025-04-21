package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

/*
This class holds the representation of an HTTP route.
Generally, outside consumers will use GetRawRouteRule to inspect the
HTTP specific features of the route.
*/

/* Route Rule */

var _ RouteRule = &convertedHTTPRouteRule{}

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
	route         *gwv1.HTTPRoute
	rules         []RouteRule
	backendLoader func(ctx context.Context, k8sClient client.Client, typeSpecificBackend interface{}, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind string) (*Backend, error)
}

func (httpRoute *httpRouteDescription) GetAttachedRules() []RouteRule {
	return httpRoute.rules
}

func (httpRoute *httpRouteDescription) loadAttachedRules(ctx context.Context, k8sClient client.Client) (RouteDescriptor, error) {
	convertedRules := make([]RouteRule, 0)
	for _, rule := range httpRoute.route.Spec.Rules {
		convertedBackends := make([]Backend, 0)
		for _, backend := range rule.BackendRefs {
			convertedBackend, err := httpRoute.backendLoader(ctx, k8sClient, backend, backend.BackendRef, httpRoute.GetRouteNamespacedName(), httpRoute.GetRouteKind())
			if err != nil {
				return nil, err
			}

			if convertedBackend != nil {
				convertedBackends = append(convertedBackends, *convertedBackend)
			}
		}

		convertedRules = append(convertedRules, convertHTTPRouteRule(&rule, convertedBackends))
	}

	httpRoute.rules = convertedRules
	return httpRoute, nil
}

func (httpRoute *httpRouteDescription) GetHostnames() []gwv1.Hostname {
	return httpRoute.route.Spec.Hostnames
}

func (httpRoute *httpRouteDescription) GetParentRefs() []gwv1.ParentReference {
	return httpRoute.route.Spec.ParentRefs
}

func (httpRoute *httpRouteDescription) GetRouteKind() string {
	return HTTPRouteKind
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

func convertHTTPRoute(r gwv1.HTTPRoute) *httpRouteDescription {
	return &httpRouteDescription{route: &r, backendLoader: commonBackendLoader}
}

func (httpRoute *httpRouteDescription) GetRawRoute() interface{} {
	return httpRoute.route
}

var _ RouteDescriptor = &httpRouteDescription{}

// Can we use an indexer here to query more efficiently?

func ListHTTPRoutes(context context.Context, k8sClient client.Client) ([]preLoadRouteDescriptor, error) {
	routeList := &gwv1.HTTPRouteList{}
	err := k8sClient.List(context, routeList)
	if err != nil {
		return nil, err
	}

	result := make([]preLoadRouteDescriptor, 0)

	for _, item := range routeList.Items {
		result = append(result, convertHTTPRoute(item))
	}

	return result, err
}
