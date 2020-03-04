package handlers

import (
	"context"
	"reflect"

	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/class"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = (*EnqueueRequestsForEndpointsEvent)(nil)

type EnqueueRequestsForEndpointsEvent struct {
	IngressClass string
	Cache        cache.Cache
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *EnqueueRequestsForEndpointsEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedIngresses(e.Object.(*corev1.Endpoints), queue)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *EnqueueRequestsForEndpointsEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	epOld := e.ObjectOld.(*corev1.Endpoints)
	epNew := e.ObjectNew.(*corev1.Endpoints)
	if !reflect.DeepEqual(epOld.Subsets, epNew.Subsets) {
		h.enqueueImpactedIngresses(epNew, queue)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *EnqueueRequestsForEndpointsEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedIngresses(e.Object.(*corev1.Endpoints), queue)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *EnqueueRequestsForEndpointsEvent) Generic(event.GenericEvent, workqueue.RateLimitingInterface) {
}

//TODO: this can be further optimized to only reconcile the target group referenced by the endpoints(service) :D
func (h *EnqueueRequestsForEndpointsEvent) enqueueImpactedIngresses(endpoints *corev1.Endpoints, queue workqueue.RateLimitingInterface) {
	ingressList := &extensions.IngressList{}
	if err := h.Cache.List(context.Background(), client.InNamespace(endpoints.Namespace), ingressList); err != nil {
		glog.Errorf("failed to fetch impacted ingresses by endpoints due to %v", err)
		return
	}

	for _, ingress := range ingressList.Items {
		if !class.IsValidIngress(h.IngressClass, &ingress) {
			continue
		}

		backends, err := tg.ExtractTargetGroupBackends(&ingress)
		if err != nil {
			glog.Errorf("Failed to extract backend services from ingress: %v, reconcile the ingress. error: %e", ingress.Name, err)
			queue.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: ingress.Namespace,
					Name:      ingress.Name,
				},
			})
			break
		}

		for _, backend := range backends {
			if backend.ServiceName == endpoints.Name {
				queue.Add(reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: ingress.Namespace,
						Name:      ingress.Name,
					},
				})
				break
			}
		}
	}
}
