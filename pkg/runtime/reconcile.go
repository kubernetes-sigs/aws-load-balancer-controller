package runtime

import (
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	errmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/error"
	ctrl "sigs.k8s.io/controller-runtime"
)

// HandleReconcileError will handle errors from reconcile handlers, which respects runtime errors.
func HandleReconcileError(inputErr error, log logr.Logger) (ctrl.Result, error) {
	resolvedErr := handleNestedError(inputErr)
	if resolvedErr == nil {
		return ctrl.Result{}, nil
	}

	var requeueNeededAfter *RequeueNeededAfter
	if errors.As(resolvedErr, &requeueNeededAfter) {
		log.V(1).Info("requeue after", "duration", requeueNeededAfter.Duration(), "reason", requeueNeededAfter.Reason())
		return ctrl.Result{RequeueAfter: requeueNeededAfter.Duration()}, nil
	}

	var requeueNeeded *RequeueNeeded
	if errors.As(resolvedErr, &requeueNeeded) {
		log.V(1).Info("requeue", "reason", requeueNeeded.Reason())
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, inputErr
}

func handleNestedError(err error) error {
	if err == nil {
		return nil
	}

	var wrappedError *errmetrics.ErrorWithMetrics
	if errors.As(err, &wrappedError) {
		return wrappedError.Err
	}

	return err
}
