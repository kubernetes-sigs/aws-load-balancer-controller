package constants

/*
Common constants
*/

const (
	// GatewayResourceGroupVersion the groupVersion used by Gateway & GatewayClass resources.
	GatewayResourceGroupVersion = "gateway.networking.k8s.io/v1"

	// LoadBalancerConfiguration the CRD name of LoadBalancerConfiguration
	LoadBalancerConfiguration = "LoadBalancerConfiguration"
)

/*
   NLB Gateway constants
*/

const (
	// NLBGatewayController gateway controller name for NLB
	NLBGatewayController = "gateway.k8s.aws/nlb"

	// NLBGatewayTagPrefix the tag applied to all resources created by the NLB Gateway controller.
	NLBGatewayTagPrefix = "gateway.k8s.aws.nlb"

	// NLBRouteResourceGroupVersion the groupVersion used by TCPRoute and UDPRoute
	NLBRouteResourceGroupVersion = "gateway.networking.k8s.io/v1alpha2"
)

/*
   ALB Gateway Constants
*/

const (
	// ALBGatewayController gateway controller name for ALB
	ALBGatewayController = "gateway.k8s.aws/alb"

	// ALBGatewayTagPrefix the tag applied to all resources created by the ALB Gateway controller.
	ALBGatewayTagPrefix = "gateway.k8s.aws.alb"

	// ALBRouteResourceGroupVersion the groupVersion used by HTTPRoute and GRPCRoute
	ALBRouteResourceGroupVersion = "gateway.networking.k8s.io/v1"
)
