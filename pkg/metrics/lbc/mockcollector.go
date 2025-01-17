package lbc

import (
	"context"
	"time"
)

type MockCollector struct {
	Invocations map[string][]interface{}
}

type MockHistogramMetric struct {
	namespace string
	name      string
	duration  time.Duration
}

// ObservePodReadinessGateReady mocks observing the readiness gate latency.
func (m *MockCollector) ObservePodReadinessGateReady(namespace string, tgbName string, d time.Duration) {
	m.recordHistogram(MetricPodReadinessGateReady, namespace, tgbName, d)
}

// UpdateManagedK8sResourceMetrics mocks updating managed Kubernetes resource metrics.
func (m *MockCollector) UpdateManagedK8sResourceMetrics(ctx context.Context) error {
	m.recordInvocation("UpdateManagedK8sResourceMetrics", ctx)
	return nil // No-op for the mock
}

// UpdateManagedALBMetrics mocks updating managed ALB resource metrics.
func (m *MockCollector) UpdateManagedALBMetrics(ctx context.Context) error {
	m.recordInvocation("UpdateManagedALBMetrics", ctx)
	return nil // No-op for the mock
}

// UpdateManagedNLBMetrics mocks updating managed ALB resource metrics.
func (m *MockCollector) UpdateManagedNLBMetrics(ctx context.Context) error {
	m.recordInvocation("UpdateManagedALBMetrics", ctx)
	return nil // No-op for the mock
}

// recordHistogram adds a histogram metric invocation.
func (m *MockCollector) recordHistogram(metricName string, namespace string, name string, d time.Duration) {
	if _, exists := m.Invocations[metricName]; !exists {
		m.Invocations[metricName] = []interface{}{}
	}
	m.Invocations[metricName] = append(m.Invocations[metricName], MockHistogramMetric{
		namespace: namespace,
		name:      name,
		duration:  d,
	})
}

// recordInvocation tracks a method invocation with arguments.
func (m *MockCollector) recordInvocation(methodName string, args ...interface{}) {
	if _, exists := m.Invocations[methodName]; !exists {
		m.Invocations[methodName] = []interface{}{}
	}
	m.Invocations[methodName] = append(m.Invocations[methodName], args)
}

// NewMockCollector creates and returns a new MockCollector.
func NewMockCollector() MetricCollector {
	mockInvocations := make(map[string][]interface{})
	mockInvocations[MetricPodReadinessGateReady] = []interface{}{}

	return &MockCollector{
		Invocations: mockInvocations,
	}
}
