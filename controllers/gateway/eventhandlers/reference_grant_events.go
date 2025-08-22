package eventhandlers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// NewEnqueueRequestsForReferenceGrantEvent creates handler for ReferenceGrant resources
func NewEnqueueRequestsForReferenceGrantEvent(httpRouteEventChan chan<- event.TypedGenericEvent[*gatewayv1.HTTPRoute],
	tcpRouteEventChan chan<- event.TypedGenericEvent[*gwalpha2.TCPRoute],
	udpRouteEventChan chan<- event.TypedGenericEvent[*gwalpha2.UDPRoute],
	tlsRouteEventChan chan<- event.TypedGenericEvent[*gwalpha2.TLSRoute],
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*gwbeta1.ReferenceGrant, reconcile.Request] {
	return &enqueueRequestsForReferenceGrantEvent{
		httpRouteEventChan: httpRouteEventChan,
		tcpRouteEventChan:  tcpRouteEventChan,
		udpRouteEventChan:  udpRouteEventChan,
		tlsRouteEventChan:  tlsRouteEventChan,
		k8sClient:          k8sClient,
		eventRecorder:      eventRecorder,
		logger:             logger,
	}
}

var _ handler.TypedEventHandler[*gwbeta1.ReferenceGrant, reconcile.Request] = (*enqueueRequestsForReferenceGrantEvent)(nil)

// enqueueRequestsForReferenceGrantEvent handles ReferenceGrant events
type enqueueRequestsForReferenceGrantEvent struct {
	httpRouteEventChan chan<- event.TypedGenericEvent[*gatewayv1.HTTPRoute]
	tcpRouteEventChan  chan<- event.TypedGenericEvent[*gwalpha2.TCPRoute]
	udpRouteEventChan  chan<- event.TypedGenericEvent[*gwalpha2.UDPRoute]
	tlsRouteEventChan  chan<- event.TypedGenericEvent[*gwalpha2.TLSRoute]
	k8sClient          client.Client
	eventRecorder      record.EventRecorder
	logger             logr.Logger
}

func (h *enqueueRequestsForReferenceGrantEvent) Create(ctx context.Context, e event.TypedCreateEvent[*gwbeta1.ReferenceGrant], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	referenceGrantNew := e.Object
	h.logger.V(1).Info("enqueue reference grant create event", "reference grant", referenceGrantNew.Name)
	h.enqueueImpactedRoutes(ctx, referenceGrantNew, nil)
}

func (h *enqueueRequestsForReferenceGrantEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*gwbeta1.ReferenceGrant], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	referenceGrantNew := e.ObjectNew
	referenceGrantOld := e.ObjectOld
	h.logger.V(1).Info("enqueue reference grant update event", "reference grant", referenceGrantNew.Name)
	h.enqueueImpactedRoutes(ctx, referenceGrantNew, referenceGrantOld)
}

func (h *enqueueRequestsForReferenceGrantEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*gwbeta1.ReferenceGrant], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	refgrant := e.Object
	h.logger.V(1).Info("enqueue reference grant delete event", "reference grant", refgrant.Name)
	h.enqueueImpactedRoutes(ctx, refgrant, nil)
}

func (h *enqueueRequestsForReferenceGrantEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*gwbeta1.ReferenceGrant], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	refgrant := e.Object
	h.logger.V(1).Info("enqueue reference grant generic event", "reference grant", refgrant.Name)
	h.enqueueImpactedRoutes(ctx, refgrant, nil)
}

func (h *enqueueRequestsForReferenceGrantEvent) enqueueImpactedRoutes(ctx context.Context, newRefGrant *gwbeta1.ReferenceGrant, oldRefGrant *gwbeta1.ReferenceGrant) {

	impactedRoutes := make(map[string]gwbeta1.ReferenceGrantFrom)

	for i, from := range newRefGrant.Spec.From {
		impactedRoutes[generateGrantFromKey(from)] = newRefGrant.Spec.From[i]
	}

	if oldRefGrant != nil {
		for i, from := range oldRefGrant.Spec.From {
			impactedRoutes[generateGrantFromKey(from)] = oldRefGrant.Spec.From[i]
		}
	}

	for _, impactedFrom := range impactedRoutes {
		switch string(impactedFrom.Kind) {
		case string(routeutils.HTTPRouteKind):
			if h.httpRouteEventChan == nil {
				continue
			}
			routes, err := routeutils.ListHTTPRoutes(ctx, h.k8sClient, &client.ListOptions{Namespace: string(impactedFrom.Namespace)})
			if err == nil {
				for _, route := range routes {
					h.httpRouteEventChan <- event.TypedGenericEvent[*gatewayv1.HTTPRoute]{
						Object: route.GetRawRoute().(*gatewayv1.HTTPRoute),
					}
				}

			} else {
				h.logger.Error(err, "Unable to list impacted http routes for reference grant event handler")
			}
		case string(routeutils.TCPRouteKind):
			if h.tcpRouteEventChan == nil {
				continue
			}
			routes, err := routeutils.ListTCPRoutes(ctx, h.k8sClient, &client.ListOptions{Namespace: string(impactedFrom.Namespace)})
			if err == nil {
				for _, route := range routes {
					h.tcpRouteEventChan <- event.TypedGenericEvent[*gwalpha2.TCPRoute]{
						Object: route.GetRawRoute().(*gwalpha2.TCPRoute),
					}
				}

			} else {
				h.logger.Error(err, "Unable to list impacted tcp routes for reference grant event handler")
			}
		case string(routeutils.UDPRouteKind):
			if h.udpRouteEventChan == nil {
				continue
			}
			routes, err := routeutils.ListUDPRoutes(ctx, h.k8sClient, &client.ListOptions{Namespace: string(impactedFrom.Namespace)})
			if err == nil {
				for _, route := range routes {
					h.udpRouteEventChan <- event.TypedGenericEvent[*gwalpha2.UDPRoute]{
						Object: route.GetRawRoute().(*gwalpha2.UDPRoute),
					}
				}

			} else {
				h.logger.Error(err, "Unable to list impacted udp routes for reference grant event handler")
			}
		case string(routeutils.TLSRouteKind):
			if h.tlsRouteEventChan == nil {
				continue
			}
			routes, err := routeutils.ListTLSRoutes(ctx, h.k8sClient, &client.ListOptions{Namespace: string(impactedFrom.Namespace)})
			if err == nil {
				for _, route := range routes {
					h.tlsRouteEventChan <- event.TypedGenericEvent[*gwalpha2.TLSRoute]{
						Object: route.GetRawRoute().(*gwalpha2.TLSRoute),
					}
				}

			} else {
				h.logger.Error(err, "Unable to list impacted tls routes for reference grant event handler")
			}
		}
	}
}

func generateGrantFromKey(from gwbeta1.ReferenceGrantFrom) string {
	return fmt.Sprintf("%s-%s", from.Kind, from.Namespace)
}
