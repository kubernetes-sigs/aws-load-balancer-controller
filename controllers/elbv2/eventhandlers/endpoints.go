package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/targetgroupbinding"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewEnqueueRequestsForEndpointsEvent constructs new enqueueRequestsForEndpointsEvent.
func NewEnqueueRequestsForEndpointsEvent(k8sClient client.Client, logger logr.Logger) handler.EventHandler {
	return &enqueueRequestsForEndpointsEvent{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForEndpointsEvent)(nil)

type enqueueRequestsForEndpointsEvent struct {
	k8sClient client.Client
	logger    logr.Logger
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *enqueueRequestsForEndpointsEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	epNew := e.Object.(*corev1.Endpoints)
	h.enqueueImpactedTargetGroupBindings(queue, epNew)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *enqueueRequestsForEndpointsEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	epOld := e.ObjectOld.(*corev1.Endpoints)
	epNew := e.ObjectNew.(*corev1.Endpoints)
	if !equality.Semantic.DeepEqual(epOld.Subsets, epNew.Subsets) {
		h.enqueueImpactedTargetGroupBindings(queue, epNew)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *enqueueRequestsForEndpointsEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	epOld := e.Object.(*corev1.Endpoints)
	h.enqueueImpactedTargetGroupBindings(queue, epOld)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile AutoScaling, or a WebHook.
func (h *enqueueRequestsForEndpointsEvent) Generic(event.GenericEvent, workqueue.RateLimitingInterface) {
}

func (h *enqueueRequestsForEndpointsEvent) enqueueImpactedTargetGroupBindings(queue workqueue.RateLimitingInterface, ep *corev1.Endpoints) {
	tgbList := &elbv2api.TargetGroupBindingList{}
	if err := h.k8sClient.List(context.Background(), tgbList,
		client.InNamespace(ep.Namespace),
		client.MatchingFields{targetgroupbinding.IndexKeyServiceRefName: ep.Name}); err != nil {
		h.logger.Error(err, "failed to fetch targetGroupBindings")
		return
	}

	epKey := k8s.NamespacedName(ep)
	for _, tgb := range tgbList.Items {
		if tgb.Spec.TargetType == nil || (*tgb.Spec.TargetType) != elbv2api.TargetTypeIP {
			continue
		}

		h.logger.V(1).Info("enqueue targetGroupBinding for endpoints event",
			"endpoints", epKey,
			"targetGroupBinding", k8s.NamespacedName(&tgb),
		)
		queue.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: tgb.Namespace,
				Name:      tgb.Name,
			},
		})
	}
}
