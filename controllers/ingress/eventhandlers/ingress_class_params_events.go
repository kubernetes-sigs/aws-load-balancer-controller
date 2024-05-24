package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// NewEnqueueRequestsForIngressClassParamsEvent constructs new enqueueRequestsForIngressClassParamsEvent.
func NewEnqueueRequestsForIngressClassParamsEvent(ingClassEventChan chan<- event.TypedGenericEvent[*networking.IngressClass],
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*elbv2api.IngressClassParams] {
	return &enqueueRequestsForIngressClassParamsEvent{
		ingClassEventChan: ingClassEventChan,
		k8sClient:         k8sClient,
		eventRecorder:     eventRecorder,
		logger:            logger,
	}
}

var _ handler.TypedEventHandler[*elbv2api.IngressClassParams] = (*enqueueRequestsForIngressClassParamsEvent)(nil)

type enqueueRequestsForIngressClassParamsEvent struct {
	ingClassEventChan chan<- event.TypedGenericEvent[*networking.IngressClass]
	k8sClient         client.Client
	eventRecorder     record.EventRecorder
	logger            logr.Logger
}

func (h *enqueueRequestsForIngressClassParamsEvent) Create(ctx context.Context, e event.TypedCreateEvent[*elbv2api.IngressClassParams], _ workqueue.RateLimitingInterface) {
	ingClassParamsNew := e.Object
	h.enqueueImpactedIngressClasses(ctx, ingClassParamsNew)
}

func (h *enqueueRequestsForIngressClassParamsEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*elbv2api.IngressClassParams], _ workqueue.RateLimitingInterface) {
	ingClassParamsOld := e.ObjectOld
	ingClassParamsNew := e.ObjectNew

	// we only care below update event:
	//	2. IngressClassParams spec updates
	//	3. IngressClassParams deletion
	if equality.Semantic.DeepEqual(ingClassParamsOld.Spec, ingClassParamsNew.Spec) &&
		equality.Semantic.DeepEqual(ingClassParamsOld.DeletionTimestamp.IsZero(), ingClassParamsNew.DeletionTimestamp.IsZero()) {
		return
	}

	h.enqueueImpactedIngressClasses(ctx, ingClassParamsNew)
}

func (h *enqueueRequestsForIngressClassParamsEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*elbv2api.IngressClassParams], _ workqueue.RateLimitingInterface) {
	ingClassParamsOld := e.Object
	h.enqueueImpactedIngressClasses(ctx, ingClassParamsOld)
}

func (h *enqueueRequestsForIngressClassParamsEvent) Generic(context.Context, event.TypedGenericEvent[*elbv2api.IngressClassParams], workqueue.RateLimitingInterface) {
	// we don't have any generic event for secrets.
}

func (h *enqueueRequestsForIngressClassParamsEvent) enqueueImpactedIngressClasses(ctx context.Context, ingClassParams *elbv2api.IngressClassParams) {
	ingClassList := &networking.IngressClassList{}
	if err := h.k8sClient.List(context.Background(), ingClassList,
		client.MatchingFields{ingress.IndexKeyIngressClassParamsRefName: ingClassParams.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch ingressClasses")
		return
	}
	for index := range ingClassList.Items {
		ingClass := &ingClassList.Items[index]

		h.logger.V(1).Info("enqueue ingressClass for ingressClassParams event",
			"ingressClassParams", ingClassParams.GetName(),
			"ingressClass", ingClass.GetName())
		h.ingClassEventChan <- event.TypedGenericEvent[*networking.IngressClass]{
			Object: ingClass,
		}
	}
}
