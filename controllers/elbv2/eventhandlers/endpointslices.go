package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	discv1 "k8s.io/api/discovery/v1beta1"
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

// NewEnqueueRequestsForEndpointSlicesEvent constructs new enqueueRequestsForEndpointSlicesEvent.
func NewEnqueueRequestsForEndpointSlicesEvent(k8sClient client.Client, logger logr.Logger) handler.EventHandler {
	return &enqueueRequestsForEndpointSlicesEvent{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForEndpointSlicesEvent)(nil)

type enqueueRequestsForEndpointSlicesEvent struct {
	k8sClient client.Client
	logger    logr.Logger
}

// Create is called in response to an create event - e.g. EndpointSlice Creation.
func (h *enqueueRequestsForEndpointSlicesEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	epNew := e.Object.(*discv1.EndpointSlice)
	h.logger.Info("Create event for EndpointSlices", "name", epNew.Name)
	h.enqueueImpactedTargetGroupBindings(queue, epNew)
}

// Update is called in response to an update event -  e.g. EndpointSlice Updated.
func (h *enqueueRequestsForEndpointSlicesEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	epOld := e.ObjectOld.(*discv1.EndpointSlice)
	epNew := e.ObjectNew.(*discv1.EndpointSlice)
	h.logger.Info("Update event for EndpointSlices", "name", epNew.Name)
	if !equality.Semantic.DeepEqual(epOld.Ports, epNew.Ports) || !equality.Semantic.DeepEqual(epOld.Endpoints, epNew.Endpoints) {
		h.logger.Info("Enqueue EndpointSlice", "name", epNew.Name)
		h.enqueueImpactedTargetGroupBindings(queue, epNew)
	}
}

// Delete is called in response to a delete event - e.g. EndpointSlice Deleted.
func (h *enqueueRequestsForEndpointSlicesEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	epOld := e.Object.(*discv1.EndpointSlice)
	h.logger.Info("Deletion event for EndpointSlices", "name", epOld.Name)
	h.enqueueImpactedTargetGroupBindings(queue, epOld)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile AutoScaling, or a WebHook.
func (h *enqueueRequestsForEndpointSlicesEvent) Generic(event.GenericEvent, workqueue.RateLimitingInterface) {
}

func (h *enqueueRequestsForEndpointSlicesEvent) enqueueImpactedTargetGroupBindings(queue workqueue.RateLimitingInterface, epslice *discv1.EndpointSlice) {
	tgbList := &elbv2api.TargetGroupBindingList{}
	svcName := epslice.Labels["kubernetes.io/service-name"]
	if err := h.k8sClient.List(context.Background(), tgbList,
		client.InNamespace(epslice.Namespace),
		client.MatchingFields{targetgroupbinding.IndexKeyServiceRefName: svcName}); err != nil {
		h.logger.Error(err, "failed to fetch targetGroupBindings")
		return
	}

	epsliceKey := k8s.NamespacedName(epslice)
	for _, tgb := range tgbList.Items {
		if tgb.Spec.TargetType == nil || (*tgb.Spec.TargetType) != elbv2api.TargetTypeIP {
			continue
		}

		h.logger.V(1).Info("enqueue targetGroupBinding for endpointslices event",
			"endpointslices", epsliceKey,
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
