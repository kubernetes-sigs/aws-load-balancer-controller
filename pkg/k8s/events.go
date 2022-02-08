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
	TargetGroupBindingEventReasonBackendNotFound        = "BackendNotFound"
	TargetGroupBindingEventReasonSuccessfullyReconciled = "SuccessfullyReconciled"
)
