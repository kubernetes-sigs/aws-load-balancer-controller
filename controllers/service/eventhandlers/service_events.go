package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	svcpkg "sigs.k8s.io/aws-load-balancer-controller/pkg/service"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewEnqueueRequestForServiceEvent constructs new enqueueRequestsForServiceEvent.
func NewEnqueueRequestForServiceEvent(eventRecorder record.EventRecorder,
	serviceUtils svcpkg.ServiceUtils, logger logr.Logger) *enqueueRequestsForServiceEvent {
	return &enqueueRequestsForServiceEvent{
		eventRecorder: eventRecorder,
		serviceUtils:  serviceUtils,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForServiceEvent)(nil)

type enqueueRequestsForServiceEvent struct {
	eventRecorder record.EventRecorder
	serviceUtils  svcpkg.ServiceUtils
	logger        logr.Logger
}

func (h *enqueueRequestsForServiceEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueManagedService(ctx, queue, e.Object.(*corev1.Service))
}

func (h *enqueueRequestsForServiceEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldSvc := e.ObjectOld.(*corev1.Service)
	newSvc := e.ObjectNew.(*corev1.Service)

	if !equality.Semantic.DeepEqual(oldSvc.ResourceVersion, newSvc.ResourceVersion) {
		if equality.Semantic.DeepEqual(oldSvc.Annotations, newSvc.Annotations) &&
			equality.Semantic.DeepEqual(oldSvc.Spec, newSvc.Spec) &&
			equality.Semantic.DeepEqual(oldSvc.DeletionTimestamp.IsZero(), newSvc.DeletionTimestamp.IsZero()) {
			return
		}
	}

	h.enqueueManagedService(ctx, queue, newSvc)
}

func (h *enqueueRequestsForServiceEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// We attach a finalizer during reconcile, and handle the user triggered delete action during the update event.
	// In case of delete, there will first be an update event with nonzero deletionTimestamp set on the object. Since
	// deletion is already taken care of during update event, we will ignore this event.
}

func (h *enqueueRequestsForServiceEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForServiceEvent) enqueueManagedService(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], service *corev1.Service) {
	// Check if the svc needs to be handled
	if !h.serviceUtils.IsServicePendingFinalization(service) && !h.serviceUtils.IsServiceSupported(service) {
		return
	}
	queue.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: service.Namespace,
			Name:      service.Name,
		},
	})
}
