package handlers

import (
	"context"

	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/class"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/backend"
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

var _ handler.EventHandler = (*EnqueueRequestsForPodsEvent)(nil)

type EnqueueRequestsForPodsEvent struct {
	IngressClass string
	Cache        cache.Cache
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *EnqueueRequestsForPodsEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *EnqueueRequestsForPodsEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	podOld := e.ObjectOld.(*corev1.Pod)
	podNew := e.ObjectNew.(*corev1.Pod)

	// we only enqueue reconcile events for pods whose containers changed state
	// (ContainersReady vs not ContainersReady).
	if backend.IsPodSuitableAsIPTarget(podNew) != backend.IsPodSuitableAsIPTarget(podOld) {
		// ... and only for pods referenced by an endpoint backing an ingress:
		h.enqueueImpactedIngresses(podNew, queue)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *EnqueueRequestsForPodsEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *EnqueueRequestsForPodsEvent) Generic(event.GenericEvent, workqueue.RateLimitingInterface) {
}

func (h *EnqueueRequestsForPodsEvent) enqueueImpactedIngresses(pod *corev1.Pod, queue workqueue.RateLimitingInterface) {
	ingressList := &extensions.IngressList{}
	if err := h.Cache.List(context.Background(), client.InNamespace(pod.Namespace), ingressList); err != nil {
		glog.Errorf("failed to fetch ingresses impacted by pod %s due to %v", pod.GetName(), err)
		return
	}

	if pod.Status.PodIP == "" {
		return
	}

	for _, ingress := range ingressList.Items {
		if !class.IsValidIngress(h.IngressClass, &ingress) {
			continue
		}

		backends, _, err := tg.ExtractTargetGroupBackends(&ingress)
		if err != nil {
			glog.Errorf("failed to extract backend services from ingress %s/%s, reconciling the ingress. Error: %e",
				ingress.Namespace, ingress.Name, err)
			queue.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: ingress.Namespace,
					Name:      ingress.Name,
				},
			})
			break
		}

		for _, backend := range backends {
			endpoint := &corev1.Endpoints{}
			nspname := types.NamespacedName{
				Namespace: ingress.Namespace,
				Name:      backend.ServiceName,
			}
			if err = h.Cache.Get(context.Background(), nspname, endpoint); err != nil {
				glog.Errorf("failed to fetch enpoint %s backing ingress %s/%s, ignoring",
					backend.ServiceName, ingress.Namespace, ingress.Name)
				continue
			}

			if h.isPodInEndpoint(pod, endpoint) {
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

func (h *EnqueueRequestsForPodsEvent) isPodInEndpoint(pod *corev1.Pod, endpoint *corev1.Endpoints) bool {
	for _, sub := range endpoint.Subsets {
		for _, addr := range sub.Addresses {
			if addr.IP == "" || addr.TargetRef == nil || addr.TargetRef.Kind != "Pod" || addr.TargetRef.Name != pod.Name {
				continue
			}
			return true
		}
		for _, addr := range sub.NotReadyAddresses {
			if addr.IP == "" || addr.TargetRef == nil || addr.TargetRef.Kind != "Pod" || addr.TargetRef.Name != pod.Name {
				continue
			}
			return true
		}
	}
	return false
}
