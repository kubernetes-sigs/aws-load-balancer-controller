package gateway

/*
Common constants
*/

const (
	// GatewayResourceGroupVersion the groupVersion used by Gateway & GatewayClass resources.
	GatewayResourceGroupVersion = "gateway.networking.k8s.io/v1"
)

/*
NLB constants
*/

const (
	// nlbGatewayController gateway controller name for NLB
	nlbGatewayController = "gateway.k8s.aws/nlb"

	// nlbGatewayTagPrefix the tag applied to all resources created by the NLB Gateway controller.
	nlbGatewayTagPrefix = "gateway.k8s.aws.nlb"

	// nlbRouteResourceGroupVersion the groupVersion used by TCPRoute and UDPRoute
	nlbRouteResourceGroupVersion = "gateway.networking.k8s.io/v1alpha2"

	// nlbGatewayFinalizer the finalizer we attach the NLB Gateway object
	nlbGatewayFinalizer = "gateway.k8s.aws/nlb-finalizer"
)

/*
ALB Constants
*/

const (
	// albGatewayController gateway controller name for ALB
	albGatewayController = "gateway.k8s.aws/alb"

	// albGatewayTagPrefix the tag applied to all resources created by the ALB Gateway controller.
	albGatewayTagPrefix = "gateway.k8s.aws.nlb"

	// albRouteResourceGroupVersion the groupVersion used by HTTPRoute and GRPCRoute
	albRouteResourceGroupVersion = "gateway.networking.k8s.io/v1"

	// albGatewayFinalizer the finalizer we attach the ALB Gateway object
	albGatewayFinalizer = "gateway.k8s.aws/alb-finalizer"
)
