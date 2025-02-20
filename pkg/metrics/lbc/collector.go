package lbc

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MetricCollector interface {
	// ObservePodReadinessGateReady this metric is useful to determine how fast pods are becoming ready in the load balancer.
	// Due to some architectural constraints, we can only emit this metric for pods that are using readiness gates.
	ObservePodReadinessGateReady(namespace string, tgbName string, duration time.Duration)
	ObserveControllerReconcileError(controller string, errorType string)
	ObserveWebhookValidationError(webhookName string, errorType string)
	ObserveWebhookMutationError(webhookName string, errorType string)
	ObserveControllerCacheSize(resource string, count int)
	ObserveControllerReconcileLatency(controller string, stage string, fn func())
}

type Collector struct {
	instruments *instruments
	mgr         ctrl.Manager
}

type noOpCollector struct{}

func (n *noOpCollector) ObservePodReadinessGateReady(_ string, _ string, _ time.Duration) {
}

func NewCollector(registerer prometheus.Registerer, mgr ctrl.Manager) *Collector {
	instruments := newInstruments(registerer)
	return &Collector{
		instruments: instruments,
		mgr:         mgr,
	}
}

func (c *Collector) ObservePodReadinessGateReady(namespace string, tgbName string, duration time.Duration) {
	c.instruments.podReadinessFlipSeconds.With(prometheus.Labels{
		labelNamespace: namespace,
		labelName:      tgbName,
	}).Observe(duration.Seconds())
}

func (c *Collector) ObserveControllerReconcileError(controller string, errorCategory string) {
	c.instruments.controllerReconcileErrors.With(prometheus.Labels{
		labelController:    controller,
		labelErrorCategory: errorCategory,
	}).Inc()
}

func (c *Collector) ObserveControllerReconcileLatency(controller string, stage string, fn func()) {
	start := time.Now()
	defer func() {
		c.instruments.controllerReconcileLatency.With(prometheus.Labels{
			labelController:     controller,
			labelReconcileStage: stage,
		}).Observe(time.Since(start).Seconds())
	}()
	fn()
}

func (c *Collector) ObserveWebhookValidationError(webhookName string, errorCategory string) {
	c.instruments.webhookValidationFailure.With(prometheus.Labels{
		labelWebhookName:   webhookName,
		labelErrorCategory: errorCategory,
	}).Inc()
}

func (c *Collector) ObserveWebhookMutationError(webhookName string, errorCategory string) {
	c.instruments.webhookMutationFailure.With(prometheus.Labels{
		labelWebhookName:   webhookName,
		labelErrorCategory: errorCategory,
	}).Inc()
}

func (c *Collector) ObserveControllerCacheSize(resource string, count int) {
	c.instruments.controllerCacheObjectCount.With(prometheus.Labels{
		LabelResource: resource,
	}).Set(float64(count))
}

func (c *Collector) StartCollectCacheSize(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.CollectCacheSize(ctx)
		}
	}
}

func (c *Collector) CollectCacheSize(ctx context.Context) error {
	cache := c.mgr.GetCache()
	colletableResources := map[string]client.ObjectList{
		"service":            &corev1.ServiceList{},
		"ingress":            &networking.IngressList{},
		"targetgroupbinding": &elbv2api.TargetGroupBindingList{},
	}
	for resourceType, resourceList := range colletableResources {
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
