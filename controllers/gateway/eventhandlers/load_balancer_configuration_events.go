package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/gatewayutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// NewEnqueueRequestsForLoadBalancerConfigurationEvent creates handler for LoadBalancerConfiguration resources
func NewEnqueueRequestsForLoadBalancerConfigurationEvent(gwClassEventChan chan<- event.TypedGenericEvent[*gatewayv1.GatewayClass],
	k8sClient client.Client, eventRecorder record.EventRecorder, gwController string, logger logr.Logger) handler.TypedEventHandler[*elbv2gw.LoadBalancerConfiguration, reconcile.Request] {
	return &enqueueRequestsForLoadBalancerConfigurationEvent{
		gwClassEventChan: gwClassEventChan,
		k8sClient:        k8sClient,
		eventRecorder:    eventRecorder,
		gwController:     gwController,
		gwControllerSet:  sets.New(gwController),
		logger:           logger,
	}
}

var _ handler.TypedEventHandler[*elbv2gw.LoadBalancerConfiguration, reconcile.Request] = (*enqueueRequestsForLoadBalancerConfigurationEvent)(nil)

// enqueueRequestsForLoadBalancerConfigurationEvent handles LoadBalancerConfiguration events
type enqueueRequestsForLoadBalancerConfigurationEvent struct {
	gwClassEventChan chan<- event.TypedGenericEvent[*gatewayv1.GatewayClass]
	k8sClient        client.Client
	eventRecorder    record.EventRecorder
	gwController     string
	gwControllerSet  sets.Set[string]
	logger           logr.Logger
}

func (h *enqueueRequestsForLoadBalancerConfigurationEvent) Create(ctx context.Context, e event.TypedCreateEvent[*elbv2gw.LoadBalancerConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	lbconfigNew := e.Object
	h.logger.V(1).Info("enqueue loadbalancerconfiguration create event", "loadbalancerconfiguration", lbconfigNew.Name)
	h.enqueueImpactedGateways(ctx, lbconfigNew, queue)
}

func (h *enqueueRequestsForLoadBalancerConfigurationEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*elbv2gw.LoadBalancerConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	lbconfigNew := e.ObjectNew
	h.logger.V(1).Info("enqueue loadbalancerconfiguration update event", "loadbalancerconfiguration", lbconfigNew.Name)
	h.enqueueImpactedGateways(ctx, lbconfigNew, queue)
}

func (h *enqueueRequestsForLoadBalancerConfigurationEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*elbv2gw.LoadBalancerConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	lbconfig := e.Object
	h.logger.V(1).Info("enqueue loadbalancerconfiguration delete event", "loadbalancerconfiguration", lbconfig.Name)
	h.enqueueImpactedGateways(ctx, lbconfig, queue)
}

func (h *enqueueRequestsForLoadBalancerConfigurationEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*elbv2gw.LoadBalancerConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	lbconfig := e.Object
	h.logger.V(1).Info("enqueue loadbalancerconfiguration generic event", "loadbalancerconfiguration", lbconfig.Name)
	h.enqueueImpactedGateways(ctx, lbconfig, queue)
}

func (h *enqueueRequestsForLoadBalancerConfigurationEvent) enqueueImpactedGateways(ctx context.Context, lbconfig *elbv2gw.LoadBalancerConfiguration, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// NOTE: That LB Config changes for GatewayClass are done a little differently.
	// LB config change -> gateway class reconciler -> patch status for new version of LB config on Gateway Class -> Trigger the Gateway Class event handler.
	gateways, err := gatewayutils.GetImpactedGatewaysFromLbConfig(ctx, h.k8sClient, lbconfig, h.gwController)
	if err != nil {
		h.logger.Error(err, "failed to get impacted gateways from loadbalancerconfiguration", "loadbalancerconfiguration", k8s.NamespacedName(lbconfig))
		return
	}
	for _, gw := range gateways {
		h.logger.V(1).Info("enqueue gateway for loadbalancerconfiguration event",
			"loadbalancerconfiguration", k8s.NamespacedName(lbconfig),
			"gateway", k8s.NamespacedName(gw))
		queue.Add(reconcile.Request{NamespacedName: k8s.NamespacedName(gw)})
	}
}
