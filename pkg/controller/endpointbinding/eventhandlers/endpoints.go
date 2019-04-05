package eventhandlers

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"reflect"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logger = log.Log.WithName("eventhandlers").WithName("endpoints")

func NewEnqueueRequestsForEndpointsEvent(ebRepo backend.EndpointBindingRepo) handler.EventHandler {
	return &enqueueRequestsForEndpointsEvent{
		ebRepo: ebRepo,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForEndpointsEvent)(nil)

type enqueueRequestsForEndpointsEvent struct {
	ebRepo backend.EndpointBindingRepo
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *enqueueRequestsForEndpointsEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedEndpointBindings(e.Object.(*corev1.Endpoints), queue)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *enqueueRequestsForEndpointsEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	epOld := e.ObjectOld.(*corev1.Endpoints)
	epNew := e.ObjectNew.(*corev1.Endpoints)
	if !reflect.DeepEqual(epOld.Subsets, epNew.Subsets) {
		h.enqueueImpactedEndpointBindings(epNew, queue)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *enqueueRequestsForEndpointsEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedEndpointBindings(e.Object.(*corev1.Endpoints), queue)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *enqueueRequestsForEndpointsEvent) Generic(event.GenericEvent, workqueue.RateLimitingInterface) {
}

func (h *enqueueRequestsForEndpointsEvent) enqueueImpactedEndpointBindings(endpoints *corev1.Endpoints, queue workqueue.RateLimitingInterface) {
	svcKey := types.NamespacedName{
		Namespace: endpoints.Namespace,
		Name:      endpoints.Name,
	}.String()

	ebList, err := h.ebRepo.List(context.Background(), client.MatchingField(backend.EndpointBindingRepoIndexService, svcKey))
	if err != nil {
		logger.Error(err, "failed to fetch impacted endpointBindings by endpoints")
		return
	}

	for _, eb := range ebList.Items {
		queue.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: eb.Namespace,
				Name:      eb.Name,
			},
		})
	}
}
