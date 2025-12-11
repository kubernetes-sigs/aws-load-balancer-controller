package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aga"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// NewEnqueueRequestsForResourceEvent creates a new handler for generic resource events
func NewEnqueueRequestsForResourceEvent(
	resourceType aga.ResourceType,
	referenceTracker *aga.ReferenceTracker,
	logger logr.Logger,
) handler.EventHandler {
	return &enqueueRequestsForResourceEvent{
		resourceType:     resourceType,
		referenceTracker: referenceTracker,
		logger:           logger,
	}
}

// enqueueRequestsForResourceEvent handles resource events and enqueues reconcile requests for GlobalAccelerators
// that reference the resource
type enqueueRequestsForResourceEvent struct {
	resourceType     aga.ResourceType
	referenceTracker *aga.ReferenceTracker
	logger           logr.Logger
}

// The following methods implement handler.TypedEventHandler interface

// Create handles Create events with the typed API
func (h *enqueueRequestsForResourceEvent) Create(ctx context.Context, evt event.TypedCreateEvent[client.Object], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.handleResource(ctx, evt.Object, "created", queue)
}

// Update handles Update events with the typed API
func (h *enqueueRequestsForResourceEvent) Update(ctx context.Context, evt event.TypedUpdateEvent[client.Object], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.handleResource(ctx, evt.ObjectNew, "updated", queue)
}

// Delete handles Delete events with the typed API
func (h *enqueueRequestsForResourceEvent) Delete(ctx context.Context, evt event.TypedDeleteEvent[client.Object], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.handleResource(ctx, evt.Object, "deleted", queue)
}

// Generic handles Generic events with the typed API
func (h *enqueueRequestsForResourceEvent) Generic(ctx context.Context, evt event.TypedGenericEvent[client.Object], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.handleResource(ctx, evt.Object, "generic event", queue)
}

// handleTypedResource handles resource events for the typed interface
func (h *enqueueRequestsForResourceEvent) handleResource(_ context.Context, obj interface{}, eventType string, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	var namespace, name string

	// Extract namespace and name based on the object type
	switch res := obj.(type) {
	case *corev1.Service:
		namespace = res.Namespace
		name = res.Name
	case *networking.Ingress:
		namespace = res.Namespace
		name = res.Name
	case *gwv1.Gateway:
		namespace = res.Namespace
		name = res.Name
	default:
		h.logger.Error(nil, "Unknown resource type", "type", h.resourceType)
		return
	}

	resourceKey := aga.ResourceKey{
		Type: h.resourceType,
		Name: types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
	}

	// If this resource is not referenced by any GA, no need to queue reconciles
	if !h.referenceTracker.IsResourceReferenced(resourceKey) {
		return
	}

	// Get all GAs that reference this resource
	gaRefs := h.referenceTracker.GetGAsForResource(resourceKey)

	// Queue reconcile for affected GAs
	for _, gaRef := range gaRefs {
		h.logger.V(1).Info("Enqueueing GA for reconcile due to resource event",
			"resourceType", h.resourceType,
			"resourceName", resourceKey.Name,
			"eventType", eventType,
			"ga", gaRef)

		queue.Add(reconcile.Request{NamespacedName: gaRef})
	}
}
