package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

// NewEnqueueRequestsForGatewayEvent constructs new enqueueRequestsForGatewayEvent.
func NewEnqueueRequestsForGatewayEvent(gatewayEventChan chan<- event.GenericEvent,
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) *enqueueRequestsForGatewayEvent {
	return &enqueueRequestsForGatewayEvent{
		gatewayEventChan: gatewayEventChan,
		k8sClient:        k8sClient,
		eventRecorder:    eventRecorder,
		logger:           logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForGatewayEvent)(nil)

type enqueueRequestsForGatewayEvent struct {
	gatewayEventChan chan<- event.GenericEvent
	k8sClient        client.Client
	eventRecorder    record.EventRecorder
	logger           logr.Logger
}

func (h *enqueueRequestsForGatewayEvent) Create(e event.CreateEvent, _ workqueue.RateLimitingInterface) {
	svcNew := e.Object.(*v1beta1.Gateway)
	h.enqueueImpactedGateways(svcNew)
}

func (h *enqueueRequestsForGatewayEvent) Update(e event.UpdateEvent, _ workqueue.RateLimitingInterface) {
	svcOld := e.ObjectOld.(*v1beta1.Gateway)
	svcNew := e.ObjectNew.(*v1beta1.Gateway)

	// we only care below update event:
	//	1. Gateway annotation updates
	//	2. Gateway spec updates
	//	3. Gateway deletions
	if equality.Semantic.DeepEqual(svcOld.Annotations, svcNew.Annotations) &&
		equality.Semantic.DeepEqual(svcOld.Spec, svcNew.Spec) &&
		equality.Semantic.DeepEqual(svcOld.DeletionTimestamp.IsZero(), svcNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueImpactedGateways(svcNew)
}

func (h *enqueueRequestsForGatewayEvent) Delete(e event.DeleteEvent, _ workqueue.RateLimitingInterface) {
	svcOld := e.Object.(*v1beta1.Gateway)
	h.enqueueImpactedGateways(svcOld)
}

func (h *enqueueRequestsForGatewayEvent) Generic(e event.GenericEvent, _ workqueue.RateLimitingInterface) {
	svc := e.Object.(*v1beta1.Gateway)
	h.enqueueImpactedGateways(svc)
}

func (h *enqueueRequestsForGatewayEvent) enqueueImpactedGateways(svc *v1beta1.Gateway) {
	gatewayList := &v1beta1.GatewayList{}
	if err := h.k8sClient.List(context.Background(), gatewayList,
		client.InNamespace(svc.GetNamespace()),
		client.MatchingFields{gateway.IndexKeyGatewayRefName: svc.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch gateways")
		return
	}

	svcKey := k8s.NamespacedName(svc)
	for index := range gatewayList.Items {
		gw := &gatewayList.Items[index]

		h.logger.V(1).Info("enqueue gateway for service event",
			"service", svcKey,
			"gateway", k8s.NamespacedName(gw))
		h.gatewayEventChan <- event.GenericEvent{
			Object: gw,
		}
	}
}
