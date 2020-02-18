package auth

import (
	"context"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

var _ handler.EventHandler = (*EnqueueRequestsForSecretEvent)(nil)

type EnqueueRequestsForSecretEvent struct {
	Cache       cache.Cache
	IngressChan chan<- event.GenericEvent
	ServiceChan chan<- event.GenericEvent
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *EnqueueRequestsForSecretEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedObjects(e.Object.(*corev1.Secret), queue)
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *EnqueueRequestsForSecretEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedObjects(e.Object.(*corev1.Secret), queue)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *EnqueueRequestsForSecretEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedObjects(e.ObjectNew.(*corev1.Secret), queue)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *EnqueueRequestsForSecretEvent) Generic(event.GenericEvent, workqueue.RateLimitingInterface) {
}

func (h *EnqueueRequestsForSecretEvent) enqueueImpactedObjects(secret *corev1.Secret, _ workqueue.RateLimitingInterface) {
	secretKey := types.NamespacedName{
		Namespace: secret.Namespace,
		Name:      secret.Name,
	}.String()

	ingressList := &networking.IngressList{}
	if err := h.Cache.List(context.TODO(), client.MatchingField(FieldAuthOIDCSecret, secretKey), ingressList); err != nil {
		glog.Errorf("failed to fetch impacted ingresses by %v due to %v", FieldAuthOIDCSecret, err)
		return
	}
	for index := range ingressList.Items {
		meta, _ := meta.Accessor(&ingressList.Items[index])
		h.IngressChan <- event.GenericEvent{
			Meta:   meta,
			Object: &ingressList.Items[index],
		}
	}

	serviceList := &corev1.ServiceList{}
	if err := h.Cache.List(context.TODO(), client.MatchingField(FieldAuthOIDCSecret, secretKey), serviceList); err != nil {
		glog.Errorf("failed to fetch impacted services by %v due to %v", FieldAuthOIDCSecret, err)
		return
	}
	for index := range serviceList.Items {
		meta, _ := meta.Accessor(&serviceList.Items[index])
		h.ServiceChan <- event.GenericEvent{
			Meta:   meta,
			Object: &serviceList.Items[index],
		}
	}
}
