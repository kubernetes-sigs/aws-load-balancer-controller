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

type MockCounterMetric struct {
	labelController    string
	labelErrorCategory string
	labelNamespace     string
	labelName          string
	resource           string
	webhookName        string
	errorType          string
}

func (m *MockCollector) ObservePodReadinessGateReady(namespace string, tgbName string, d time.Duration) {
	m.recordHistogram(MetricPodReadinessGateReady, namespace, tgbName, d)
}

func (m *MockCollector) ObserveControllerReconcileError(controller string, errorCategory string) {
	m.Invocations[MetricControllerReconcileErrors] = append(m.Invocations[MetricControllerReconcileErrors], MockCounterMetric{
		labelController:    controller,
		labelErrorCategory: errorCategory,
	})
}

func (m *MockCollector) ObserveControllerReconcileLatency(controller string, stage string, fn func()) {
	m.Invocations[MetricControllerReconcileStageDuration] = append(m.Invocations[MetricControllerReconcileStageDuration], MockCounterMetric{
		labelController:    controller,
		labelErrorCategory: stage,
	})
}

func (m *MockCollector) ObserveWebhookValidationError(webhookName string, errorCategory string) {
	m.Invocations[MetricWebhookValidationFailure] = append(m.Invocations[MetricWebhookValidationFailure], MockCounterMetric{
		webhookName:        webhookName,
		labelErrorCategory: errorCategory,
	})
}

func (m *MockCollector) ObserveWebhookMutationError(webhookName string, errorCategory string) {
	m.Invocations[MetricWebhookMutationFailure] = append(m.Invocations[MetricWebhookMutationFailure], MockCounterMetric{
		webhookName:        webhookName,
		labelErrorCategory: errorCategory,
	})
}

func (m *MockCollector) ObserveControllerCacheSize(resource string, count int) {
	m.Invocations[MetricControllerCacheObjectCount] = append(m.Invocations[MetricControllerCacheObjectCount], MockCounterMetric{
		resource: resource,
	})
}

func (m *MockCollector) ObserveControllerTopTalkers(controller, namespace string, name string) {
	m.Invocations[MetricControllerTopTalkers] = append(m.Invocations[MetricControllerTopTalkers], MockCounterMetric{
		labelController: controller,
		labelNamespace:  namespace,
		labelName:       name,
	})
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
	mockInvocations[MetricControllerReconcileErrors] = make([]interface{}, 0)
	mockInvocations[MetricControllerReconcileStageDuration] = make([]interface{}, 0)
	mockInvocations[MetricWebhookValidationFailure] = make([]interface{}, 0)
	mockInvocations[MetricWebhookMutationFailure] = make([]interface{}, 0)
	mockInvocations[MetricControllerCacheObjectCount] = make([]interface{}, 0)

	return &MockCollector{
		Invocations: mockInvocations,
	}
}
