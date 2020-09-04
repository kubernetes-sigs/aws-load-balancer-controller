package runtime

import (
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

// HandleReconcileError will handle errors from reconcile handlers, which respects runtime errors.
func HandleReconcileError(err error, log logr.Logger) (ctrl.Result, error) {
	if err == nil {
		return ctrl.Result{}, nil
	}

	var requeueAfterErr *RequeueAfterError
	if errors.As(err, &requeueAfterErr) {
		log.V(1).Info("requeue after due to error", "duration", requeueAfterErr.Duration(), "error", requeueAfterErr.Unwrap())
		return ctrl.Result{RequeueAfter: requeueAfterErr.Duration()}, nil
	}

	var requeueError *RequeueError
	if errors.As(err, &requeueError) {
		log.V(1).Info("requeue due to error", "error", requeueError.Unwrap())
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, err
}
