package errmetrics

import (
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
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
	metricCollector.ObserveControllerReconcileError(resourceType, errorCategory)
	return reconcileErr
}

func (e *ErrorWithMetrics) Error() string {
	return e.Err.Error()
}
