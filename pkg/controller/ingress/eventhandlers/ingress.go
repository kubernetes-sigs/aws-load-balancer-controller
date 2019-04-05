package eventhandlers

import (
	"context"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logger = log.Log.WithName("eventhandlers").WithName("ingress")

func NewEnqueueRequestsForIngressEvent(ingGroupBuilder ingress.GroupBuilder, ingressClass string) handler.EventHandler {
	return &enqueueRequestsForIngressEvent{
		ingGroupBuilder: ingGroupBuilder,
		ingressClass:    ingressClass,
	}
}

type enqueueRequestsForIngressEvent struct {
	ingGroupBuilder ingress.GroupBuilder
	ingressClass    string
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *enqueueRequestsForIngressEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfIngressClassMatched(e.Object.(*extensions.Ingress), queue)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *enqueueRequestsForIngressEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfIngressClassMatched(e.ObjectOld.(*extensions.Ingress), queue)
	h.enqueueIfIngressClassMatched(e.ObjectNew.(*extensions.Ingress), queue)
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *enqueueRequestsForIngressEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfIngressClassMatched(e.Object.(*extensions.Ingress), queue)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *enqueueRequestsForIngressEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfIngressClassMatched(e.Object.(*extensions.Ingress), queue)
}

func (h *enqueueRequestsForIngressEvent) enqueueIfIngressClassMatched(ing *extensions.Ingress, queue workqueue.RateLimitingInterface) {
	if !ingress.MatchesIngressClass(h.ingressClass, ing) {
		return
	}

	groupID, err := h.ingGroupBuilder.BuildGroupID(context.Background(), ing)
	if err != nil {
		logger.Error(err, "failed to build ingress group ID", "ingress", k8s.NamespacedName(ing).String())
	}
	queue.Add(groupID.EncodeToReconcileRequest())
}
