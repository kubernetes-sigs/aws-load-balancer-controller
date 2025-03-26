package routeutils

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type tlsRouteDescription struct {
	route *gwalpha2.TLSRoute
}

func (t *tlsRouteDescription) GetRouteKind() string {
	return TLSRouteKind
}

func convertTLSRoute(r gwalpha2.TLSRoute) *tlsRouteDescription {
	return &tlsRouteDescription{route: &r}
}

func (t *tlsRouteDescription) GetRouteNamespace() string {
	return t.route.Namespace
}

func (t *tlsRouteDescription) GetRouteName() string {
	return t.route.Name
}

func (t *tlsRouteDescription) GetAttachedRules() []BackendDescription {
	//TODO implement me
	panic("implement me")
}

func (t *tlsRouteDescription) GetRawRoute() interface{} {
	return t.route
}

var _ RouteDescriptor = &tlsRouteDescription{}

func ListTLSRoutes(context context.Context, client client.Client) ([]RouteDescriptor, error) {
	routeList := &gwalpha2.TLSRouteList{}
	err := client.List(context, routeList)
	if err != nil {
		return nil, err
	}

	result := make([]RouteDescriptor, 0)

	for _, item := range routeList.Items {
		result = append(result, convertTLSRoute(item))
	}

	return result, err
}
