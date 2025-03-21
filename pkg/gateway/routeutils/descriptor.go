package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type routeMetadataDescriptor interface {
	GetRouteNamespacedName() types.NamespacedName
	GetRouteKind() string
	GetHostnames() []gwv1.Hostname
	GetParentRefs() []gwv1.ParentReference
	GetRawRoute() interface{}
}

type preLoadRouteDescriptor interface {
	routeMetadataDescriptor
	loadAttachedRules(context context.Context, k8sClient client.Client) (RouteDescriptor, error)
}

type RouteDescriptor interface {
	routeMetadataDescriptor
	GetAttachedRules() []RouteRule
}
