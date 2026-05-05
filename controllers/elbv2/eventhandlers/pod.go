package eventhandlers

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/targetgroupbinding"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewEnqueueRequestsForPodEvent constructs new enqueueRequestsForPodEvent.
func NewEnqueueRequestsForPodEvent(logger logr.Logger) handler.TypedEventHandler[*k8s.PodInfo, reconcile.Request] {
	return &enqueueRequestsForPodEvent{
		logger: logger,
	}
}

var _ handler.TypedEventHandler[*k8s.PodInfo, reconcile.Request] = (*enqueueRequestsForPodEvent)(nil)

type enqueueRequestsForPodEvent struct {
	logger logr.Logger
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *enqueueRequestsForPodEvent) Create(ctx context.Context, e event.TypedCreateEvent[*k8s.PodInfo], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	podNew := e.Object
	h.enqueueImpactedTargetGroupBindings(ctx, queue, podNew)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *enqueueRequestsForPodEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*k8s.PodInfo], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	podOld := e.ObjectOld
	podNew := e.ObjectNew
	if !equality.Semantic.DeepEqual(podOld, podNew) {
		h.enqueueImpactedTargetGroupBindings(ctx, queue, podNew)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *enqueueRequestsForPodEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*k8s.PodInfo], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	podOld := e.Object
	h.enqueueImpactedTargetGroupBindings(ctx, queue, podOld)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile AutoScaling, or a WebHook.
func (h *enqueueRequestsForPodEvent) Generic(context.Context, event.TypedGenericEvent[*k8s.PodInfo], workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForPodEvent) enqueueImpactedTargetGroupBindings(_ context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], pod *k8s.PodInfo) {
	for _, gate := range pod.ReadinessGates {
		gateCondition := string(gate.ConditionType)
		for _, prefix := range []string{targetgroupbinding.TargetHealthPodConditionTypePrefix, targetgroupbinding.TargetHealthPodConditionTypePrefixLegacy} {
			if strings.HasPrefix(gateCondition, prefix) {
				tgb := types.NamespacedName{
					Namespace: pod.Key.Namespace,
					Name:      gateCondition[len(prefix)+1:],
				}

				h.logger.V(1).Info("enqueue targetGroupBinding for pod event", "pod", pod.Key.Name, "targetGroupBinding", tgb)
				queue.Add(reconcile.Request{
					NamespacedName: tgb,
				})
			}
		}
	}
}
