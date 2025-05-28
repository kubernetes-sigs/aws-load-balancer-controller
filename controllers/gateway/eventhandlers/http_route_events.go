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

// NewEnqueueRequestsForHTTPRouteEvent creates handler for HTTPRoute resources
func NewEnqueueRequestsForHTTPRouteEvent(
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*gatewayv1.HTTPRoute, reconcile.Request] {
	return &enqueueRequestsForHTTPRouteEvent{
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.TypedEventHandler[*gatewayv1.HTTPRoute, reconcile.Request] = (*enqueueRequestsForHTTPRouteEvent)(nil)

// enqueueRequestsForHTTPRouteEvent handles HTTPRoute events
type enqueueRequestsForHTTPRouteEvent struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForHTTPRouteEvent) Create(ctx context.Context, e event.TypedCreateEvent[*gatewayv1.HTTPRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	routeNew := e.Object
	h.logger.V(1).Info("enqueue httproute create event", "httproute", routeNew.Name)
	h.enqueueImpactedGateways(ctx, routeNew, queue)
}

func (h *enqueueRequestsForHTTPRouteEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*gatewayv1.HTTPRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	routeNew := e.ObjectNew
	h.logger.V(1).Info("enqueue httproute update event", "httproute", routeNew.Name)
	h.enqueueImpactedGateways(ctx, routeNew, queue)
}

func (h *enqueueRequestsForHTTPRouteEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*gatewayv1.HTTPRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	route := e.Object
	h.logger.V(1).Info("enqueue httproute delete event", "httproute", route.Name)
	h.enqueueImpactedGateways(ctx, route, queue)
}

func (h *enqueueRequestsForHTTPRouteEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*gatewayv1.HTTPRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	route := e.Object
	h.logger.V(1).Info("enqueue grpcroute generic event", "grpcroute", route.Name)
	h.enqueueImpactedGateways(ctx, route, queue)
}

func (h *enqueueRequestsForHTTPRouteEvent) enqueueImpactedGateways(ctx context.Context, route *gatewayv1.HTTPRoute, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gateways, err := GetImpactedGatewaysFromParentRefs(ctx, h.k8sClient, route.Spec.ParentRefs, route.Status.Parents, route.Namespace, constants.ALBGatewayController)
	if err != nil {
		h.logger.V(1).Info("ignoring unknown gateways referred by", "httproute", route.Name, "error", err)
	}
	for _, gw := range gateways {
		h.logger.V(1).Info("enqueue gateway for httproute event",
			"httproute", k8s.NamespacedName(route),
			"gateway", gw)
		queue.Add(reconcile.Request{NamespacedName: gw})
	}
}
