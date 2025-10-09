package constants

import "k8s.io/apimachinery/pkg/util/sets"

/*
Common constants
*/

var (
	FullGatewayControllerSet = sets.New(ALBGatewayController, NLBGatewayController)
)

const (
	// GatewayResourceGroupName the groupName used by Gateway & GatewayClass resources.
	GatewayResourceGroupName = "gateway.networking.k8s.io"

	// GatewayResourceGroupVersion the groupVersion used by Gateway & GatewayClass resources.
	GatewayResourceGroupVersion = "gateway.networking.k8s.io/v1"

	//ControllerCRDGroupVersion the groupVersion used by customization CRDs
	ControllerCRDGroupVersion = "gateway.k8s.aws"

	// LoadBalancerConfiguration the CRD name of LoadBalancerConfiguration
	LoadBalancerConfiguration = "LoadBalancerConfiguration"

	// ListenerRuleConfiguration the CRD name of ListenerRuleConfiguration
	ListenerRuleConfiguration = "ListenerRuleConfiguration"
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

const (
	// GatewayClassController the controller that reconciles gateway class changes
	GatewayClassController = "aws-lbc-gateway-class-controller"

	//LoadBalancerConfigurationController the controller that reconciles LoadBalancerConfiguration changes
	LoadBalancerConfigurationController = "aws-lbc-loadbalancerconfiguration-controller"

	//TargetGroupConfigurationController the controller that reconciles TargetGroupConfiguration changes
	TargetGroupConfigurationController = "aws-lbc-targetgroupconfiguration-controller"

	//ListenerRuleConfigurationController the controller that reconciles ListenerRuleConfiguration changes
	ListenerRuleConfigurationController = "aws-lbc-listenerruleconfiguration-controller"

	// GatewayLBPrefixEnabledAddon The prefix tracking addons within the Gateway.
	GatewayLBPrefixEnabledAddon = "gateway.k8s.aws.addon."
)

// constant for status update
const (
	GatewayAcceptedFalseMessage = "Gateway is not accepted because there is an invalid listener."
	ListenerAcceptedMessage     = "Listener is accepted"
)
