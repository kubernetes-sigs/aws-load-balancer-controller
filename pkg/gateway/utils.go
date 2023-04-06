package gateway

import (
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

// GatewayUtils to check if the gateway is supported by the controller
type GatewayUtils interface {
	// IsGatewaySupported returns true if the gateway is supported by the controller
	IsGatewaySupported(gateway *v1beta1.Gateway) bool

	// IsGatewayPendingFinalization returns true if the gateway contains the aws-load-balancer-controller finalizer
	IsGatewayPendingFinalization(gateway *v1beta1.Gateway) bool
}

func NewGatewayUtils(annotationsParser annotations.Parser, gatewayFinalizer string, loadBalancerClass string,
	featureGates config.FeatureGates) *defaultGatewayUtils {
	return &defaultGatewayUtils{
		annotationParser:  annotationsParser,
		gatewayFinalizer:  gatewayFinalizer,
		loadBalancerClass: loadBalancerClass,
		featureGates:      featureGates,
	}
}

var _ GatewayUtils = (*defaultGatewayUtils)(nil)

type defaultGatewayUtils struct {
	annotationParser  annotations.Parser
	gatewayFinalizer  string
	loadBalancerClass string
	featureGates      config.FeatureGates
}

// IsGatewayPendingFinalization returns true if gateway has the aws-load-balancer-controller finalizer
func (u *defaultGatewayUtils) IsGatewayPendingFinalization(gateway *v1beta1.Gateway) bool {
	if k8s.HasFinalizer(gateway, u.gatewayFinalizer) {
		return true
	}
	return false
}

// IsGatewaySupported returns true if the gateway is supported by the controller
func (u *defaultGatewayUtils) IsGatewaySupported(gateway *v1beta1.Gateway) bool {
	if !gateway.DeletionTimestamp.IsZero() {
		return false
	}
	return u.checkAWSLoadBalancerTypeAnnotation(gateway)
}

func (u *defaultGatewayUtils) checkAWSLoadBalancerTypeAnnotation(gateway *v1beta1.Gateway) bool {
	lbType := ""

	_ = u.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixLoadBalancerType, &lbType, gateway.Annotations)
	if lbType == LoadBalancerTypeNLBIP {
		return true
	}
	var lbTargetType string
	_ = u.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixTargetType, &lbTargetType, gateway.Annotations)
	if lbType == LoadBalancerTypeExternal && (lbTargetType == LoadBalancerTargetTypeIP ||
		lbTargetType == LoadBalancerTargetTypeInstance) {
		return true
	}
	return false
}
