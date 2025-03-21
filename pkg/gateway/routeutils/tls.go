package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

/* Route Rule */

var _ RouteRule = &convertedTLSRouteRule{}

type convertedTLSRouteRule struct {
	rule     *gwalpha2.TLSRouteRule
	backends []Backend
}

func convertTLSRouteRule(rule *gwalpha2.TLSRouteRule, backends []Backend) RouteRule {
	return &convertedTLSRouteRule{
		rule:     rule,
		backends: backends,
	}
}

func (t *convertedTLSRouteRule) GetRawRouteRule() interface{} {
	return t.rule
}

func (t *convertedTLSRouteRule) GetSectionName() *gwv1.SectionName {
	return t.rule.Name
}

func (t *convertedTLSRouteRule) GetBackends() []Backend {
	return t.backends
}

/* Route Description */

type tlsRouteDescription struct {
	route         *gwalpha2.TLSRoute
	rules         []RouteRule
	backendLoader func(ctx context.Context, k8sClient client.Client, typeSpecificBackend interface{}, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind string) (*Backend, error)
}

func (tlsRoute *tlsRouteDescription) GetAttachedRules() []RouteRule {
	return tlsRoute.rules
}

func (tlsRoute *tlsRouteDescription) loadAttachedRules(ctx context.Context, k8sClient client.Client) (RouteDescriptor, error) {
	convertedRules := make([]RouteRule, 0)
	for _, rule := range tlsRoute.route.Spec.Rules {
		convertedBackends := make([]Backend, 0)

		for _, backend := range rule.BackendRefs {
			convertedBackend, err := tlsRoute.backendLoader(ctx, k8sClient, backend, backend, tlsRoute.GetRouteNamespacedName(), tlsRoute.GetRouteKind())
			if err != nil {
				return nil, err
			}

			if convertedBackend != nil {
				convertedBackends = append(convertedBackends, *convertedBackend)
			}
		}

		convertedRules = append(convertedRules, convertTLSRouteRule(&rule, convertedBackends))
	}

	tlsRoute.rules = convertedRules
	return tlsRoute, nil
}

func (tlsRoute *tlsRouteDescription) GetHostnames() []gwv1.Hostname {
	return tlsRoute.route.Spec.Hostnames
}

func (tlsRoute *tlsRouteDescription) GetParentRefs() []gwv1.ParentReference {
	return tlsRoute.route.Spec.ParentRefs
}

func (tlsRoute *tlsRouteDescription) GetRouteKind() string {
	return TLSRouteKind
}

func convertTLSRoute(r gwalpha2.TLSRoute) *tlsRouteDescription {
	return &tlsRouteDescription{route: &r, backendLoader: commonBackendLoader}
}

func (tlsRoute *tlsRouteDescription) GetRouteNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(tlsRoute.route)
}

func (tlsRoute *tlsRouteDescription) GetRawRoute() interface{} {
	return tlsRoute.route
}

var _ RouteDescriptor = &tlsRouteDescription{}

func ListTLSRoutes(context context.Context, k8sClient client.Client) ([]preLoadRouteDescriptor, error) {
	routeList := &gwalpha2.TLSRouteList{}
	err := k8sClient.List(context, routeList)
	if err != nil {
		return nil, err
	}

	result := make([]preLoadRouteDescriptor, 0)

	for _, item := range routeList.Items {
		result = append(result, convertTLSRoute(item))
	}

	return result, err
}
