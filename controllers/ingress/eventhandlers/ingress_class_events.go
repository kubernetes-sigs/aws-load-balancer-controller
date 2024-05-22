package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// NewEnqueueRequestsForIngressClassEvent constructs new enqueueRequestsForIngressClassEvent.
func NewEnqueueRequestsForIngressClassEvent(ingEventChan chan<- event.TypedGenericEvent[*networking.Ingress],
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*networking.IngressClass] {
	return &enqueueRequestsForIngressClassEvent{
		ingEventChan:  ingEventChan,
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.TypedEventHandler[*networking.IngressClass] = (*enqueueRequestsForIngressClassEvent)(nil)

type enqueueRequestsForIngressClassEvent struct {
	ingEventChan  chan<- event.TypedGenericEvent[*networking.Ingress]
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForIngressClassEvent) Create(ctx context.Context, e event.TypedCreateEvent[*networking.IngressClass], _ workqueue.RateLimitingInterface) {
	ingClassNew := e.Object
	h.enqueueImpactedIngresses(ingClassNew)
}

func (h *enqueueRequestsForIngressClassEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*networking.IngressClass], _ workqueue.RateLimitingInterface) {
	ingClassOld := e.ObjectOld
	ingClassNew := e.ObjectNew

	// we only care below update event:
	//	2. IngressClass spec updates
	//	3. IngressClass deletions
	if equality.Semantic.DeepEqual(ingClassOld.Spec, ingClassNew.Spec) &&
		equality.Semantic.DeepEqual(ingClassOld.DeletionTimestamp.IsZero(), ingClassNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueImpactedIngresses(ingClassNew)
}

func (h *enqueueRequestsForIngressClassEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*networking.IngressClass], _ workqueue.RateLimitingInterface) {
	ingClassOld := e.Object
	h.enqueueImpactedIngresses(ingClassOld)
}

func (h *enqueueRequestsForIngressClassEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*networking.IngressClass], _ workqueue.RateLimitingInterface) {
	ingClass := e.Object
	h.enqueueImpactedIngresses(ingClass)
}

func (h *enqueueRequestsForIngressClassEvent) enqueueImpactedIngresses(ingClass *networking.IngressClass) {
	ingList := &networking.IngressList{}
	if err := h.k8sClient.List(context.Background(), ingList,
		client.MatchingFields{ingress.IndexKeyIngressClassRefName: ingClass.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch ingresses")
		return
	}

	for index := range ingList.Items {
		ing := &ingList.Items[index]

		h.logger.V(1).Info("enqueue ingress for ingressClass event",
			"ingressClass", ingClass.GetName(),
			"ingress", k8s.NamespacedName(ing))
		h.ingEventChan <- event.TypedGenericEvent[*networking.Ingress]{
			Object: ing,
		}
	}
}
