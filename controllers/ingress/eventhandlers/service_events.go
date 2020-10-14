package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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

// NewEnqueueRequestsForServiceEvent constructs new enqueueRequestsForServiceEvent.
func NewEnqueueRequestsForServiceEvent(ingEventChan chan<- event.GenericEvent,
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) *enqueueRequestsForServiceEvent {
	return &enqueueRequestsForServiceEvent{
		ingEventChan:  ingEventChan,
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForServiceEvent)(nil)

type enqueueRequestsForServiceEvent struct {
	ingEventChan  chan<- event.GenericEvent
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForServiceEvent) Create(e event.CreateEvent, _ workqueue.RateLimitingInterface) {
	h.enqueueImpactedIngresses(e.Meta)
}

func (h *enqueueRequestsForServiceEvent) Update(e event.UpdateEvent, _ workqueue.RateLimitingInterface) {
	svcOld := e.ObjectOld.(*corev1.Service)
	svcNew := e.ObjectNew.(*corev1.Service)

	// we only care below update event:
	//	1. Service annotation updates
	//	2. Service spec updates
	//	3. Service deletions
	if equality.Semantic.DeepEqual(svcOld.Annotations, svcNew.Annotations) &&
		equality.Semantic.DeepEqual(svcOld.Spec, svcNew.Spec) &&
		equality.Semantic.DeepEqual(svcOld.DeletionTimestamp.IsZero(), svcNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueImpactedIngresses(e.MetaNew)
}

func (h *enqueueRequestsForServiceEvent) Delete(e event.DeleteEvent, _ workqueue.RateLimitingInterface) {
	h.enqueueImpactedIngresses(e.Meta)
}

func (h *enqueueRequestsForServiceEvent) Generic(e event.GenericEvent, _ workqueue.RateLimitingInterface) {
	h.enqueueImpactedIngresses(e.Meta)
}

func (h *enqueueRequestsForServiceEvent) enqueueImpactedIngresses(svc metav1.Object) {
	ingList := &networking.IngressList{}
	if err := h.k8sClient.List(context.Background(), ingList,
		client.InNamespace(svc.GetNamespace()),
		client.MatchingFields{ingress.IndexKeyServiceRefName: svc.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch ingresses")
		return
	}

	svcKey := k8s.NamespacedName(svc)
	for index := range ingList.Items {
		ing := &ingList.Items[index]
		meta, _ := meta.Accessor(ing)

		h.logger.V(1).Info("enqueue ingress for service event",
			"service", svcKey,
			"ingress", k8s.NamespacedName(ing))
		h.ingEventChan <- event.GenericEvent{
			Meta:   meta,
			Object: ing,
		}
	}
}
