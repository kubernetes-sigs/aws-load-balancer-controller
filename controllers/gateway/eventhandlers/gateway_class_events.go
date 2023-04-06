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

// NewEnqueueRequestsForGatewayClassEvent constructs new enqueueRequestsForGatewayClassEvent.
func NewEnqueueRequestsForGatewayClassEvent(ingEventChan chan<- event.GenericEvent,
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) *enqueueRequestsForGatewayClassEvent {
	return &enqueueRequestsForGatewayClassEvent{
		ingEventChan:  ingEventChan,
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForGatewayClassEvent)(nil)

type enqueueRequestsForGatewayClassEvent struct {
	ingEventChan  chan<- event.GenericEvent
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForGatewayClassEvent) Create(e event.CreateEvent, _ workqueue.RateLimitingInterface) {
	gatewayClassNew := e.Object.(*v1beta1.GatewayClass)
	h.enqueueImpactedGateways(gatewayClassNew)
}

func (h *enqueueRequestsForGatewayClassEvent) Update(e event.UpdateEvent, _ workqueue.RateLimitingInterface) {
	gatewayClassOld := e.ObjectOld.(*v1beta1.GatewayClass)
	gatewayClassNew := e.ObjectNew.(*v1beta1.GatewayClass)

	// we only care below update event:
	//	2. GatewayClass spec updates
	//	3. GatewayClass deletions
	if equality.Semantic.DeepEqual(gatewayClassOld.Spec, gatewayClassNew.Spec) &&
		equality.Semantic.DeepEqual(gatewayClassOld.DeletionTimestamp.IsZero(), gatewayClassNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueImpactedGateways(gatewayClassNew)
}

func (h *enqueueRequestsForGatewayClassEvent) Delete(e event.DeleteEvent, _ workqueue.RateLimitingInterface) {
	gatewayClassOld := e.Object.(*v1beta1.GatewayClass)
	h.enqueueImpactedGateways(gatewayClassOld)
}

func (h *enqueueRequestsForGatewayClassEvent) Generic(e event.GenericEvent, _ workqueue.RateLimitingInterface) {
	gatewayClass := e.Object.(*v1beta1.GatewayClass)
	h.enqueueImpactedGateways(gatewayClass)
}

func (h *enqueueRequestsForGatewayClassEvent) enqueueImpactedGateways(gatewayClass *v1beta1.GatewayClass) {
	gatewayClassList := &v1beta1.GatewayClassList{}
	if err := h.k8sClient.List(context.Background(), gatewayClassList,
		client.MatchingFields{gateway.IndexKeyGatewayClassRefName: gatewayClass.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch GatewayClass")
		return
	}

	for index := range gatewayClassList.Items {
		gc := &gatewayClassList.Items[index]

		h.logger.V(1).Info("enqueue gatewayClass for gatewayClass event",
			"gatewayClass", gatewayClass.GetName(),
			"gatewayClass", k8s.NamespacedName(gc))
		h.ingEventChan <- event.GenericEvent{
			Object: gc,
		}
	}
}
