package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/gatewayutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// NewEnqueueRequestsForListenerSetEvent creates a handler that watches
// ListenerSet events and enqueues the parent Gateway for reconciliation.
func NewEnqueueRequestsForListenerSetEvent(
	k8sClient client.Client,
	eventRecorder record.EventRecorder,
	gwController string,
	logger logr.Logger,
) handler.TypedEventHandler[*gwv1.ListenerSet, reconcile.Request] {
	return &enqueueRequestsForListenerSetEvent{
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		gwController:  gwController,
		logger:        logger,
	}
}

var _ handler.TypedEventHandler[*gwv1.ListenerSet, reconcile.Request] = (*enqueueRequestsForListenerSetEvent)(nil)

// enqueueRequestsForListenerSetEvent handles ListenerSet events by resolving
// the parent Gateway and enqueuing it for reconciliation.
type enqueueRequestsForListenerSetEvent struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	gwController  string
	logger        logr.Logger
}

func (h *enqueueRequestsForListenerSetEvent) Create(ctx context.Context, e event.TypedCreateEvent[*gwv1.ListenerSet], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	ls := e.Object
	h.logger.V(1).Info("enqueue listenerset create event", "listenerset", k8s.NamespacedName(ls))
	h.enqueueParentGateway(ctx, ls, queue)
}

func (h *enqueueRequestsForListenerSetEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*gwv1.ListenerSet], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	lsOld := e.ObjectOld
	lsNew := e.ObjectNew
	h.logger.V(1).Info("enqueue listenerset update event", "listenerset", k8s.NamespacedName(lsNew))
	h.enqueueParentGateway(ctx, lsOld, queue)
	h.enqueueParentGateway(ctx, lsNew, queue)
}

func (h *enqueueRequestsForListenerSetEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*gwv1.ListenerSet], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	ls := e.Object
	h.logger.V(1).Info("enqueue listenerset delete event", "listenerset", k8s.NamespacedName(ls))
	h.enqueueParentGateway(ctx, ls, queue)
}

func (h *enqueueRequestsForListenerSetEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*gwv1.ListenerSet], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	ls := e.Object
	h.logger.V(1).Info("enqueue listenerset generic event", "listenerset", k8s.NamespacedName(ls))
	h.enqueueParentGateway(ctx, ls, queue)
}

func (h *enqueueRequestsForListenerSetEvent) enqueueParentGateway(ctx context.Context, ls *gwv1.ListenerSet, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	if ls == nil {
		return
	}

	// Resolve the parent Gateway namespace: use parentRef.Namespace if set,
	// otherwise default to the ListenerSet's own namespace.
	gwNamespace := ls.Namespace
	if ls.Spec.ParentRef.Namespace != nil {
		gwNamespace = string(*ls.Spec.ParentRef.Namespace)
	}
	gwName := string(ls.Spec.ParentRef.Name)

	// Fetch the parent Gateway.
	gw := &gwv1.Gateway{}
	if err := h.k8sClient.Get(ctx, types.NamespacedName{Name: gwName, Namespace: gwNamespace}, gw); err != nil {
		h.logger.V(1).Info("failed to get parent gateway for listenerset",
			"listenerset", k8s.NamespacedName(ls),
			"gateway", types.NamespacedName{Name: gwName, Namespace: gwNamespace},
			"error", err)
		return
	}

	// Only enqueue if the Gateway is managed by this controller instance.
	if !gatewayutils.IsGatewayManagedByLBController(ctx, h.k8sClient, gw, h.gwController) {
		return
	}

	h.logger.V(1).Info("enqueue gateway for listenerset event",
		"listenerset", k8s.NamespacedName(ls),
		"gateway", k8s.NamespacedName(gw))
	queue.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}})
}
