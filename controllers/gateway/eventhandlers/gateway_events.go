package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// NewEnqueueRequestsForGatewayEvent creates handler for Gateway resources
func NewEnqueueRequestsForGatewayEventHandler(
	k8sClient client.Client, eventRecorder record.EventRecorder, gwController string, logger logr.Logger) handler.TypedEventHandler[*gwv1.Gateway, reconcile.Request] {
	return &enqueueRequestsForGatewayEvent{
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		gwController:  gwController,
		logger:        logger,
	}
}

var _ handler.TypedEventHandler[*gwv1.Gateway, reconcile.Request] = (*enqueueRequestsForGatewayEvent)(nil)

// enqueueRequestsForGatewayEvent handles GatewayClass events
type enqueueRequestsForGatewayEvent struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	gwController  string
	logger        logr.Logger
}

func (h *enqueueRequestsForGatewayEvent) Create(ctx context.Context, e event.TypedCreateEvent[*gwv1.Gateway], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gw := e.Object
	h.logger.V(1).Info("enqueue gateway create event", "gateway", k8s.NamespacedName(gw))
	h.enqueueImpactedGateway(ctx, gw, queue)
}

func (h *enqueueRequestsForGatewayEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*gwv1.Gateway], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gw := e.ObjectNew
	h.logger.V(1).Info("enqueue gateway update event", "gateway", k8s.NamespacedName(gw))
	h.enqueueImpactedGateway(ctx, gw, queue)
}

func (h *enqueueRequestsForGatewayEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*gwv1.Gateway], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gw := e.Object
	h.logger.V(1).Info("enqueue gateway delete event", "gateway", k8s.NamespacedName(gw))
	h.enqueueImpactedGateway(ctx, gw, queue)
}

func (h *enqueueRequestsForGatewayEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*gwv1.Gateway], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gw := e.Object
	h.logger.V(1).Info("enqueue gateway delete event", "gateway", k8s.NamespacedName(gw))
	h.enqueueImpactedGateway(ctx, gw, queue)
}

func (h *enqueueRequestsForGatewayEvent) enqueueImpactedGateway(ctx context.Context, gw *gwv1.Gateway, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	if gw == nil {
		return
	}
	if IsGatewayManagedByLBController(ctx, h.k8sClient, gw, h.gwController) {
		h.logger.V(1).Info("enqueue gateway",
			"gateway", k8s.NamespacedName(gw))
		queue.Add(reconcile.Request{NamespacedName: k8s.NamespacedName(gw)})
	}
}
