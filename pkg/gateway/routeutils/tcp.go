package routeutils

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type tcpRouteDescription struct {
	route *gwalpha2.TCPRoute
}

func (t *tcpRouteDescription) GetRouteKind() string {
	return TCPRouteKind
}

func (t *tcpRouteDescription) GetRouteNamespace() string {
	return t.route.Namespace
}

func (t *tcpRouteDescription) GetRouteName() string {
	return t.route.Name
}

func convertTCPRoute(r gwalpha2.TCPRoute) *tcpRouteDescription {
	return &tcpRouteDescription{route: &r}
}

func (t *tcpRouteDescription) GetAttachedRules() []BackendDescription {
	//TODO implement me
	panic("implement me")
}

func (t *tcpRouteDescription) GetRawRoute() interface{} {
	return t.route
}

var _ RouteDescriptor = &tcpRouteDescription{}

// Can we use an indexer here to query more efficiently?

func ListTCPRoutes(context context.Context, client client.Client) ([]RouteDescriptor, error) {
	routeList := &gwalpha2.TCPRouteList{}
	err := client.List(context, routeList)
	if err != nil {
		return nil, err
	}

	result := make([]RouteDescriptor, 0)

	for _, item := range routeList.Items {
		result = append(result, convertTCPRoute(item))
	}

	return result, err
}
