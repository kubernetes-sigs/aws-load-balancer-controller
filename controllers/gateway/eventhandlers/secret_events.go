package eventhandlers

import (
	"context"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// NewEnqueueRequestsForSecretEvent constructs new enqueueRequestsForSecretEvent.
func NewEnqueueRequestsForSecretEvent(listenerRuleConfigEventChan chan<- event.TypedGenericEvent[*elbv2gw.ListenerRuleConfiguration],
	k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) handler.TypedEventHandler[*corev1.Secret, reconcile.Request] {
	return &enqueueRequestsForSecretEvent{
		listenerRuleConfigEventChan: listenerRuleConfigEventChan,
		k8sClient:                   k8sClient,
		eventRecorder:               eventRecorder,
		logger:                      logger,
	}
}

var _ handler.TypedEventHandler[*corev1.Secret, reconcile.Request] = (*enqueueRequestsForSecretEvent)(nil)

type enqueueRequestsForSecretEvent struct {
	listenerRuleConfigEventChan chan<- event.TypedGenericEvent[*elbv2gw.ListenerRuleConfiguration]
	k8sClient                   client.Client
	eventRecorder               record.EventRecorder
	logger                      logr.Logger
}

func (h *enqueueRequestsForSecretEvent) Create(ctx context.Context, e event.TypedCreateEvent[*corev1.Secret], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	//No-Op : We will only start monitoring secret events after they have been created and associated with gateway specific resources. We don't watch cluster-wide secret events.
}

func (h *enqueueRequestsForSecretEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*corev1.Secret], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	secretOld := e.ObjectOld
	secretNew := e.ObjectNew

	// we only care below update event:
	//	1. Secret data updates
	//	2. Secret deletions
	if equality.Semantic.DeepEqual(secretOld.Data, secretNew.Data) &&
		equality.Semantic.DeepEqual(secretOld.DeletionTimestamp.IsZero(), secretNew.DeletionTimestamp.IsZero()) {
		return
	}
	h.logger.V(1).Info("enqueue secret update event", "secret", secretNew.Name)
	h.enqueueImpactedListenerRulesConfigs(ctx, secretNew)
}

func (h *enqueueRequestsForSecretEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*corev1.Secret], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	secretOld := e.Object
	h.logger.V(1).Info("enqueue secret delete event", "secret", secretOld.Name)
	h.enqueueImpactedListenerRulesConfigs(ctx, secretOld)
}

func (h *enqueueRequestsForSecretEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*corev1.Secret], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	secretObj := e.Object
	h.logger.V(1).Info("enqueue secret generic event", "secret", secretObj.Name)
	h.enqueueImpactedListenerRulesConfigs(ctx, secretObj)
}

func (h *enqueueRequestsForSecretEvent) enqueueImpactedListenerRulesConfigs(ctx context.Context, secret *corev1.Secret) {
	listenerRuleCfgList, err := routeutils.FilterListenerRuleConfigBySecret(ctx, h.k8sClient, secret)
	if err != nil {
		h.logger.Error(err, "failed to fetch listener rule configs referring to secret", "secret", k8s.NamespacedName(secret))
		return
	}

	for _, listenerRuleCfg := range listenerRuleCfgList {
		h.logger.V(1).Info("enqueue listenerRuleCfg for secret event",
			"secret", k8s.NamespacedName(secret),
			"listenerRuleCfg", k8s.NamespacedName(listenerRuleCfg))
		h.listenerRuleConfigEventChan <- event.TypedGenericEvent[*elbv2gw.ListenerRuleConfiguration]{
			Object: listenerRuleCfg,
		}
	}
}
