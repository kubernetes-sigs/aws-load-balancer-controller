package ctrlerrors

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/metrics/lbc"
)

type ErrorWithMetrics struct {
	ResourceType  string
	ErrorCategory string
	Err           error
}

func NewErrorWithMetrics(resourceType string, errorCategory string, err error, metricCollector lbcmetrics.MetricCollector) *ErrorWithMetrics {
	reconcileErr := &ErrorWithMetrics{
		ResourceType:  resourceType,
		ErrorCategory: errorCategory,
		Err:           err,
	}

	var skipErrorMetric bool
	var requeueNeededAfter *RequeueNeededAfter
	var requeueAfter *RequeueNeeded
	if errors.As(err, &requeueNeededAfter) || errors.As(err, &requeueAfter) {
		skipErrorMetric = true
	}
	// A create in a namespace that is being deleted is expected to fail, and the deletion resolves it.
	if apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
		skipErrorMetric = true
	}

	if !skipErrorMetric {
		metricCollector.ObserveControllerReconcileError(resourceType, errorCategory)
	}

	return reconcileErr
}

func (e *ErrorWithMetrics) Error() string {
	return e.Err.Error()
}

// Unwrap exposes the wrapped error so errors.Is/errors.As can classify it, e.g. requeue detection.
func (e *ErrorWithMetrics) Unwrap() error {
	return e.Err
}
