package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// NewEnqueueRequestsForTargetGroupConfigurationEvent creates handler for TargetGroupConfiguration resources
func NewEnqueueRequestsForTargetGroupConfigurationEvent(svcEventChan chan<- event.TypedGenericEvent[*corev1.Service], tcpRouteEventChan chan<- event.TypedGenericEvent[*gwalpha2.TCPRoute],
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*elbv2gw.TargetGroupConfiguration, reconcile.Request] {
	return &enqueueRequestsForTargetGroupConfigurationEvent{
		svcEventChan:      svcEventChan,
		tcpRouteEventChan: tcpRouteEventChan,
		k8sClient:         k8sClient,
		eventRecorder:     eventRecorder,
		logger:            logger,
	}
}

var _ handler.TypedEventHandler[*elbv2gw.TargetGroupConfiguration, reconcile.Request] = (*enqueueRequestsForTargetGroupConfigurationEvent)(nil)

// enqueueRequestsForTargetGroupConfigurationEvent handles TargetGroupConfiguration events
type enqueueRequestsForTargetGroupConfigurationEvent struct {
	svcEventChan      chan<- event.TypedGenericEvent[*corev1.Service]
	tcpRouteEventChan chan<- event.TypedGenericEvent[*gwalpha2.TCPRoute]
	k8sClient         client.Client
	eventRecorder     record.EventRecorder
	logger            logr.Logger
}

func (h *enqueueRequestsForTargetGroupConfigurationEvent) Create(ctx context.Context, e event.TypedCreateEvent[*elbv2gw.TargetGroupConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	tgconfigNew := e.Object
	h.logger.V(1).Info("enqueue targetgroupconfiguration create event", "targetgroupconfiguration", tgconfigNew.Name)
	h.enqueueImpactedObject(ctx, tgconfigNew, queue)
}

func (h *enqueueRequestsForTargetGroupConfigurationEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*elbv2gw.TargetGroupConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	tgconfigNew := e.ObjectNew
	h.logger.V(1).Info("enqueue targetgroupconfiguration update event", "targetgroupconfiguration", tgconfigNew.Name)
	h.enqueueImpactedObject(ctx, tgconfigNew, queue)
}

func (h *enqueueRequestsForTargetGroupConfigurationEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*elbv2gw.TargetGroupConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	tgconfig := e.Object
	h.logger.V(1).Info("enqueue targetgroupconfiguration delete event", "targetgroupconfiguration", tgconfig.Name)
	h.enqueueImpactedObject(ctx, tgconfig, queue)
}

func (h *enqueueRequestsForTargetGroupConfigurationEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*elbv2gw.TargetGroupConfiguration], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	tgconfig := e.Object
	h.logger.V(1).Info("enqueue targetgroupconfiguration generic event", "targetgroupconfiguration", tgconfig.Name)
	h.enqueueImpactedObject(ctx, tgconfig, queue)
}

func (h *enqueueRequestsForTargetGroupConfigurationEvent) enqueueImpactedObject(ctx context.Context, tgconfig *elbv2gw.TargetGroupConfiguration, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	objName := types.NamespacedName{Namespace: tgconfig.Namespace, Name: tgconfig.Spec.TargetReference.Name}

	if tgconfig.Spec.TargetReference.Kind == nil || *tgconfig.Spec.TargetReference.Kind == "Service" {
		svc := &corev1.Service{}
		if err := h.k8sClient.Get(ctx, objName, svc); err != nil {
			h.logger.V(1).Info("ignoring targetgroupconfiguration event for unknown service",
				"targetgroupconfiguration", k8s.NamespacedName(tgconfig),
				"service", k8s.NamespacedName(svc))
			return
		}
		h.logger.V(1).Info("enqueue service for targetgroupconfiguration event",
			"targetgroupconfiguration", k8s.NamespacedName(tgconfig),
			"service", k8s.NamespacedName(svc))
		h.svcEventChan <- event.TypedGenericEvent[*corev1.Service]{
			Object: svc,
		}
	}

	// TODO - We should probably use an indexer here, we have a task to do this.
	if tgconfig.Spec.TargetReference.Kind != nil && *tgconfig.Spec.TargetReference.Kind == "Gateway" && h.tcpRouteEventChan != nil {
		tcpRouteList := &gwalpha2.TCPRouteList{}

		if err := h.k8sClient.List(ctx, tcpRouteList); err != nil {
			h.logger.V(1).Info("failed to list tcp routes for target group configuration event", "targetgroupconfiguration", k8s.NamespacedName(tgconfig))
			return
		}

		impactedRoutes := getImpactedTCPRoutes(tcpRouteList, tgconfig)
		for i := range impactedRoutes {
			h.tcpRouteEventChan <- event.TypedGenericEvent[*gwalpha2.TCPRoute]{
				Object: impactedRoutes[i],
			}
		}

	}
}

func getImpactedTCPRoutes(list *gwalpha2.TCPRouteList, tgconfig *elbv2gw.TargetGroupConfiguration) []*gwalpha2.TCPRoute {
	seen := sets.Set[types.NamespacedName]{}
	res := make([]*gwalpha2.TCPRoute, 0)

	for i, route := range list.Items {
		nsn := k8s.NamespacedName(&route)
		for _, rule := range route.Spec.Rules {
			for _, beRef := range rule.BackendRefs {
				if beRef.Kind != nil && *beRef.Kind == "Gateway" {
					if string(beRef.Name) == tgconfig.Spec.TargetReference.Name {

						// The route backend ns
						var routeNs string
						if beRef.Namespace == nil {
							routeNs = route.Namespace
						} else {
							routeNs = string(*beRef.Namespace)
						}

						if routeNs == tgconfig.Namespace && !seen.Has(nsn) {
							res = append(res, &list.Items[i])
							seen.Insert(nsn)
						}

					}
				}
			}
		}
	}
	return res
}
