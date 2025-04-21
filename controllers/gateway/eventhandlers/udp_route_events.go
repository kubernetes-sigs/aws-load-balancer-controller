package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// NewEnqueueRequestsForUDPRouteEvent creates handler for UDPRoute resources
func NewEnqueueRequestsForUDPRouteEvent(
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*gwalpha2.UDPRoute, reconcile.Request] {
	return &enqueueRequestsForUDPRouteEvent{
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.TypedEventHandler[*gwalpha2.UDPRoute, reconcile.Request] = (*enqueueRequestsForUDPRouteEvent)(nil)

// enqueueRequestsForUDPRouteEvent handles UDPRoute events
type enqueueRequestsForUDPRouteEvent struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForUDPRouteEvent) Create(ctx context.Context, e event.TypedCreateEvent[*gwalpha2.UDPRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	routeNew := e.Object
	h.logger.V(1).Info("enqueue udproute create event", "udproute", routeNew.Name)
	h.enqueueImpactedGateways(ctx, routeNew, queue)
}

func (h *enqueueRequestsForUDPRouteEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*gwalpha2.UDPRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	routeNew := e.ObjectNew
	h.logger.V(1).Info("enqueue udproute update event", "udproute", routeNew.Name)
	h.enqueueImpactedGateways(ctx, routeNew, queue)
}

func (h *enqueueRequestsForUDPRouteEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*gwalpha2.UDPRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	route := e.Object
	h.logger.V(1).Info("enqueue udproute delete event", "udproute", route.Name)
	h.enqueueImpactedGateways(ctx, route, queue)
}

func (h *enqueueRequestsForUDPRouteEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*gwalpha2.UDPRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	route := e.Object
	h.logger.V(1).Info("enqueue udproute generic event", "udproute", route.Name)
	h.enqueueImpactedGateways(ctx, route, queue)
}

func (h *enqueueRequestsForUDPRouteEvent) enqueueImpactedGateways(ctx context.Context, route *gwalpha2.UDPRoute, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gateways, err := GetImpactedGatewaysFromParentRefs(ctx, h.k8sClient, route.Spec.ParentRefs, route.Namespace, constants.NLBGatewayController)
	if err != nil {
		h.logger.V(1).Info("ignoring unknown gateways referred by", "udproute", route.Name, "error", err)
	}
	for _, gw := range gateways {
		h.logger.V(1).Info("enqueue gateway for udproute event",
			"udproute", k8s.NamespacedName(route),
			"gateway", gw)
		queue.Add(reconcile.Request{NamespacedName: gw})
	}
}
