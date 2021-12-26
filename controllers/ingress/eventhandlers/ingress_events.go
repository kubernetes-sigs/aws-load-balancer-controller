package eventhandlers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

func NewEnqueueRequestsForIngressEvent(groupLoader ingress.GroupLoader, eventRecorder record.EventRecorder,
	logger logr.Logger) *enqueueRequestsForIngressEvent {
	return &enqueueRequestsForIngressEvent{
		groupLoader:   groupLoader,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForIngressEvent)(nil)

type enqueueRequestsForIngressEvent struct {
	groupLoader   ingress.GroupLoader
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForIngressEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfBelongsToGroup(queue, e.Object.(*networking.Ingress))
}

func (h *enqueueRequestsForIngressEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	ingOld := e.ObjectOld.(*networking.Ingress)
	ingNew := e.ObjectNew.(*networking.Ingress)

	// we only care below update event:
	//	1. Ingress annotation updates
	//	2. Ingress spec updates
	//	3. Ingress deletion
	if equality.Semantic.DeepEqual(ingOld.Annotations, ingNew.Annotations) &&
		equality.Semantic.DeepEqual(ingOld.Spec, ingNew.Spec) &&
		equality.Semantic.DeepEqual(ingOld.DeletionTimestamp.IsZero(), ingNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueIfBelongsToGroup(queue, ingNew)
}

func (h *enqueueRequestsForIngressEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	// since we'll always attach an finalizer before doing any reconcile action,
	// user triggered delete action will actually be an update action with deletionTimestamp set,
	// which will be handled by update event handler.
	// so we'll just ignore delete events to avoid unnecessary reconcile call.
}

func (h *enqueueRequestsForIngressEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfBelongsToGroup(queue, e.Object.(*networking.Ingress))
}

func (h *enqueueRequestsForIngressEvent) enqueueIfBelongsToGroup(queue workqueue.RateLimitingInterface, ing *networking.Ingress) {
	ctx := context.Background()
	ingKey := k8s.NamespacedName(ing)
	groupIDsSet := make(map[ingress.GroupID]struct{})

	groupIDsPendingFinalization := h.groupLoader.LoadGroupIDsPendingFinalization(ctx, ing)
	for _, groupID := range groupIDsPendingFinalization {
		groupIDsSet[groupID] = struct{}{}
	}

	if groupID, err := h.groupLoader.LoadGroupIDIfAny(ctx, ing); err != nil {
		h.eventRecorder.Event(ing, corev1.EventTypeWarning, k8s.IngressEventReasonFailedLoadGroupID, fmt.Sprintf("failed load groupID due to %v", err))
	} else if groupID != nil {
		groupIDsSet[*groupID] = struct{}{}
	}

	for groupID, _ := range groupIDsSet {
		h.logger.V(1).Info("enqueue ingressGroup for ingress event",
			"ingress", ingKey.String(),
			"ingressGroup", groupID,
		)
		queue.Add(ingress.EncodeGroupIDToReconcileRequest(groupID))
	}
}
