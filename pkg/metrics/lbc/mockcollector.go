package lbc

import (
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

func (m *MockCollector) ObservePodReadinessGateReady(namespace string, tgbName string, d time.Duration) {
	m.recordHistogram(MetricPodReadinessGateReady, namespace, tgbName, d)
}

func (m *MockCollector) recordHistogram(metricName string, namespace string, name string, d time.Duration) {
	m.Invocations[metricName] = append(m.Invocations[MetricPodReadinessGateReady], MockHistogramMetric{
		namespace: namespace,
		name:      name,
		duration:  d,
	})
}

func NewMockCollector() MetricCollector {

	mockInvocations := make(map[string][]interface{})
	mockInvocations[MetricPodReadinessGateReady] = make([]interface{}, 0)

	return &MockCollector{
		Invocations: mockInvocations,
	}
}
