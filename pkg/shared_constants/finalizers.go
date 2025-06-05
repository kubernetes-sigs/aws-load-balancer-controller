package shared_constants

const (
	// ExplicitGroupFinalizerPrefix the prefix for finalizers applied to an ingress group
	ExplicitGroupFinalizerPrefix = "group.ingress.k8s.aws/"

	// ImplicitGroupFinalizer the finalizer used on an ingress resource
	ImplicitGroupFinalizer = "ingress.k8s.aws/resources"

	// ServiceFinalizer the finalizer used on service resources
	ServiceFinalizer = "service.k8s.aws/resources"

	// NLBGatewayFinalizer the finalizer we attach to an NLB Gateway resource
	NLBGatewayFinalizer = "gateway.k8s.aws/nlb"

	// ALBGatewayFinalizer the finalizer we attach to an ALB Gateway resource
	ALBGatewayFinalizer = "gateway.k8s.aws/alb"

	// TargetGroupConfigurationFinalizer the finalizer we attach to a target group configuration resource
	TargetGroupConfigurationFinalizer = "gateway.k8s.aws/targetgroupconfigurations"

	// LoadBalancerConfigurationFinalizer the finalizer we attach to a load balancer configuration resource
	LoadBalancerConfigurationFinalizer = "gateway.k8s.aws/loadbalancerconfigurations"
)
