package eventhandlers

import (
	"context"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var logger = log.Log.WithName("eventhandlers").WithName("ingress")

func NewEnqueueRequestsForIngressEvent(groupLoader ingress.GroupLoader, eventRecorder record.EventRecorder) *enqueueRequestsForIngressEvent {
	return &enqueueRequestsForIngressEvent{groupLoader: groupLoader, eventRecorder: eventRecorder}
}

var _ handler.EventHandler = (*enqueueRequestsForIngressEvent)(nil)

type enqueueRequestsForIngressEvent struct {
	groupLoader   ingress.GroupLoader
	eventRecorder record.EventRecorder
}

func (h *enqueueRequestsForIngressEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfBelongsToGroup(queue, e.Object.(*networking.Ingress))
}

func (h *enqueueRequestsForIngressEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	ingOld := e.ObjectOld.(*networking.Ingress)
	ingNew := e.ObjectNew.(*networking.Ingress)

	// we only care three update event: 1. Ingress annotation updates 2. Ingress spec updates 3. Ingress deletion
	if equality.Semantic.DeepEqual(ingOld.Annotations, ingNew.Annotations) &&
		equality.Semantic.DeepEqual(ingOld.Spec, ingNew.Spec) &&
		equality.Semantic.DeepEqual(ingOld.DeletionTimestamp.IsZero(), ingNew.DeletionTimestamp.IsZero()) {
		logger.V(1).Info("ignoring unchanged Ingress Update event", "event", e)
		return
	}

	h.enqueueIfBelongsToGroup(queue, ingOld, ingNew)
}

func (h *enqueueRequestsForIngressEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	// since we'll always attach an finalizer before doing any reconcile action,
	// user triggered delete action will actually be an update action with deletionTimestamp set,
	// which will be handled by update event handler.
	// so we'll just ignore delete events to avoid unnecessary reconcile call.
}

func (h *enqueueRequestsForIngressEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {
}

func (h *enqueueRequestsForIngressEvent) enqueueIfBelongsToGroup(queue workqueue.RateLimitingInterface, ingList ...*networking.Ingress) {
	groupIDs := make(map[ingress.GroupID]struct{})
	for _, ing := range ingList {
		groupID, err := h.groupLoader.FindGroupID(context.Background(), ing)
		if err != nil {
			// TODO: define eventType, reason enums.
			h.eventRecorder.Eventf(ing, "Warning", "malformed Ingress", "failed to find group for Ingress due to %w", err)
			continue
		}
		if groupID == nil {
			logger.V(1).Info("ignoring Ingress", "Ingress", ing)
			continue
		}
		groupIDs[*groupID] = struct{}{}
	}

	for groupID := range groupIDs {
		queue.Add(ingress.EncodeGroupIDToReconcileRequest(groupID))
	}
}
