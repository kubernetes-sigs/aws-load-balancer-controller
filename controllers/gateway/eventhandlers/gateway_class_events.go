package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/gatewayutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// NewEnqueueRequestsForGatewayClassEvent creates handler for GatewayClass resources
func NewEnqueueRequestsForGatewayClassEvent(
	k8sClient client.Client, eventRecorder record.EventRecorder, gwController string, finalizerManager k8s.FinalizerManager, logger logr.Logger) handler.TypedEventHandler[*gatewayv1.GatewayClass, reconcile.Request] {
	return &enqueueRequestsForGatewayClassEvent{
		k8sClient:        k8sClient,
		finalizerManager: finalizerManager,
		eventRecorder:    eventRecorder,
		gwController:     gwController,
		logger:           logger,
	}
}

var _ handler.TypedEventHandler[*gatewayv1.GatewayClass, reconcile.Request] = (*enqueueRequestsForGatewayClassEvent)(nil)

// enqueueRequestsForGatewayClassEvent handles GatewayClass events
type enqueueRequestsForGatewayClassEvent struct {
	k8sClient        client.Client
	eventRecorder    record.EventRecorder
	gwController     string
	finalizerManager k8s.FinalizerManager
	logger           logr.Logger
}

func (h *enqueueRequestsForGatewayClassEvent) Create(ctx context.Context, e event.TypedCreateEvent[*gatewayv1.GatewayClass], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gwClassNew := e.Object
	h.logger.V(1).Info("enqueue gatewayclass create event", "gatewayclass", gwClassNew.Name)
	h.enqueueImpactedGateways(ctx, gwClassNew, queue)
}

func (h *enqueueRequestsForGatewayClassEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*gatewayv1.GatewayClass], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gwClassNew := e.ObjectNew

	h.logger.V(1).Info("enqueue gatewayclass update event", "gatewayclass", gwClassNew.Name)
	h.enqueueImpactedGateways(ctx, gwClassNew, queue)
}

// Delete is not implemented for this handler as GatewayClass deletion should be finalized and is prevented while referenced by Gateways
func (h *enqueueRequestsForGatewayClassEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*gatewayv1.GatewayClass], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForGatewayClassEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*gatewayv1.GatewayClass], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gwClass := e.Object
	h.enqueueImpactedGateways(ctx, gwClass, queue)
}

func (h *enqueueRequestsForGatewayClassEvent) enqueueImpactedGateways(ctx context.Context, gwClass *gatewayv1.GatewayClass, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gwList := gatewayutils.GetGatewaysManagedByGatewayClass(ctx, h.k8sClient, gwClass, h.gwController)

	for _, gw := range gwList {
		h.logger.V(1).Info("enqueue gateway for gatewayclass event",
			"gatewayclass", gwClass.GetName(),
			"gateway", k8s.NamespacedName(gw))
		queue.Add(reconcile.Request{NamespacedName: k8s.NamespacedName(gw)})

	}
}
