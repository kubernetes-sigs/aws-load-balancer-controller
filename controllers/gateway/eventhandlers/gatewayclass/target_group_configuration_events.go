package gatewayclasseventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewEnqueueRequestsForTargetGroupConfigurationEvent creates a handler for TargetGroupConfiguration resources
// that emits synthetic LoadBalancerConfiguration events for LBCs referencing the changed TGC as their default.
func NewEnqueueRequestsForTargetGroupConfigurationEvent(lbcEventChan chan<- event.TypedGenericEvent[*elbv2gw.LoadBalancerConfiguration],
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*elbv2gw.TargetGroupConfiguration, reconcile.Request] {
	return &enqueueRequestsForTargetGroupConfigurationEvent{
		lbcEventChan:  lbcEventChan,
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.TypedEventHandler[*elbv2gw.TargetGroupConfiguration, reconcile.Request] = (*enqueueRequestsForTargetGroupConfigurationEvent)(nil)

type enqueueRequestsForTargetGroupConfigurationEvent struct {
	lbcEventChan  chan<- event.TypedGenericEvent[*elbv2gw.LoadBalancerConfiguration]
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForTargetGroupConfigurationEvent) Create(ctx context.Context, e event.TypedCreateEvent[*elbv2gw.TargetGroupConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.logger.V(1).Info("enqueue targetgroupconfiguration create event for gatewayclass", "targetgroupconfiguration", k8s.NamespacedName(e.Object))
	h.enqueueImpactedLBCs(ctx, e.Object)
}

func (h *enqueueRequestsForTargetGroupConfigurationEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*elbv2gw.TargetGroupConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.logger.V(1).Info("enqueue targetgroupconfiguration update event for gatewayclass", "targetgroupconfiguration", k8s.NamespacedName(e.ObjectNew))
	h.enqueueImpactedLBCs(ctx, e.ObjectNew)
}

func (h *enqueueRequestsForTargetGroupConfigurationEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*elbv2gw.TargetGroupConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.logger.V(1).Info("enqueue targetgroupconfiguration delete event for gatewayclass", "targetgroupconfiguration", k8s.NamespacedName(e.Object))
	h.enqueueImpactedLBCs(ctx, e.Object)
}

func (h *enqueueRequestsForTargetGroupConfigurationEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*elbv2gw.TargetGroupConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.logger.V(1).Info("enqueue targetgroupconfiguration generic event for gatewayclass", "targetgroupconfiguration", k8s.NamespacedName(e.Object))
	h.enqueueImpactedLBCs(ctx, e.Object)
}

// enqueueImpactedLBCs finds LoadBalancerConfigurations that reference this TGC as their
// defaultTargetGroupConfiguration and emits synthetic LBC events so the existing LBC handler
// can resolve impacted GatewayClasses.
// This handler only processes default TGCs (those without a targetReference).
func (h *enqueueRequestsForTargetGroupConfigurationEvent) enqueueImpactedLBCs(ctx context.Context, tgconfig *elbv2gw.TargetGroupConfiguration) {
	if tgconfig.Spec.TargetReference != nil {
		return
	}

	lbConfigList := &elbv2gw.LoadBalancerConfigurationList{}
	if err := h.k8sClient.List(ctx, lbConfigList, client.InNamespace(tgconfig.Namespace)); err != nil {
		h.logger.V(1).Info("failed to list loadbalancerconfigurations for targetgroupconfiguration event",
			"targetgroupconfiguration", k8s.NamespacedName(tgconfig),
			"error", err)
		return
	}

	for i := range lbConfigList.Items {
		lbConfig := &lbConfigList.Items[i]
		if lbConfig.Spec.DefaultTargetGroupConfiguration == nil || lbConfig.Spec.DefaultTargetGroupConfiguration.Name != tgconfig.Name {
			continue
		}

		h.logger.V(1).Info("enqueue loadbalancerconfiguration for targetgroupconfiguration event via gatewayclass path",
			"targetgroupconfiguration", k8s.NamespacedName(tgconfig),
			"loadbalancerconfiguration", k8s.NamespacedName(lbConfig))
		h.lbcEventChan <- event.TypedGenericEvent[*elbv2gw.LoadBalancerConfiguration]{
			Object: lbConfig,
		}
	}
}
