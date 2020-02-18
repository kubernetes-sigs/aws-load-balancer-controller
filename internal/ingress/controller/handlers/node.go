package handlers

import (
	"context"

	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/class"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = (*EnqueueRequestsForNodeEvent)(nil)

type EnqueueRequestsForNodeEvent struct {
	IngressClass string

	Cache cache.Cache
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *EnqueueRequestsForNodeEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedIngresses(queue)
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *EnqueueRequestsForNodeEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedIngresses(queue)
}

// TODO: change this to only enqueue ingresses when available node set is changed.(rely on node's ready condition)
// We can store an copy of previous known valid nodeSet inside this class, and compare them when events occurs.
// Pending work:
//    1. rely on node's ready condition instead of aws.IsNodeHealth API
//    1. when modify/detach instance sg, rely on describeNetworkInterface API to get enis attached, to avoid edge cases like node turned into unhealthy or excluded by "alpha.service-controller.kubernetes.io/exclude-balancer"

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *EnqueueRequestsForNodeEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	//h.enqueueImpactedIngresses(queue)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *EnqueueRequestsForNodeEvent) Generic(event.GenericEvent, workqueue.RateLimitingInterface) {
}

// Ideally this should only enqueue ingresses that have changed
func (h *EnqueueRequestsForNodeEvent) enqueueImpactedIngresses(queue workqueue.RateLimitingInterface) {
	ingressList := &networking.IngressList{}
	if err := h.Cache.List(context.Background(), nil, ingressList); err != nil {
		glog.Errorf("failed to fetch impacted ingresses by node due to %v", err)
		return
	}

	for _, ingress := range ingressList.Items {
		if !class.IsValidIngress(h.IngressClass, &ingress) {
			continue
		}
		queue.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: ingress.Namespace,
				Name:      ingress.Name,
			},
		})
	}
}
