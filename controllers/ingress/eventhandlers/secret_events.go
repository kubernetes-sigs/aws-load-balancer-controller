package eventhandlers

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
func NewEnqueueRequestsForSecretEvent(ingEventChan chan<- event.TypedGenericEvent[*networking.Ingress], svcEventChan chan<- event.TypedGenericEvent[*corev1.Service],
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*corev1.Secret, reconcile.Request] {
	return &enqueueRequestsForSecretEvent{
		ingEventChan:  ingEventChan,
		svcEventChan:  svcEventChan,
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.TypedEventHandler[*corev1.Secret, reconcile.Request] = (*enqueueRequestsForSecretEvent)(nil)

type enqueueRequestsForSecretEvent struct {
	ingEventChan  chan<- event.TypedGenericEvent[*networking.Ingress]
	svcEventChan  chan<- event.TypedGenericEvent[*corev1.Service]
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForSecretEvent) Create(ctx context.Context, e event.TypedCreateEvent[*corev1.Secret], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	secretNew := e.Object
	h.enqueueImpactedObjects(ctx, secretNew)
}

func (h *enqueueRequestsForSecretEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*corev1.Secret], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	secretOld := e.ObjectOld
	secretNew := e.ObjectNew

	// we only care below update event:
	//	1. Secret data updates
	//	2. Secret deletions
	if equality.Semantic.DeepEqual(secretOld.Data, secretNew.Data) &&
		equality.Semantic.DeepEqual(secretOld.DeletionTimestamp.IsZero(), secretNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueImpactedObjects(ctx, secretNew)
}

func (h *enqueueRequestsForSecretEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*corev1.Secret], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	secretOld := e.Object
	h.enqueueImpactedObjects(ctx, secretOld)
}

func (h *enqueueRequestsForSecretEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*corev1.Secret], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	secretObj := e.Object
	h.enqueueImpactedObjects(ctx, secretObj)
}

func (h *enqueueRequestsForSecretEvent) enqueueImpactedObjects(ctx context.Context, secret *corev1.Secret) {
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
		h.ingEventChan <- event.TypedGenericEvent[*networking.Ingress]{
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
		h.svcEventChan <- event.TypedGenericEvent[*corev1.Service]{
			Object: svc,
		}
	}
}
