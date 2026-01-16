package eventhandlers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/client-go/util/workqueue"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// NewEnqueueRequestsForReferenceGrantEvent creates handler for ReferenceGrant resources
func NewEnqueueRequestsForReferenceGrantEvent(
	k8sClient client.Client,
	logger logr.Logger,
) handler.TypedEventHandler[*gwbeta1.ReferenceGrant, reconcile.Request] {
	return &enqueueRequestsForReferenceGrantEvent{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

var _ handler.TypedEventHandler[*gwbeta1.ReferenceGrant, reconcile.Request] = (*enqueueRequestsForReferenceGrantEvent)(nil)

// enqueueRequestsForReferenceGrantEvent handles ReferenceGrant events
type enqueueRequestsForReferenceGrantEvent struct {
	k8sClient client.Client
	logger    logr.Logger
}

func (h *enqueueRequestsForReferenceGrantEvent) Create(ctx context.Context, e event.TypedCreateEvent[*gwbeta1.ReferenceGrant], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	refGrant := e.Object
	h.logger.V(1).Info("enqueue reference grant create event", "reference grant", refGrant.Name)
	h.enqueueImpactedGlobalAccelerators(ctx, refGrant, nil, queue)
}

func (h *enqueueRequestsForReferenceGrantEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*gwbeta1.ReferenceGrant], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	refGrantNew := e.ObjectNew
	refGrantOld := e.ObjectOld
	h.logger.V(1).Info("enqueue reference grant update event", "reference grant", refGrantNew.Name)
	h.enqueueImpactedGlobalAccelerators(ctx, refGrantNew, refGrantOld, queue)
}

func (h *enqueueRequestsForReferenceGrantEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*gwbeta1.ReferenceGrant], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	refGrant := e.Object
	h.logger.V(1).Info("enqueue reference grant delete event", "reference grant", refGrant.Name)
	h.enqueueImpactedGlobalAccelerators(ctx, refGrant, nil, queue)
}

func (h *enqueueRequestsForReferenceGrantEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*gwbeta1.ReferenceGrant], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	refGrant := e.Object
	h.logger.V(1).Info("enqueue reference grant generic event", "reference grant", refGrant.Name)
	h.enqueueImpactedGlobalAccelerators(ctx, refGrant, nil, queue)
}

// enqueueImpactedGlobalAccelerators finds and enqueues GlobalAccelerators impacted by a ReferenceGrant change
func (h *enqueueRequestsForReferenceGrantEvent) enqueueImpactedGlobalAccelerators(
	ctx context.Context,
	newRefGrant *gwbeta1.ReferenceGrant,
	oldRefGrant *gwbeta1.ReferenceGrant,
	queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {

	// Collect all relevant namespaces from both old and new ReferenceGrant
	impactedFroms := make(map[string]gwbeta1.ReferenceGrantFrom)

	// Process new reference grant
	for i, from := range newRefGrant.Spec.From {
		if from.Group == shared_constants.GlobalAcceleratorResourcesGroup && from.Kind == shared_constants.GlobalAcceleratorKind {
			key := generateGrantFromKey(from)
			impactedFroms[key] = newRefGrant.Spec.From[i]
		}
	}

	// Also process old reference grant if it exists (for updates)
	if oldRefGrant != nil {
		for i, from := range oldRefGrant.Spec.From {
			if from.Group == shared_constants.GlobalAcceleratorResourcesGroup && from.Kind == shared_constants.GlobalAcceleratorKind {
				key := generateGrantFromKey(from)
				impactedFroms[key] = oldRefGrant.Spec.From[i]
			}
		}
	}

	// If no GlobalAccelerator references found, nothing to do
	if len(impactedFroms) == 0 {
		h.logger.V(1).Info("ReferenceGrant doesn't reference GlobalAccelerators, skipping",
			"referenceGrant", k8s.NamespacedName(newRefGrant))
		return
	}

	totalMatched := 0

	// Process each impacted GlobalAccelerator namespace
	for _, from := range impactedFroms {

		var gaList agaapi.GlobalAcceleratorList
		if err := h.k8sClient.List(ctx, &gaList, &client.ListOptions{Namespace: string(from.Namespace)}); err != nil {
			h.logger.Error(err, "Failed to list GlobalAccelerators for ReferenceGrant",
				"referenceGrant", k8s.NamespacedName(newRefGrant),
				"from namespace", from.Namespace)
			continue
		}

		// Check each GA to see if it references resources in the target namespace
		for i := range gaList.Items {
			ga := &gaList.Items[i]

			// Only check GAs that reference resources in the ReferenceGrant's namespace
			hasRelevantCrossNamespaceRef := h.hasCrossNamespaceReferences(
				ga,
				newRefGrant.Namespace,
			)

			if hasRelevantCrossNamespaceRef {
				totalMatched++

				// Enqueue reconcile request for this GA
				request := reconcile.Request{
					NamespacedName: k8s.NamespacedName(ga),
				}

				h.logger.V(1).Info("Enqueueing GlobalAccelerator for reconcile due to ReferenceGrant change",
					"globalAccelerator", request.NamespacedName,
					"referenceGrant", k8s.NamespacedName(newRefGrant))

				queue.Add(request)
			}
		}
	}

	h.logger.V(1).Info("ReferenceGrant event processing completed",
		"referenceGrant", k8s.NamespacedName(newRefGrant),
		"totalMatchedGAs", totalMatched)
}

// hasCrossNamespaceReferences checks if a GlobalAccelerator has cross-namespace references to resources in the target namespace
func (h *enqueueRequestsForReferenceGrantEvent) hasCrossNamespaceReferences(
	ga *agaapi.GlobalAccelerator,
	targetNamespace string) bool {

	// Go through all endpoints in the GA spec
	if ga.Spec.Listeners == nil {
		return false
	}

	for _, listener := range *ga.Spec.Listeners {
		if listener.EndpointGroups == nil {
			continue
		}

		for _, endpointGroup := range *listener.EndpointGroups {
			if endpointGroup.Endpoints == nil {
				continue
			}

			for _, endpoint := range *endpointGroup.Endpoints {
				// Check for cross-namespace references
				if endpoint.Namespace != nil && *endpoint.Namespace == targetNamespace && *endpoint.Namespace != ga.Namespace {
					return true // Found a cross-namespace reference to target namespace
				}
			}
		}
	}

	return false
}

// generateGrantFromKey creates a unique key for a ReferenceGrantFrom
func generateGrantFromKey(from gwbeta1.ReferenceGrantFrom) string {
	return fmt.Sprintf("%s-%s", from.Kind, from.Namespace)
}
