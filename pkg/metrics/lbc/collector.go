package lbc

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	metricsutil "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MetricCollector interface {
	// ObservePodReadinessGateReady this metric is useful to determine how fast pods are becoming ready in the load balancer.
	// Due to some architectural constraints, we can only emit this metric for pods that are using readiness gates.
	ObservePodReadinessGateReady(namespace string, tgbName string, duration time.Duration)
	ObserveQUICTargetMissingServerId(namespace string, tgbName string)
	ObserveControllerReconcileError(controller string, errorType string)
	ObserveControllerReconcileLatency(controller string, stage string, fn func())
	ObserveWebhookValidationError(webhookName string, errorType string)
	ObserveWebhookMutationError(webhookName string, errorType string)
	// ObserveIngressCertErrorSkipped increments the counter when an Ingress is skipped due to certificate error
	ObserveIngressCertErrorSkipped(namespace, ingressName, groupName string)
	StartCollectTopTalkers(ctx context.Context)
	StartCollectCacheSize(ctx context.Context)
}

type collector struct {
	instruments *instruments
	mgr         ctrl.Manager
	counter     *metricsutil.ReconcileCounters
	logger      logr.Logger
}

type noOpCollector struct{}

func (n *noOpCollector) ObserveQUICTargetMissingServerId(namespace string, tgbName string) {
}

func (n *noOpCollector) ObservePodReadinessGateReady(_ string, _ string, _ time.Duration) {
}

func (n *noOpCollector) ObserveControllerReconcileError(_ string, _ string) {
}

func (n *noOpCollector) ObserveWebhookValidationError(_ string, _ string) {
}

func (n *noOpCollector) ObserveWebhookMutationError(_ string, _ string) {
}

func (n *noOpCollector) ObserveControllerCacheSize(_ string, _ int) {
}

func (n *noOpCollector) ObserveControllerReconcileLatency(_ string, _ string, fn func()) {
}

func (n *noOpCollector) StartCollectTopTalkers(_ context.Context) {
}

func (n *noOpCollector) StartCollectCacheSize(_ context.Context) {
}

func (n *noOpCollector) ObserveIngressCertErrorSkipped(_, _, _ string) {
}

func NewCollector(registerer prometheus.Registerer, mgr ctrl.Manager, reconcileCounters *metricsutil.ReconcileCounters, logger logr.Logger) MetricCollector {
	if registerer == nil {
		return &noOpCollector{}
	}

	instruments := newInstruments(registerer)
	return &collector{
		instruments: instruments,
		mgr:         mgr,
		counter:     reconcileCounters,
		logger:      logger,
	}
}

func (c *collector) ObservePodReadinessGateReady(namespace string, tgbName string, duration time.Duration) {
	c.instruments.podReadinessFlipSeconds.With(prometheus.Labels{
		labelNamespace: namespace,
		labelName:      tgbName,
	}).Observe(duration.Seconds())
}

func (c *collector) ObserveQUICTargetMissingServerId(namespace string, tgbName string) {
	c.instruments.quicTargetsMissingServerId.With(prometheus.Labels{
		labelNamespace: namespace,
		labelName:      tgbName,
	}).Inc()
}

func (c *collector) ObserveControllerReconcileError(controller string, errorCategory string) {
	c.instruments.controllerReconcileErrors.With(prometheus.Labels{
		labelController:    controller,
		labelErrorCategory: errorCategory,
	}).Inc()
}

func (c *collector) ObserveControllerReconcileLatency(controller string, stage string, fn func()) {
	start := time.Now()
	defer func() {
		c.instruments.controllerReconcileLatency.With(prometheus.Labels{
			labelController:     controller,
			labelReconcileStage: stage,
		}).Observe(time.Since(start).Seconds())
	}()
	fn()
}

func (c *collector) ObserveWebhookValidationError(webhookName string, errorCategory string) {
	c.instruments.webhookValidationFailure.With(prometheus.Labels{
		labelWebhookName:   webhookName,
		labelErrorCategory: errorCategory,
	}).Inc()
}

func (c *collector) ObserveWebhookMutationError(webhookName string, errorCategory string) {
	c.instruments.webhookMutationFailure.With(prometheus.Labels{
		labelWebhookName:   webhookName,
		labelErrorCategory: errorCategory,
	}).Inc()
}

func (c *collector) ObserveIngressCertErrorSkipped(namespace, ingressName, groupName string) {
	c.instruments.ingressCertErrorSkipped.With(prometheus.Labels{
		labelNamespace:   namespace,
		labelIngressName: ingressName,
		labelGroupName:   groupName,
	}).Inc()
}

func (c *collector) ObserveControllerCacheSize(resource string, count int) {
	c.instruments.controllerCacheObjectCount.With(prometheus.Labels{
		LabelResource: resource,
	}).Set(float64(count))
}

func (c *collector) ObserveControllerTopThreeTalkers(controller, namespace string, name string, count int) {
	c.instruments.controllerReconcileTopTalkers.With(prometheus.Labels{
		labelController: controller,
		labelNamespace:  namespace,
		labelName:       name,
	}).Set(float64(count))
}

func (c *collector) StartCollectCacheSize(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.CollectCacheSize(ctx); err != nil {
				c.logger.Error(err, "failed to collect cache size")
			}
		}
	}
}

func (c *collector) CollectCacheSize(ctx context.Context) error {
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

func (c *collector) StartCollectTopTalkers(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.CollectTopTalker(ctx)
		}
	}
}

func (c *collector) CollectTopTalker(ctx context.Context) {
	topThreeReconciles := c.counter.GetTopReconciles(3)
	for resourceType, items := range topThreeReconciles {
		for _, item := range items {
			c.ObserveControllerTopThreeTalkers(resourceType, item.Resource.Namespace, item.Resource.Name, item.Count)
		}
	}
	c.counter.ResetCounter()
}
