package eventhandlers

import (
	"context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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

// NewEnqueueRequestsForServiceEvent constructs new enqueueRequestsForServiceEvent.
func NewEnqueueRequestsForServiceEvent(ingEventChan chan<- event.GenericEvent,
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) *enqueueRequestsForServiceEvent {
	return &enqueueRequestsForServiceEvent{
		ingEventChan:  ingEventChan,
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForServiceEvent)(nil)

type enqueueRequestsForServiceEvent struct {
	ingEventChan  chan<- event.GenericEvent
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func (h *enqueueRequestsForServiceEvent) Create(e event.CreateEvent, _ workqueue.RateLimitingInterface) {
	svcNew := e.Object.(*corev1.Service)
	h.enqueueImpactedGateways(svcNew)
}

func (h *enqueueRequestsForServiceEvent) Update(e event.UpdateEvent, _ workqueue.RateLimitingInterface) {
	svcOld := e.ObjectOld.(*corev1.Service)
	svcNew := e.ObjectNew.(*corev1.Service)

	// we only care below update event:
	//	1. Service annotation updates
	//	2. Service spec updates
	//	3. Service deletions
	if equality.Semantic.DeepEqual(svcOld.Annotations, svcNew.Annotations) &&
		equality.Semantic.DeepEqual(svcOld.Spec, svcNew.Spec) &&
		equality.Semantic.DeepEqual(svcOld.DeletionTimestamp.IsZero(), svcNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueImpactedGateways(svcNew)
}

func (h *enqueueRequestsForServiceEvent) Delete(e event.DeleteEvent, _ workqueue.RateLimitingInterface) {
	svcOld := e.Object.(*corev1.Service)
	h.enqueueImpactedGateways(svcOld)
}

func (h *enqueueRequestsForServiceEvent) Generic(e event.GenericEvent, _ workqueue.RateLimitingInterface) {
	svc := e.Object.(*corev1.Service)
	h.enqueueImpactedGateways(svc)
}

func (h *enqueueRequestsForServiceEvent) enqueueImpactedGateways(svc *corev1.Service) {
	gatewayList := &v1beta1.GatewayList{}
	if err := h.k8sClient.List(context.Background(), gatewayList,
		client.InNamespace(svc.GetNamespace()),
		client.MatchingFields{gateway.IndexKeyServiceRefName: svc.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch gateways")
		return
	}

	svcKey := k8s.NamespacedName(svc)
	for index := range gatewayList.Items {
		gw := &gatewayList.Items[index]

		h.logger.V(1).Info("enqueue gateway for service event",
			"service", svcKey,
			"gateway", k8s.NamespacedName(gw))
		h.ingEventChan <- event.GenericEvent{
			Object: gw,
		}
	}
}
