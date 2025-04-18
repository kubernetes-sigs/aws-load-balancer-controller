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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// NewEnqueueRequestsForGRPCRouteEvent creates handler for GRPCRoute resources
func NewEnqueueRequestsForGRPCRouteEvent(
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*gatewayv1.GRPCRoute, reconcile.Request] {
	return &enqueueRequestsForGRPCRouteEvent{
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.TypedEventHandler[*gatewayv1.GRPCRoute, reconcile.Request] = (*enqueueRequestsForGRPCRouteEvent)(nil)

// enqueueRequestsForGRPCRouteEvent handles GRPCRoute events
type enqueueRequestsForGRPCRouteEvent struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForGRPCRouteEvent) Create(ctx context.Context, e event.TypedCreateEvent[*gatewayv1.GRPCRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	routeNew := e.Object
	h.logger.V(1).Info("enqueue grpcroute create event", "grpcroute", routeNew.Name)
	h.enqueueImpactedGateways(ctx, routeNew, queue)
}

func (h *enqueueRequestsForGRPCRouteEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*gatewayv1.GRPCRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	routeNew := e.ObjectNew
	h.logger.V(1).Info("enqueue grpcroute update event", "grpcroute", routeNew.Name)
	h.enqueueImpactedGateways(ctx, routeNew, queue)
}

func (h *enqueueRequestsForGRPCRouteEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*gatewayv1.GRPCRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	route := e.Object
	h.logger.V(1).Info("enqueue grpcroute delete event", "grpcroute", route.Name)
	h.enqueueImpactedGateways(ctx, route, queue)
}

func (h *enqueueRequestsForGRPCRouteEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*gatewayv1.GRPCRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	route := e.Object
	h.logger.V(1).Info("enqueue grpcroute generic event", "grpcroute", route.Name)
	h.enqueueImpactedGateways(ctx, route, queue)
}

func (h *enqueueRequestsForGRPCRouteEvent) enqueueImpactedGateways(ctx context.Context, route *gatewayv1.GRPCRoute, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gateways, err := GetImpactedGatewaysFromParentRefs(ctx, h.k8sClient, route.Spec.ParentRefs, route.Namespace, constants.ALBGatewayController)
	if err != nil {
		h.logger.V(1).Info("ignoring unknown gateways referred by", "grpcroute", route.Name, "error", err)
	}
	for _, gw := range gateways {
		h.logger.V(1).Info("enqueue gateway for grpcroute event",
			"grpcroute", k8s.NamespacedName(route),
			"gateway", gw)
		queue.Add(reconcile.Request{NamespacedName: gw})
	}
}
