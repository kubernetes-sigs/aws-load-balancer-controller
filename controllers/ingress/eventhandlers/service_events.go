package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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

// NewEnqueueRequestsForServiceEvent constructs new enqueueRequestsForServiceEvent.
func NewEnqueueRequestsForServiceEvent(ingEventChan chan<- event.TypedGenericEvent[*networking.Ingress],
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*corev1.Service] {
	return &enqueueRequestsForServiceEvent{
		ingEventChan:  ingEventChan,
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.TypedEventHandler[*corev1.Service] = (*enqueueRequestsForServiceEvent)(nil)

type enqueueRequestsForServiceEvent struct {
	ingEventChan  chan<- event.TypedGenericEvent[*networking.Ingress]
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForServiceEvent) Create(ctx context.Context, e event.TypedCreateEvent[*corev1.Service], _ workqueue.RateLimitingInterface) {
	svcNew := e.Object
	h.enqueueImpactedIngresses(ctx, svcNew)
}

func (h *enqueueRequestsForServiceEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*corev1.Service], _ workqueue.RateLimitingInterface) {
	svcOld := e.ObjectOld
	svcNew := e.ObjectNew

	// we only care below update event:
	//	1. Service annotation updates
	//	2. Service spec updates
	//	3. Service deletions
	if equality.Semantic.DeepEqual(svcOld.Annotations, svcNew.Annotations) &&
		equality.Semantic.DeepEqual(svcOld.Spec, svcNew.Spec) &&
		equality.Semantic.DeepEqual(svcOld.DeletionTimestamp.IsZero(), svcNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueImpactedIngresses(ctx, svcNew)
}

func (h *enqueueRequestsForServiceEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*corev1.Service], _ workqueue.RateLimitingInterface) {
	svcOld := e.Object
	h.enqueueImpactedIngresses(ctx, svcOld)
}

func (h *enqueueRequestsForServiceEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*corev1.Service], _ workqueue.RateLimitingInterface) {
	svc := e.Object
	h.enqueueImpactedIngresses(ctx, svc)
}

func (h *enqueueRequestsForServiceEvent) enqueueImpactedIngresses(ctx context.Context, svc *corev1.Service) {
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

		h.logger.V(1).Info("enqueue ingress for service event",
			"service", svcKey,
			"ingress", k8s.NamespacedName(ing))
		h.ingEventChan <- event.TypedGenericEvent[*networking.Ingress]{
			Object: ing,
		}
	}
}
