package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// NewenqueueRequestsForServiceEvent detects changes to TargetGroupConfiguration and enqueues all gateway classes and gateways that
// would effected by a change in the TargetGroupConfiguration
func NewEnqueueRequestsForServiceEvent(httpRouteEventChan chan<- event.TypedGenericEvent[*gatewayv1.HTTPRoute],
	grpcRouteEventChan chan<- event.TypedGenericEvent[*gatewayv1.GRPCRoute],
	tcpRouteEventChan chan<- event.TypedGenericEvent[*gwalpha2.TCPRoute],
	udpRouteEventChan chan<- event.TypedGenericEvent[*gwalpha2.UDPRoute],
	tlsRouteEventChan chan<- event.TypedGenericEvent[*gwalpha2.TLSRoute], k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger, gwController string) handler.TypedEventHandler[*corev1.Service, reconcile.Request] {
	return &enqueueRequestsForServiceEvent{
		httpRouteEventChan: httpRouteEventChan,
		grpcRouteEventChan: grpcRouteEventChan,
		tcpRouteEventChan:  tcpRouteEventChan,
		udpRouteEventChan:  udpRouteEventChan,
		tlsRouteEventChan:  tlsRouteEventChan,
		k8sClient:          k8sClient,
		eventRecorder:      eventRecorder,
		logger:             logger,
		gwController:       gwController,
	}
}

var _ handler.TypedEventHandler[*corev1.Service, reconcile.Request] = (*enqueueRequestsForServiceEvent)(nil)

type enqueueRequestsForServiceEvent struct {
	httpRouteEventChan chan<- event.TypedGenericEvent[*gatewayv1.HTTPRoute]
	grpcRouteEventChan chan<- event.TypedGenericEvent[*gatewayv1.GRPCRoute]
	tcpRouteEventChan  chan<- event.TypedGenericEvent[*gwalpha2.TCPRoute]
	udpRouteEventChan  chan<- event.TypedGenericEvent[*gwalpha2.UDPRoute]
	tlsRouteEventChan  chan<- event.TypedGenericEvent[*gwalpha2.TLSRoute]
	k8sClient          client.Client
	eventRecorder      record.EventRecorder
	logger             logr.Logger
	gwController       string
}

func (h *enqueueRequestsForServiceEvent) Create(ctx context.Context, e event.TypedCreateEvent[*corev1.Service], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	svcNew := e.Object
	h.logger.V(1).Info("enqueue service create event", "service", svcNew.Name)
	h.enqueueImpactedRoutes(ctx, svcNew)
}

func (h *enqueueRequestsForServiceEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*corev1.Service], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	svcNew := e.ObjectNew
	h.logger.V(1).Info("enqueue service update event", "service", svcNew.Name)
	h.enqueueImpactedRoutes(ctx, svcNew)
}

func (h *enqueueRequestsForServiceEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*corev1.Service], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	svc := e.Object
	h.logger.V(1).Info("enqueue service delete event", "service", svc.Name)
	h.enqueueImpactedRoutes(ctx, svc)
}

func (h *enqueueRequestsForServiceEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*corev1.Service], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	svc := e.Object
	h.logger.V(1).Info("enqueue service generic event", "service", svc.Name)
	h.enqueueImpactedRoutes(ctx, svc)
}

func (h *enqueueRequestsForServiceEvent) enqueueImpactedRoutes(
	ctx context.Context,
	svc *corev1.Service,
) {
	if h.gwController == constants.NLBGatewayController {
		h.enqueueImpactedL4Routes(ctx, svc)
		return
	}
	h.enqueueImpactedL7Routes(ctx, svc)

}

func (h *enqueueRequestsForServiceEvent) enqueueImpactedL4Routes(
	ctx context.Context,
	svc *corev1.Service,
) {
	l4Routes, err := routeutils.ListL4Routes(ctx, h.k8sClient)
	if err != nil {
		h.logger.V(1).Info("ignoring to enqueue L4 impacted routes ", "error: ", err)
	}
	filteredRoutesBySvc := routeutils.FilterRoutesBySvc(l4Routes, svc)
	for _, route := range filteredRoutesBySvc {
		routeType := route.GetRouteKind()
		switch routeType {
		case routeutils.TCPRouteKind:
			h.logger.V(1).Info("enqueue tcproute for service event",
				"service", svc.Name,
				"tcproute", route.GetRouteNamespacedName())
			h.tcpRouteEventChan <- event.TypedGenericEvent[*gwalpha2.TCPRoute]{
				Object: route.GetRawRoute().(*gwalpha2.TCPRoute),
			}
		case routeutils.UDPRouteKind:
			h.logger.V(1).Info("enqueue updroute for service event",
				"service", svc.Name,
				"udproute", route.GetRouteNamespacedName())
			h.udpRouteEventChan <- event.TypedGenericEvent[*gwalpha2.UDPRoute]{
				Object: route.GetRawRoute().(*gwalpha2.UDPRoute),
			}
		case routeutils.TLSRouteKind:
			h.logger.V(1).Info("enqueue tlsroute for service event",
				"service", svc.Name,
				"tlsroute", route.GetRouteNamespacedName())
			h.tlsRouteEventChan <- event.TypedGenericEvent[*gwalpha2.TLSRoute]{
				Object: route.GetRawRoute().(*gwalpha2.TLSRoute),
			}
		}
	}
	return
}

func (h *enqueueRequestsForServiceEvent) enqueueImpactedL7Routes(
	ctx context.Context,
	svc *corev1.Service,
) {
	l7Routes, err := routeutils.ListL7Routes(ctx, h.k8sClient)
	if err != nil {
		h.logger.V(1).Info("ignoring to enqueue impacted L7 routes ", "error: ", err)
	}
	filteredRoutesBySvc := routeutils.FilterRoutesBySvc(l7Routes, svc)
	for _, route := range filteredRoutesBySvc {
		routeType := route.GetRouteKind()
		switch routeType {
		case routeutils.HTTPRouteKind:
			h.logger.V(1).Info("enqueue httproute for service event",
				"service", svc.Name,
				"httproute", route.GetRouteNamespacedName())
			h.httpRouteEventChan <- event.TypedGenericEvent[*gatewayv1.HTTPRoute]{
				Object: route.GetRawRoute().(*gatewayv1.HTTPRoute),
			}
		case routeutils.GRPCRouteKind:
			h.logger.V(1).Info("enqueue grpcroute for service event",
				"service", svc.Name,
				"grpcroute", route.GetRouteNamespacedName())
			h.grpcRouteEventChan <- event.TypedGenericEvent[*gatewayv1.GRPCRoute]{
				Object: route.GetRawRoute().(*gatewayv1.GRPCRoute),
			}
		}
	}
	return
}
