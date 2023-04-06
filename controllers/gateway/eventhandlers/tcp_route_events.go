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
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

// NewEnqueueRequestsForTCPRouteEvent constructs new enqueueRequestsForTCPRouteEvent.
func NewEnqueueRequestsForTCPRouteEvent(ingEventChan chan<- event.GenericEvent,
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) *enqueueRequestsForTCPRouteEvent {
	return &enqueueRequestsForTCPRouteEvent{
		ingEventChan:  ingEventChan,
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForTCPRouteEvent)(nil)

type enqueueRequestsForTCPRouteEvent struct {
	ingEventChan  chan<- event.GenericEvent
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForTCPRouteEvent) Create(e event.CreateEvent, _ workqueue.RateLimitingInterface) {
	svcNew := e.Object.(*v1alpha2.TCPRoute)
	h.enqueueImpactedGateways(svcNew)
}

func (h *enqueueRequestsForTCPRouteEvent) Update(e event.UpdateEvent, _ workqueue.RateLimitingInterface) {
	svcOld := e.ObjectOld.(*v1alpha2.TCPRoute)
	svcNew := e.ObjectNew.(*v1alpha2.TCPRoute)

	// we only care below update event:
	//	1. TCPRoute annotation updates
	//	2. TCPRoute spec updates
	//	3. TCPRoute deletions
	if equality.Semantic.DeepEqual(svcOld.Annotations, svcNew.Annotations) &&
		equality.Semantic.DeepEqual(svcOld.Spec, svcNew.Spec) &&
		equality.Semantic.DeepEqual(svcOld.DeletionTimestamp.IsZero(), svcNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueImpactedGateways(svcNew)
}

func (h *enqueueRequestsForTCPRouteEvent) Delete(e event.DeleteEvent, _ workqueue.RateLimitingInterface) {
	svcOld := e.Object.(*v1alpha2.TCPRoute)
	h.enqueueImpactedGateways(svcOld)
}

func (h *enqueueRequestsForTCPRouteEvent) Generic(e event.GenericEvent, _ workqueue.RateLimitingInterface) {
	svc := e.Object.(*v1alpha2.TCPRoute)
	h.enqueueImpactedGateways(svc)
}

func (h *enqueueRequestsForTCPRouteEvent) enqueueImpactedGateways(svc *v1alpha2.TCPRoute) {
	gatewayList := &v1beta1.GatewayList{}
	if err := h.k8sClient.List(context.Background(), gatewayList,
		client.InNamespace(svc.GetNamespace()),
		client.MatchingFields{gateway.IndexKeyTCPRouteRefName: svc.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch gateways")
		return
	}
	// TODO:  We actually have to reference all the service names to match up a tcp route with a gateway

	svcKey := k8s.NamespacedName(svc)
	for index := range gatewayList.Items {
		gw := &gatewayList.Items[index]

		h.logger.V(1).Info("enqueue gateway for TCPRoute event",
			"service", svcKey,
			"gateway", k8s.NamespacedName(gw))
		h.ingEventChan <- event.GenericEvent{
			Object: gw,
		}
	}
}
