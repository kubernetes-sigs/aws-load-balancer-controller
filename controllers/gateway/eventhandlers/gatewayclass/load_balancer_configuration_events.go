package gatewayclasseventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/gatewayutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// NewEnqueueRequestsForLoadBalancerConfigurationEvent creates handler for LoadBalancerConfiguration resources
func NewEnqueueRequestsForLoadBalancerConfigurationEvent(gwClassEventChan chan<- event.TypedGenericEvent[*gatewayv1.GatewayClass],
	k8sClient client.Client, eventRecorder record.EventRecorder, gwControllers sets.Set[string], finalizerManager k8s.FinalizerManager, logger logr.Logger) handler.TypedEventHandler[*elbv2gw.LoadBalancerConfiguration, reconcile.Request] {
	return &enqueueRequestsForLoadBalancerConfigurationEvent{
		gwClassEventChan: gwClassEventChan,
		k8sClient:        k8sClient,
		eventRecorder:    eventRecorder,
		gwControllers:    gwControllers,
		finalizerManager: finalizerManager,
		logger:           logger,
	}
}

var _ handler.TypedEventHandler[*elbv2gw.LoadBalancerConfiguration, reconcile.Request] = (*enqueueRequestsForLoadBalancerConfigurationEvent)(nil)

// enqueueRequestsForLoadBalancerConfigurationEvent handles LoadBalancerConfiguration events
type enqueueRequestsForLoadBalancerConfigurationEvent struct {
	gwClassEventChan chan<- event.TypedGenericEvent[*gatewayv1.GatewayClass]
	k8sClient        client.Client
	eventRecorder    record.EventRecorder
	gwControllers    sets.Set[string]
	finalizerManager k8s.FinalizerManager
	logger           logr.Logger
}

func (h *enqueueRequestsForLoadBalancerConfigurationEvent) Create(ctx context.Context, e event.TypedCreateEvent[*elbv2gw.LoadBalancerConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	lbconfig := e.Object
	h.logger.V(1).Info("enqueue loadbalancerconfiguration create event", "loadbalancerconfiguration", k8s.NamespacedName(lbconfig))
	h.enqueueImpactedGatewayClass(ctx, lbconfig, queue)
}

func (h *enqueueRequestsForLoadBalancerConfigurationEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*elbv2gw.LoadBalancerConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	lbconfigNew := e.ObjectNew
	h.logger.V(1).Info("enqueue loadbalancerconfiguration update event", "loadbalancerconfiguration", k8s.NamespacedName(lbconfigNew))
	// to remove finalizers on this residual unused lb config so that the deletion can be done on these
	if !lbconfigNew.DeletionTimestamp.IsZero() && k8s.HasFinalizer(lbconfigNew, shared_constants.LoadBalancerConfigurationFinalizer) && !gatewayutils.IsLBConfigInUse(ctx, lbconfigNew, nil, nil, h.k8sClient, h.gwControllers) {
		if err := h.finalizerManager.RemoveFinalizers(ctx, lbconfigNew, shared_constants.LoadBalancerConfigurationFinalizer); err != nil {
			h.logger.V(1).Info("failed to remove finalizers on load balancer configuration as its currently in use", "load balancer configuration", lbconfigNew.Name)
			return
		}
		return
	}
	h.enqueueImpactedGatewayClass(ctx, lbconfigNew, queue)
}

func (h *enqueueRequestsForLoadBalancerConfigurationEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*elbv2gw.LoadBalancerConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	lbconfig := e.Object
	h.logger.V(1).Info("enqueue loadbalancerconfiguration delete event", "loadbalancerconfiguration", k8s.NamespacedName(lbconfig))
	h.enqueueImpactedGatewayClass(ctx, lbconfig, queue)
}

func (h *enqueueRequestsForLoadBalancerConfigurationEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*elbv2gw.LoadBalancerConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	lbconfig := e.Object
	h.logger.V(1).Info("enqueue loadbalancerconfiguration generic event", "loadbalancerconfiguration", k8s.NamespacedName(lbconfig))
	h.enqueueImpactedGatewayClass(ctx, lbconfig, queue)
}

func (h *enqueueRequestsForLoadBalancerConfigurationEvent) enqueueImpactedGatewayClass(ctx context.Context, lbconfig *elbv2gw.LoadBalancerConfiguration, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gwClasses := gatewayutils.GetImpactedGatewayClassesFromLbConfig(ctx, h.k8sClient, lbconfig, h.gwControllers)
	for _, gwClass := range gwClasses {
		h.logger.V(1).Info("enqueue gatewayClass for loadbalancerconfiguration event",
			"loadbalancerconfiguration", k8s.NamespacedName(lbconfig),
			"gatewayclass", k8s.NamespacedName(gwClass))
		h.gwClassEventChan <- event.TypedGenericEvent[*gatewayv1.GatewayClass]{
			Object: gwClass,
		}
	}
}
