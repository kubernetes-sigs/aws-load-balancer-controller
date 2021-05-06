package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// NewEnqueueRequestsForIngressClassEvent constructs new enqueueRequestsForIngressClassEvent.
func NewEnqueueRequestsForIngressClassEvent(ingEventChan chan<- event.GenericEvent,
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) *enqueueRequestsForIngressClassEvent {
	return &enqueueRequestsForIngressClassEvent{
		ingEventChan:  ingEventChan,
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForIngressClassEvent)(nil)

type enqueueRequestsForIngressClassEvent struct {
	ingEventChan  chan<- event.GenericEvent
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForIngressClassEvent) Create(e event.CreateEvent, _ workqueue.RateLimitingInterface) {
	h.enqueueImpactedIngresses(e.Meta)
}

func (h *enqueueRequestsForIngressClassEvent) Update(e event.UpdateEvent, _ workqueue.RateLimitingInterface) {
	ingClassOld := e.ObjectOld.(*networking.IngressClass)
	ingClassNew := e.ObjectNew.(*networking.IngressClass)

	// we only care below update event:
	//	2. IngressClass spec updates
	//	3. IngressClass deletions
	if equality.Semantic.DeepEqual(ingClassOld.Spec, ingClassNew.Spec) &&
		equality.Semantic.DeepEqual(ingClassOld.DeletionTimestamp.IsZero(), ingClassNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueImpactedIngresses(e.MetaNew)
}

func (h *enqueueRequestsForIngressClassEvent) Delete(e event.DeleteEvent, _ workqueue.RateLimitingInterface) {
	h.enqueueImpactedIngresses(e.Meta)
}

func (h *enqueueRequestsForIngressClassEvent) Generic(e event.GenericEvent, _ workqueue.RateLimitingInterface) {
	h.enqueueImpactedIngresses(e.Meta)
}

func (h *enqueueRequestsForIngressClassEvent) enqueueImpactedIngresses(ingClass metav1.Object) {
	ingList := &networking.IngressList{}
	if err := h.k8sClient.List(context.Background(), ingList,
		client.MatchingFields{ingress.IndexKeyIngressClassRefName: ingClass.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch ingresses")
		return
	}

	for index := range ingList.Items {
		ing := &ingList.Items[index]
		meta, _ := meta.Accessor(ing)

		h.logger.V(1).Info("enqueue ingress for ingressClass event",
			"ingressClass", ingClass.GetName(),
			"ingress", k8s.NamespacedName(ing))
		h.ingEventChan <- event.GenericEvent{
			Meta:   meta,
			Object: ing,
		}
	}
}
