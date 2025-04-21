package routeutils

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// routeMetadataDescriptor a common set of functions that will describe a route.
// These are intentionally meant to be type agnostic;
// however, consumers can use `GetRawRoute()` to inspect the actual route fields if needed.
type routeMetadataDescriptor interface {
	GetRouteNamespacedName() types.NamespacedName
	GetRouteKind() RouteKind
	GetHostnames() []gwv1.Hostname
	GetParentRefs() []gwv1.ParentReference
	GetRawRoute() interface{}
	GetBackendRefs() []gwv1.BackendRef
}

// preLoadRouteDescriptor this object is used to represent a route description that has not loaded its child data (services, tg config)
// generally use this interface to represent broad data, filter that data down to the absolutely required data, and the call
// loadAttachedRules() to generate a full route description.
type preLoadRouteDescriptor interface {
	routeMetadataDescriptor
	loadAttachedRules(context context.Context, k8sClient client.Client) (RouteDescriptor, error)
}

// RouteDescriptor is a type agnostic representation of a Gateway Route.
// This interface holds all data necessary to construct
// an ELBv2 object out of Kubernetes objects.
type RouteDescriptor interface {
	routeMetadataDescriptor
	GetAttachedRules() []RouteRule
}
