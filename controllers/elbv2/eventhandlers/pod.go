package eventhandlers

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/targetgroupbinding"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewEnqueueRequestsForPodEvent constructs new enqueueRequestsForPodEvent.
func NewEnqueueRequestsForPodEvent(logger logr.Logger) handler.EventHandler {
	return &enqueueRequestsForPodEvent{
		logger: logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForPodEvent)(nil)

type enqueueRequestsForPodEvent struct {
	logger logr.Logger
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *enqueueRequestsForPodEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	podNew := e.Object.(*corev1.Pod)
	h.logger.V(1).Info("Create event for Pods", "name", podNew.Name)
	h.enqueueImpactedTargetGroupBindings(ctx, queue, podNew)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *enqueueRequestsForPodEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	podOld := e.ObjectOld.(*corev1.Pod)
	podNew := e.ObjectNew.(*corev1.Pod)
	h.logger.V(1).Info("Update event for Pods", "name", podNew.Name)
	if !equality.Semantic.DeepEqual(podOld.Spec, podNew.Spec) || !equality.Semantic.DeepEqual(podOld.Status, podNew.Status) {
		h.logger.V(1).Info("Enqueue Pod", "name", podNew.Name)
		h.enqueueImpactedTargetGroupBindings(ctx, queue, podNew)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *enqueueRequestsForPodEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	podOld := e.Object.(*corev1.Pod)
	h.logger.V(1).Info("Deletion event for Pods", "name", podOld.Name)
	h.enqueueImpactedTargetGroupBindings(ctx, queue, podOld)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile AutoScaling, or a WebHook.
func (h *enqueueRequestsForPodEvent) Generic(context.Context, event.GenericEvent, workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForPodEvent) enqueueImpactedTargetGroupBindings(_ context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], pod *corev1.Pod) {
	for _, gate := range pod.Spec.ReadinessGates {
		gateCondition := string(gate.ConditionType)
		for _, prefix := range []string{targetgroupbinding.TargetHealthPodConditionTypePrefix, targetgroupbinding.TargetHealthPodConditionTypePrefixLegacy} {
			if strings.HasPrefix(gateCondition, prefix) {
				tgb := types.NamespacedName{
					Namespace: pod.Namespace,
					Name:      gateCondition[len(prefix)+1:],
				}

				h.logger.V(1).Info("enqueue targetGroupBinding for pod event", "pod", pod.Name, "targetGroupBinding", tgb)
				queue.Add(reconcile.Request{
					NamespacedName: tgb,
				})
			}
		}
	}
}
