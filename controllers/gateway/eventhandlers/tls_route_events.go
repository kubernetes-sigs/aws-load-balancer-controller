package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/gatewayutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// NewEnqueueRequestsForTLSRouteEvent creates handler for TLSRoute resources
func NewEnqueueRequestsForTLSRouteEvent(
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*gwalpha2.TLSRoute, reconcile.Request] {
	return &enqueueRequestsForTLSRouteEvent{
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.TypedEventHandler[*gwalpha2.TLSRoute, reconcile.Request] = (*enqueueRequestsForTLSRouteEvent)(nil)

// enqueueRequestsForTLSRouteEvent handles TLSRoute events
type enqueueRequestsForTLSRouteEvent struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForTLSRouteEvent) Create(ctx context.Context, e event.TypedCreateEvent[*gwalpha2.TLSRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	routeNew := e.Object
	h.logger.V(1).Info("enqueue tlsroute create event", "tlsroute", routeNew.Name)
	h.enqueueImpactedGateways(ctx, routeNew, queue)
}

func (h *enqueueRequestsForTLSRouteEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*gwalpha2.TLSRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	routeNew := e.ObjectNew
	h.logger.V(1).Info("enqueue tlsroute update event", "tlsroute", routeNew.Name)
	h.enqueueImpactedGateways(ctx, routeNew, queue)
}

func (h *enqueueRequestsForTLSRouteEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*gwalpha2.TLSRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	route := e.Object
	h.logger.V(1).Info("enqueue tlsroute delete event", "tlsroute", route.Name)
	h.enqueueImpactedGateways(ctx, route, queue)
}

func (h *enqueueRequestsForTLSRouteEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*gwalpha2.TLSRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	route := e.Object
	h.logger.V(1).Info("enqueue tlsroute generic event", "tlsroute", route.Name)
	h.enqueueImpactedGateways(ctx, route, queue)
}

func (h *enqueueRequestsForTLSRouteEvent) enqueueImpactedGateways(ctx context.Context, route *gwalpha2.TLSRoute, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gateways, err := gatewayutils.GetImpactedGatewaysFromParentRefs(ctx, h.k8sClient, route.Spec.ParentRefs, route.Status.Parents, route.Namespace, constants.NLBGatewayController)
	if err != nil {
		h.logger.V(1).Info("ignoring unknown gateways referred by", "tlsroute", route.Name, "error", err)
	}
	for _, gw := range gateways {
		h.logger.V(1).Info("enqueue gateway for tlsroute event",
			"tlsroute", k8s.NamespacedName(route),
			"gateway", gw)
		queue.Add(reconcile.Request{NamespacedName: gw})
	}
}
