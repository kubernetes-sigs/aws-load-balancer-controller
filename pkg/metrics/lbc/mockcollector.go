package lbc

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	metricsutil "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockCollector struct {
	Invocations map[string][]interface{}
	mgr         ctrl.Manager
	counter     *metricsutil.ReconcileCounters
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

func (m *MockCollector) CollectTopTalker(ctx context.Context) {
	topThreeReconciles := m.counter.GetTopReconciles(3)
	for resourceType, items := range topThreeReconciles {
		for _, item := range items {
			m.ObserveControllerTopTalkers(resourceType, item.Resource.Namespace, item.Resource.Name)
		}
	}
}

func (m *MockCollector) StartCollectTopTalkers(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.CollectTopTalker(ctx)
		}
	}

}

func (m *MockCollector) StartCollectCacheSize(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.CollectCacheSize(ctx)
		}
	}

}

func (c *MockCollector) CollectCacheSize(ctx context.Context) error {
	cache := c.mgr.GetCache()
	collectableResources := map[string]client.ObjectList{
		"service":            &corev1.ServiceList{},
		"ingress":            &networking.IngressList{},
		"targetgroupbinding": &elbv2api.TargetGroupBindingList{},
	}
	for resourceType, resourceList := range collectableResources {
		if err := cache.List(ctx, resourceList); err != nil {
			return fmt.Errorf("failed to list %s: %w", resourceType, err)
		}
		items, err := meta.ExtractList(resourceList)
		if err != nil {
			return fmt.Errorf("failed to extract items from %s list: %w", resourceType, err)
		}

		c.ObserveControllerCacheSize(resourceType, len(items))
	}
	return nil
}

func NewMockCollector() MetricCollector {
	mockInvocations := make(map[string][]interface{})
	mockInvocations[MetricPodReadinessGateReady] = make([]interface{}, 0)
	mockInvocations[MetricControllerReconcileErrors] = make([]interface{}, 0)
	mockInvocations[MetricControllerReconcileStageDuration] = make([]interface{}, 0)
	mockInvocations[MetricWebhookValidationFailure] = make([]interface{}, 0)
	mockInvocations[MetricWebhookMutationFailure] = make([]interface{}, 0)
	mockInvocations[MetricControllerCacheObjectCount] = make([]interface{}, 0)
	mockInvocations[MetricControllerTopTalkers] = make([]interface{}, 0)

	return &MockCollector{
		Invocations: mockInvocations,
	}
}
