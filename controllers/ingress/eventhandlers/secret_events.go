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

// NewEnqueueRequestsForSecretEvent constructs new enqueueRequestsForSecretEvent.
func NewEnqueueRequestsForSecretEvent(ingEventChan chan<- event.GenericEvent, svcEventChan chan<- event.GenericEvent,
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) *enqueueRequestsForSecretEvent {
	return &enqueueRequestsForSecretEvent{
		ingEventChan:  ingEventChan,
		svcEventChan:  svcEventChan,
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForSecretEvent)(nil)

type enqueueRequestsForSecretEvent struct {
	ingEventChan  chan<- event.GenericEvent
	svcEventChan  chan<- event.GenericEvent
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForSecretEvent) Create(e event.CreateEvent, _ workqueue.RateLimitingInterface) {
	secretNew := e.Object.(*corev1.Secret)
	h.enqueueImpactedObjects(secretNew)
}

func (h *enqueueRequestsForSecretEvent) Update(e event.UpdateEvent, _ workqueue.RateLimitingInterface) {
	secretOld := e.ObjectOld.(*corev1.Secret)
	secretNew := e.ObjectNew.(*corev1.Secret)

	// we only care below update event:
	//	1. Secret data updates
	//	2. Secret deletions
	if equality.Semantic.DeepEqual(secretOld.Data, secretNew.Data) &&
		equality.Semantic.DeepEqual(secretOld.DeletionTimestamp.IsZero(), secretNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueImpactedObjects(secretNew)
}

func (h *enqueueRequestsForSecretEvent) Delete(e event.DeleteEvent, _ workqueue.RateLimitingInterface) {
	secretOld := e.Object.(*corev1.Secret)
	h.enqueueImpactedObjects(secretOld)
}

func (h *enqueueRequestsForSecretEvent) Generic(e event.GenericEvent, _ workqueue.RateLimitingInterface) {
	secretObj := e.Object.(*corev1.Secret)
	h.enqueueImpactedObjects(secretObj)
}

func (h *enqueueRequestsForSecretEvent) enqueueImpactedObjects(secret *corev1.Secret) {
	secretKey := k8s.NamespacedName(secret)

	ingList := &networking.IngressList{}
	if err := h.k8sClient.List(context.Background(), ingList,
		client.InNamespace(secret.GetNamespace()),
		client.MatchingFields{ingress.IndexKeySecretRefName: secret.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch ingresses")
		return
	}
	for index := range ingList.Items {
		ing := &ingList.Items[index]

		h.logger.V(1).Info("enqueue ingress for secret event",
			"secret", secretKey,
			"ingress", k8s.NamespacedName(ing))
		h.ingEventChan <- event.GenericEvent{
			Object: ing,
		}
	}

	svcList := &corev1.ServiceList{}
	if err := h.k8sClient.List(context.Background(), svcList,
		client.InNamespace(secret.GetNamespace()),
		client.MatchingFields{ingress.IndexKeySecretRefName: secret.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch services")
		return
	}
	for index := range svcList.Items {
		svc := &svcList.Items[index]

		h.logger.V(1).Info("enqueue service for secret event",
			"secret", secretKey,
			"service", k8s.NamespacedName(svc))
		h.svcEventChan <- event.GenericEvent{
			Object: svc,
		}
	}
}
