package runtime

import (
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	ctrlerrors "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/error"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/metrics/lbc"
	ctrl "sigs.k8s.io/controller-runtime"
)

// HandleReconcileError will handle errors from reconcile handlers, which respects runtime errors.
func HandleReconcileError(inputErr error, log logr.Logger) (ctrl.Result, error) {
	resolvedErr := handleNestedError(inputErr)
	if resolvedErr == nil {
		return ctrl.Result{}, nil
	}

	var requeueNeededAfter *ctrlerrors.RequeueNeededAfter
	if errors.As(resolvedErr, &requeueNeededAfter) {
		log.V(1).Info("requeue after", "duration", requeueNeededAfter.Duration(), "reason", requeueNeededAfter.Reason())
		return ctrl.Result{RequeueAfter: requeueNeededAfter.Duration()}, nil
	}

	var requeueNeeded *ctrlerrors.RequeueNeeded
	if errors.As(resolvedErr, &requeueNeeded) {
		log.V(1).Info("requeue", "reason", requeueNeeded.Reason())
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, inputErr
}

// HandleReconcileErrorWithCondition records the reconcile condition of the reconciled resource as
// failing when inputErr is a genuine failure (requeues are expected retries, not failures), then
// handles the error like HandleReconcileError.
func HandleReconcileErrorWithCondition(inputErr error, controller string, req ctrl.Request, metricsCollector lbcmetrics.MetricCollector, log logr.Logger) (ctrl.Result, error) {
	if inputErr != nil && !IsRequeueError(inputErr) {
		metricsCollector.ObserveControllerReconcileCondition(controller, req.Namespace, req.Name, false)
	}
	return HandleReconcileError(inputErr, log)
}

// IsRequeueError returns true when err indicates a requeue (expected retry).
func IsRequeueError(err error) bool {
	var requeueNeededAfter *ctrlerrors.RequeueNeededAfter
	if errors.As(err, &requeueNeededAfter) {
		return true
	}
	var requeueNeeded *ctrlerrors.RequeueNeeded
	if errors.As(err, &requeueNeeded) {
		return true
	}
	return false
}

func handleNestedError(err error) error {
	if err == nil {
		return nil
	}

	var wrappedError *ctrlerrors.ErrorWithMetrics
	if errors.As(err, &wrappedError) {
		return wrappedError.Err
	}

	return err
}
