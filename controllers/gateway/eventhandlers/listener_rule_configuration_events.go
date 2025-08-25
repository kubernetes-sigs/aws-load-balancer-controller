package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/client-go/util/workqueue"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// NewEnqueueRequestsForListenerRuleConfigurationEvent creates handler for ListenerRuleConfiguration resources
func NewEnqueueRequestsForListenerRuleConfigurationEvent(httpRouteEventChan chan<- event.TypedGenericEvent[*gatewayv1.HTTPRoute],
	grpcRouteEventChan chan<- event.TypedGenericEvent[*gatewayv1.GRPCRoute], k8sClient client.Client, logger logr.Logger) handler.TypedEventHandler[*elbv2gw.ListenerRuleConfiguration, reconcile.Request] {
	return &enqueueRequestsForListenerRuleConfigurationEvent{
		httpRouteEventChan: httpRouteEventChan,
		grpcRouteEventChan: grpcRouteEventChan,
		k8sClient:          k8sClient,
		logger:             logger,
	}
}

var _ handler.TypedEventHandler[*elbv2gw.ListenerRuleConfiguration, reconcile.Request] = (*enqueueRequestsForListenerRuleConfigurationEvent)(nil)

// enqueueRequestsForListenerRuleConfigurationEvent handles ListenerRuleConfiguration events
type enqueueRequestsForListenerRuleConfigurationEvent struct {
	httpRouteEventChan chan<- event.TypedGenericEvent[*gatewayv1.HTTPRoute]
	grpcRouteEventChan chan<- event.TypedGenericEvent[*gatewayv1.GRPCRoute]
	k8sClient          client.Client
	logger             logr.Logger
}

func (h *enqueueRequestsForListenerRuleConfigurationEvent) Create(ctx context.Context, e event.TypedCreateEvent[*elbv2gw.ListenerRuleConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	ruleConfig := e.Object
	h.logger.V(1).Info("enqueue listenerruleconfiguration create event", "listenerruleconfiguration", ruleConfig.Name)
	h.enqueueImpactedRoutes(ctx, ruleConfig, queue)
}

func (h *enqueueRequestsForListenerRuleConfigurationEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*elbv2gw.ListenerRuleConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	ruleConfig := e.ObjectNew
	h.logger.V(1).Info("enqueue listenerruleconfiguration update event", "listenerruleconfiguration", ruleConfig.Name)
	h.enqueueImpactedRoutes(ctx, ruleConfig, queue)
}

func (h *enqueueRequestsForListenerRuleConfigurationEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*elbv2gw.ListenerRuleConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	ruleConfig := e.Object
	h.logger.V(1).Info("enqueue listenerruleconfiguration delete event", "listenerruleconfiguration", ruleConfig.Name)
	h.enqueueImpactedRoutes(ctx, ruleConfig, queue)
}

func (h *enqueueRequestsForListenerRuleConfigurationEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*elbv2gw.ListenerRuleConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	ruleConfig := e.Object
	h.logger.V(1).Info("enqueue listenerruleconfiguration generic event", "listenerruleconfiguration", ruleConfig.Name)
	h.enqueueImpactedRoutes(ctx, ruleConfig, queue)
}

func (h *enqueueRequestsForListenerRuleConfigurationEvent) enqueueImpactedRoutes(ctx context.Context, ruleConfig *elbv2gw.ListenerRuleConfiguration, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Find all L7 Routes that reference this ListenerRuleConfiguration via ExtensionRef

	l7Routes, err := routeutils.ListL7Routes(ctx, h.k8sClient)
	if err != nil {
		h.logger.V(1).Info("ignoring to enqueue impacted L7 routes ", "error: ", err)
	}
	filteredRoutesByListenerRuleCfg := routeutils.FilterRoutesByListenerRuleCfg(l7Routes, ruleConfig)
	for _, route := range filteredRoutesByListenerRuleCfg {
		routeType := route.GetRouteKind()
		switch routeType {
		case routeutils.HTTPRouteKind:
			h.logger.V(1).Info("enqueue httproute for listenerruleconfiguration event",
				"listenerruleconfiguration", ruleConfig.Name,
				"httproute", route.GetRouteNamespacedName())
			h.httpRouteEventChan <- event.TypedGenericEvent[*gatewayv1.HTTPRoute]{
				Object: route.GetRawRoute().(*gatewayv1.HTTPRoute),
			}
		case routeutils.GRPCRouteKind:
			h.logger.V(1).Info("enqueue grpcroute for listenerruleconfiguration event",
				"listenerruleconfiguration", ruleConfig.Name,
				"grpcroute", route.GetRouteNamespacedName())
			h.grpcRouteEventChan <- event.TypedGenericEvent[*gatewayv1.GRPCRoute]{
				Object: route.GetRawRoute().(*gatewayv1.GRPCRoute),
			}
		}
	}

}
