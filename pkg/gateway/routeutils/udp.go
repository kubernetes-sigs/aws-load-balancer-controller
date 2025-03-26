package routeutils

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type udpRouteDescription struct {
	route *gwalpha2.UDPRoute
}

func (t *udpRouteDescription) GetRouteKind() string {
	return UDPRouteKind
}

func convertUDPRoute(r gwalpha2.UDPRoute) *udpRouteDescription {
	return &udpRouteDescription{route: &r}
}

func (t *udpRouteDescription) GetRouteNamespace() string {
	return t.route.Namespace
}

func (t *udpRouteDescription) GetRouteName() string {
	return t.route.Name
}

func (t *udpRouteDescription) GetAttachedRules() []BackendDescription {
	//TODO implement me
	panic("implement me")
}

func (t *udpRouteDescription) GetRawRoute() interface{} {
	return t.route
}

var _ RouteDescriptor = &udpRouteDescription{}

func ListUDPRoutes(context context.Context, client client.Client) ([]RouteDescriptor, error) {
	routeList := &gwalpha2.UDPRouteList{}
	err := client.List(context, routeList)
	if err != nil {
		return nil, err
	}

	result := make([]RouteDescriptor, 0)

	for _, item := range routeList.Items {
		result = append(result, convertUDPRoute(item))
	}

	return result, err
}
