package config

import (
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/shared_constants"
)

// GatewayFinalizerConfig holds the finalizers the controller attaches to Gateway API related
// resources.
type GatewayFinalizerConfig struct {
	// GatewayClassFinalizer is attached to an in-use LBC GatewayClass.
	GatewayClassFinalizer string
	// NLBGatewayFinalizer is attached to an NLB Gateway resource.
	NLBGatewayFinalizer string
	// ALBGatewayFinalizer is attached to an ALB Gateway resource.
	ALBGatewayFinalizer string
	// TargetGroupConfigurationFinalizer is attached to a TargetGroupConfiguration resource.
	TargetGroupConfigurationFinalizer string
	// LoadBalancerConfigurationFinalizer is attached to a LoadBalancerConfiguration resource.
	LoadBalancerConfigurationFinalizer string
	// ListenerRuleConfigurationFinalizer is attached to a ListenerRuleConfiguration resource.
	ListenerRuleConfigurationFinalizer string
}

// NewDefaultGatewayFinalizerConfig returns a GatewayFinalizerConfig populated with the built-in
// finalizer values. Callers may override any field to customize the finalizers ad-hoc.
func NewDefaultGatewayFinalizerConfig() GatewayFinalizerConfig {
	return GatewayFinalizerConfig{
		GatewayClassFinalizer:              shared_constants.GatewayClassFinalizer,
		NLBGatewayFinalizer:                shared_constants.NLBGatewayFinalizer,
		ALBGatewayFinalizer:                shared_constants.ALBGatewayFinalizer,
		TargetGroupConfigurationFinalizer:  shared_constants.TargetGroupConfigurationFinalizer,
		LoadBalancerConfigurationFinalizer: shared_constants.LoadBalancerConfigurationFinalizer,
		ListenerRuleConfigurationFinalizer: shared_constants.ListenerRuleConfigurationFinalizer,
	}
}
