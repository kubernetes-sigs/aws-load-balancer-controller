package k8s

const (
	// Ingress events
	IngressEventReasonConflictingIngressClass = "ConflictingIngressClass"
	IngressEventReasonFailedLoadGroupID       = "FailedLoadGroupID"
	IngressEventReasonFailedAddFinalizer      = "FailedAddFinalizer"
	IngressEventReasonFailedRemoveFinalizer   = "FailedRemoveFinalizer"
	IngressEventReasonFailedUpdateStatus      = "FailedUpdateStatus"
	IngressEventReasonFailedBuildModel        = "FailedBuildModel"
	IngressEventReasonFailedDeployModel       = "FailedDeployModel"
	IngressEventReasonSuccessfullyReconciled  = "SuccessfullyReconciled"

	// Service events
	ServiceEventReasonFailedAddFinalizer     = "FailedAddFinalizer"
	ServiceEventReasonFailedRemoveFinalizer  = "FailedRemoveFinalizer"
	ServiceEventReasonFailedUpdateStatus     = "FailedUpdateStatus"
	ServiceEventReasonFailedCleanupStatus    = "FailedCleanupStatus"
	ServiceEventReasonFailedBuildModel       = "FailedBuildModel"
	ServiceEventReasonFailedDeployModel      = "FailedDeployModel"
	ServiceEventReasonSuccessfullyReconciled = "SuccessfullyReconciled"

	// TargetGroupBinding events
	TargetGroupBindingEventReasonFailedAddFinalizer     = "FailedAddFinalizer"
	TargetGroupBindingEventReasonFailedRemoveFinalizer  = "FailedRemoveFinalizer"
	TargetGroupBindingEventReasonFailedUpdateStatus     = "FailedUpdateStatus"
	TargetGroupBindingEventReasonFailedCleanup          = "FailedCleanup"
	TargetGroupBindingEventReasonFailedNetworkReconcile = "FailedNetworkReconcile"
	TargetGroupBindingEventReasonBackendNotFound        = "BackendNotFound"
	TargetGroupBindingEventReasonSuccessfullyReconciled = "SuccessfullyReconciled"

	// Gateway events
	GatewayEventReasonFailedAddFinalizer             = "FailedAddFinalizer"
	GatewayEventReasonFailedRemoveFinalizer          = "FailedRemoveFinalizer"
	GatewayEventReasonFailedDeleteWithRoutesAttached = "FailedDeleteRoutesAttached"
	GatewayEventReasonFailedUpdateStatus             = "FailedUpdateStatus"
	GatewayEventReasonSuccessfullyReconciled         = "SuccessfullyReconciled"
	GatewayEventReasonFailedDeployModel              = "FailedDeployModel"
	GatewayEventReasonFailedBuildModel               = "FailedBuildModel"

	// Target Group Configuration events
	TargetGroupConfigurationEventReasonFailedAddFinalizer    = "FailedAddFinalizer"
	TargetGroupConfigurationEventReasonFailedRemoveFinalizer = "FailedRemoveFinalizer"

	// Load Balancer Configuration events
	LoadBalancerConfigurationEventReasonFailedAddFinalizer    = "FailedAddFinalizer"
	LoadBalancerConfigurationEventReasonFailedRemoveFinalizer = "FailedRemoveFinalizer"

	// GlobalAccelerator events
	GlobalAcceleratorEventReasonFailedAddFinalizer     = "FailedAddFinalizer"
	GlobalAcceleratorEventReasonFailedRemoveFinalizer  = "FailedRemoveFinalizer"
	GlobalAcceleratorEventReasonFailedUpdateStatus     = "FailedUpdateStatus"
	GlobalAcceleratorEventReasonFailedCleanup          = "FailedCleanup"
	GlobalAcceleratorEventReasonFailedBuildModel       = "FailedBuildModel"
	GlobalAcceleratorEventReasonFailedDeploy           = "FailedDeploy"
	GlobalAcceleratorEventReasonSuccessfullyReconciled = "SuccessfullyReconciled"
)
