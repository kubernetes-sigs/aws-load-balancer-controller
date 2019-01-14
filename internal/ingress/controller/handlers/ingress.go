package handlers

import (
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/class"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = (*EnqueueRequestsForIngressEvent)(nil)

type EnqueueRequestsForIngressEvent struct {
	IngressClass string
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *EnqueueRequestsForIngressEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfIngressClassMatched(e.Object.(*extensions.Ingress), queue)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *EnqueueRequestsForIngressEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfIngressClassMatched(e.ObjectOld.(*extensions.Ingress), queue)
	h.enqueueIfIngressClassMatched(e.ObjectNew.(*extensions.Ingress), queue)
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *EnqueueRequestsForIngressEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfIngressClassMatched(e.Object.(*extensions.Ingress), queue)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *EnqueueRequestsForIngressEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfIngressClassMatched(e.Object.(*extensions.Ingress), queue)
}

func (h *EnqueueRequestsForIngressEvent) enqueueIfIngressClassMatched(ingress *extensions.Ingress, queue workqueue.RateLimitingInterface) {
	if !class.IsValidIngress(h.IngressClass, ingress) {
		return
	}
	queue.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: ingress.Namespace,
			Name:      ingress.Name,
		},
	})
}
