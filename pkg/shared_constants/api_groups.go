package shared_constants

const (
	// GlobalAcceleratorResourcesGroup is the API group for GlobalAccelerator resources
	GlobalAcceleratorResourcesGroup = "aga.k8s.aws"

	// GlobalAcceleratorResourcesGroupVersion is the complete API group/version for GlobalAccelerator resources
	GlobalAcceleratorResourcesGroupVersion = "aga.k8s.aws/v1beta1"

	// GlobalAcceleratorKind is the resource kind for GlobalAccelerator
	GlobalAcceleratorKind = "GlobalAccelerator"

	// GatewayAPIResourcesGroup is the API group for Gateway API resources
	GatewayAPIResourcesGroup = "gateway.networking.k8s.io"

	// GatewayApiKind is the resource kind for Gateway API Gateway resources
	GatewayApiKind = "Gateway"

	// HTTPRouteKind is the resource kind for HTTPRoute resources
	HTTPRouteKind = "HTTPRoute"

	// CoreAPIGroup represents the core API group (empty string)
	CoreAPIGroup = ""

	// ServiceKind is the resource kind for Kubernetes Service resources
	ServiceKind = "Service"

	// IngressAPIGroup is the API group for Kubernetes Ingress resources
	IngressAPIGroup = "networking.k8s.io"

	// IngressKind is the resource kind for Kubernetes Ingress resources
	IngressKind = "Ingress"
)
