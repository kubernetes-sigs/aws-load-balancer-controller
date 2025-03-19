package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type tcpRouteDescription struct {
	route *gwalpha2.TCPRoute
	rules []RouteRule
}

var _ RouteRule = &convertedTCPRouteRule{}

type convertedTCPRouteRule struct {
	rule     *gwalpha2.TCPRouteRule
	backends []Backend
}

func convertTCPRouteRule(rule *gwalpha2.TCPRouteRule, backends []Backend) RouteRule {
	return &convertedTCPRouteRule{
		rule:     rule,
		backends: backends,
	}
}

func (t *convertedTCPRouteRule) GetSectionName() *gwv1.SectionName {
	return t.rule.Name
}

func (t *convertedTCPRouteRule) GetBackends() []Backend {
	return t.backends
}

func (t *convertedTCPRouteRule) GetHostnames() []string {
	// Not supported for TCP route rules
	return []string{}
}

func (tcpRoute *tcpRouteDescription) GetAttachedRules() []RouteRule {
	return tcpRoute.rules
}

func (tcpRoute *tcpRouteDescription) loadAttachedRules(ctx context.Context, k8sClient client.Client) (RouteDescriptor, error) {

	convertedRules := make([]RouteRule, 0)
	for _, rule := range tcpRoute.route.Spec.Rules {
		convertedBackends := make([]Backend, 0)

		for _, backend := range rule.BackendRefs {
			convertedBackend, err := commonBackendLoader(ctx, k8sClient, backend, tcpRoute.GetRouteNamespacedName(), tcpRoute.GetRouteKind())
			if err != nil {
				return nil, err
			}
			convertedBackends = append(convertedBackends, *convertedBackend)
		}

		convertedRules = append(convertedRules, convertTCPRouteRule(&rule, convertedBackends))
	}

	tcpRoute.rules = convertedRules
	return tcpRoute, nil
}

func (tcpRoute *tcpRouteDescription) GetRouteKind() string {
	return TCPRouteKind
}

func (tcpRoute *tcpRouteDescription) GetRouteNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(tcpRoute.route)
}

func convertTCPRoute(r gwalpha2.TCPRoute) *tcpRouteDescription {
	return &tcpRouteDescription{route: &r}
}

func (tcpRoute *tcpRouteDescription) GetRawRoute() interface{} {
	return tcpRoute.route
}

func (tcpRoute *tcpRouteDescription) GetParentRefs() []gwv1.ParentReference {
	return tcpRoute.route.Spec.ParentRefs
}

var _ RouteDescriptor = &tcpRouteDescription{}

// Can we use an indexer here to query more efficiently?

func ListTCPRoutes(context context.Context, k8sClient client.Client) ([]preLoadRouteDescriptor, error) {
	routeList := &gwalpha2.TCPRouteList{}
	err := k8sClient.List(context, routeList)
	if err != nil {
		return nil, err
	}

	result := make([]preLoadRouteDescriptor, 0)

	for _, item := range routeList.Items {
		result = append(result, convertTCPRoute(item))
	}

	return result, err
}
