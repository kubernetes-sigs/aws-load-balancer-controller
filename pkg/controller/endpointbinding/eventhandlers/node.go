package eventhandlers

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var nodeLogger = log.Log.WithName("eventhandlers").WithName("node")

func NewEnqueueRequestsForNodeEvent(ebRepo backend.EndpointBindingRepo, k8sCache cache.Cache) handler.EventHandler {
	return &enqueueRequestsForNodeEvent{
		ebRepo:   ebRepo,
		k8sCache: k8sCache,
	}
}

type enqueueRequestsForNodeEvent struct {
	ebRepo   backend.EndpointBindingRepo
	k8sCache cache.Cache
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *enqueueRequestsForNodeEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	node := e.Object.(*corev1.Node)
	if backend.IsNodeSuitableAsTrafficProxy(node) {
		h.enqueueImpactedEndpointBindings(queue)
	}
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *enqueueRequestsForNodeEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	nodeOld := e.ObjectOld.(*corev1.Node)
	nodeNew := e.ObjectNew.(*corev1.Node)
	if backend.IsNodeSuitableAsTrafficProxy(nodeOld) != backend.IsNodeSuitableAsTrafficProxy(nodeNew) {
		h.enqueueImpactedEndpointBindings(queue)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *enqueueRequestsForNodeEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	node := e.Object.(*corev1.Node)
	if backend.IsNodeSuitableAsTrafficProxy(node) {
		h.enqueueImpactedEndpointBindings(queue)
	}
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *enqueueRequestsForNodeEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedEndpointBindings(queue)
}

func (h *enqueueRequestsForNodeEvent) enqueueImpactedEndpointBindings(queue workqueue.RateLimitingInterface) {
	ebList, err := h.ebRepo.List(context.Background(), nil)
	if err != nil {
		logger.Error(err, "failed to fetch impacted endpointBindings")
		return
	}

	for _, eb := range ebList.Items {
		if eb.Spec.TargetType == v1alpha1.TargetTypeInstance {
			queue.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: eb.Namespace,
					Name:      eb.Name,
				},
			})
		}
	}
}
