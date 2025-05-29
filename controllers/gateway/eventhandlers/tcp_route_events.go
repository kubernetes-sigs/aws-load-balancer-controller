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

// NewEnqueueRequestsForTCPRouteEvent creates handler for TCPRoute resources
func NewEnqueueRequestsForTCPRouteEvent(
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*gwalpha2.TCPRoute, reconcile.Request] {
	return &enqueueRequestsForTCPRouteEvent{
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.TypedEventHandler[*gwalpha2.TCPRoute, reconcile.Request] = (*enqueueRequestsForTCPRouteEvent)(nil)

// enqueueRequestsForTCPRouteEvent handles TCPRoute events
type enqueueRequestsForTCPRouteEvent struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForTCPRouteEvent) Create(ctx context.Context, e event.TypedCreateEvent[*gwalpha2.TCPRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	routeNew := e.Object
	h.logger.V(1).Info("enqueue tcproute create event", "tcproute", routeNew.Name)
	h.enqueueImpactedGateways(ctx, routeNew, queue)
}

func (h *enqueueRequestsForTCPRouteEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*gwalpha2.TCPRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	routeNew := e.ObjectNew
	h.logger.V(1).Info("enqueue tcproute update event", "tcproute", routeNew.Name)
	h.enqueueImpactedGateways(ctx, routeNew, queue)
}

func (h *enqueueRequestsForTCPRouteEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*gwalpha2.TCPRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	route := e.Object
	h.logger.V(1).Info("enqueue tcproute delete event", "tcproute", route.Name)
	h.enqueueImpactedGateways(ctx, route, queue)
}

func (h *enqueueRequestsForTCPRouteEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*gwalpha2.TCPRoute], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	route := e.Object
	h.logger.V(1).Info("enqueue tcproute generic event", "tcproute", route.Name)
	h.enqueueImpactedGateways(ctx, route, queue)
}

func (h *enqueueRequestsForTCPRouteEvent) enqueueImpactedGateways(ctx context.Context, route *gwalpha2.TCPRoute, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gateways, err := GetImpactedGatewaysFromParentRefs(ctx, h.k8sClient, route.Spec.ParentRefs, route.Status.Parents, route.Namespace, constants.NLBGatewayController)
	if err != nil {
		h.logger.V(1).Info("ignoring unknown gateways referred by", "tcproute", route.Name, "error", err)
	}
	for _, gw := range gateways {
		h.logger.V(1).Info("enqueue gateway for tcproute event",
			"tcproute", k8s.NamespacedName(route),
			"gateway", gw)
		queue.Add(reconcile.Request{NamespacedName: gw})
	}
}
