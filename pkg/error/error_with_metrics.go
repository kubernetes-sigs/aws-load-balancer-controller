package ctrlerrors

import (
	"github.com/pkg/errors"
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
