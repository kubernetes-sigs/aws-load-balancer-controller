package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

// NewEnqueueRequestsForGatewayClassParamsEvent constructs new enqueueRequestsForGatewayClassParamsEvent.
func NewEnqueueRequestsForGatewayClassParamsEvent(gatewayClassEventChan chan<- event.GenericEvent,
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) *enqueueRequestsForGatewayClassParamsEvent {
	return &enqueueRequestsForGatewayClassParamsEvent{
		gatewayClassEventChan: gatewayClassEventChan,
		k8sClient:             k8sClient,
		eventRecorder:         eventRecorder,
		logger:                logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForGatewayClassParamsEvent)(nil)

type enqueueRequestsForGatewayClassParamsEvent struct {
	gatewayClassEventChan chan<- event.GenericEvent
	k8sClient             client.Client
	eventRecorder         record.EventRecorder
	logger                logr.Logger
}

func (h *enqueueRequestsForGatewayClassParamsEvent) Create(e event.CreateEvent, _ workqueue.RateLimitingInterface) {
	gatewayClassParamsNew := e.Object.(*elbv2api.GatewayClassParams)
	h.enqueueImpactedGatewayClasses(gatewayClassParamsNew)
}

func (h *enqueueRequestsForGatewayClassParamsEvent) Update(e event.UpdateEvent, _ workqueue.RateLimitingInterface) {
	gatewayClassParamsOld := e.ObjectOld.(*elbv2api.GatewayClassParams)
	gatewayClassParamsNew := e.ObjectNew.(*elbv2api.GatewayClassParams)

	// we only care below update event:
	//	2. GatewayClassParams spec updates
	//	3. GatewayClassParams deletion
	if equality.Semantic.DeepEqual(gatewayClassParamsOld.Spec, gatewayClassParamsNew.Spec) &&
		equality.Semantic.DeepEqual(gatewayClassParamsOld.DeletionTimestamp.IsZero(), gatewayClassParamsNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueImpactedGatewayClasses(gatewayClassParamsNew)
}

func (h *enqueueRequestsForGatewayClassParamsEvent) Delete(e event.DeleteEvent, _ workqueue.RateLimitingInterface) {
	gatewayClassParamsOld := e.Object.(*elbv2api.GatewayClassParams)
	h.enqueueImpactedGatewayClasses(gatewayClassParamsOld)
}

func (h *enqueueRequestsForGatewayClassParamsEvent) Generic(e event.GenericEvent, _ workqueue.RateLimitingInterface) {
	// we don't have any generic event for secrets.
}

func (h *enqueueRequestsForGatewayClassParamsEvent) enqueueImpactedGatewayClasses(gatewayClassParams *elbv2api.GatewayClassParams) {
	gatewayClassList := &v1beta1.GatewayClassList{}
	if err := h.k8sClient.List(context.Background(), gatewayClassList,
		client.MatchingFields{gateway.IndexKeyGatewayClassParamsRefName: gatewayClassParams.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch gatewayClasses")
		return
	}
	for index := range gatewayClassList.Items {
		gatewayClass := &gatewayClassList.Items[index]

		h.logger.V(1).Info("enqueue gatewayClass for gatewayClassParams event",
			"gatewayClassParams", gatewayClassParams.GetName(),
			"gatewayClass", gatewayClass.GetName())
		h.gatewayClassEventChan <- event.GenericEvent{
			Object: gatewayClass,
		}
	}
}
