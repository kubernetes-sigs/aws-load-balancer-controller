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

// NewEnqueueRequestsForServiceEvent constructs new enqueueRequestsForServiceEvent.
func NewEnqueueRequestsForServiceEvent(k8sClient client.Client, logger logr.Logger) handler.EventHandler {
	return &enqueueRequestsForServiceEvent{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

type enqueueRequestsForServiceEvent struct {
	k8sClient client.Client
	logger    logr.Logger
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *enqueueRequestsForServiceEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	svcNew := e.Object.(*corev1.Service)
	h.enqueueImpactedTargetGroupBindings(ctx, queue, svcNew)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *enqueueRequestsForServiceEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	svcOld := e.ObjectOld.(*corev1.Service)
	svcNew := e.ObjectNew.(*corev1.Service)
	if !equality.Semantic.DeepEqual(svcOld.Spec.Ports, svcNew.Spec.Ports) {
		h.enqueueImpactedTargetGroupBindings(ctx, queue, svcNew)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *enqueueRequestsForServiceEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	svcOld := e.Object.(*corev1.Service)
	h.enqueueImpactedTargetGroupBindings(ctx, queue, svcOld)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile AutoScaling, or a WebHook.
func (h *enqueueRequestsForServiceEvent) Generic(context.Context, event.GenericEvent, workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// nothing to do here
}

// enqueueImpactedEndpointBindings will enqueue all impacted TargetGroupBindings for service events.
func (h *enqueueRequestsForServiceEvent) enqueueImpactedTargetGroupBindings(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], svc *corev1.Service) {
	tgbList := &elbv2api.TargetGroupBindingList{}
	if err := h.k8sClient.List(context.Background(), tgbList,
		client.InNamespace(svc.Namespace),
		client.MatchingFields{targetgroupbinding.IndexKeyServiceRefName: svc.Name}); err != nil {
		h.logger.Error(err, "failed to fetch targetGroupBindings")
		return
	}

	svcKey := k8s.NamespacedName(svc)
	for _, tgb := range tgbList.Items {
		if tgb.Spec.TargetType == nil || (*tgb.Spec.TargetType) != elbv2api.TargetTypeInstance {
			continue
		}

		h.logger.V(1).Info("enqueue targetGroupBinding for service event",
			"service", svcKey,
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
