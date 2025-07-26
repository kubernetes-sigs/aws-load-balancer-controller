package gateway

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// NewListenerRuleConfigurationReconciler constructs a reconciler that responds to listener rule configuration changes
func NewListenerRuleConfigurationReconciler(k8sClient client.Client, eventRecorder record.EventRecorder, controllerConfig config.ControllerConfig, finalizerManager k8s.FinalizerManager, logger logr.Logger) Reconciler {

	return &listenerRuleConfigurationReconciler{
		k8sClient:        k8sClient,
		eventRecorder:    eventRecorder,
		logger:           logger,
		finalizerManager: finalizerManager,
		workers:          controllerConfig.GatewayClassMaxConcurrentReconciles,
	}
}

// listenerRuleConfigurationReconciler reconciles listener rule configurations
type listenerRuleConfigurationReconciler struct {
	k8sClient        client.Client
	logger           logr.Logger
	eventRecorder    record.EventRecorder
	finalizerManager k8s.FinalizerManager
	workers          int
}

func (r *listenerRuleConfigurationReconciler) SetupWatches(_ context.Context, ctrl controller.Controller, mgr ctrl.Manager) error {

	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &elbv2gw.ListenerRuleConfiguration{}, &handler.TypedEnqueueRequestForObject[*elbv2gw.ListenerRuleConfiguration]{})); err != nil {
		return err
	}

	return nil
}

func (r *listenerRuleConfigurationReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	return runtime.HandleReconcileError(r.reconcile(ctx, req), r.logger)
}

func (r *listenerRuleConfigurationReconciler) reconcile(ctx context.Context, req reconcile.Request) error {
	listenerRuleConf := &elbv2gw.ListenerRuleConfiguration{}
	if err := r.k8sClient.Get(ctx, req.NamespacedName, listenerRuleConf); err != nil {
		return client.IgnoreNotFound(err)
	}

	r.logger.V(1).Info("Reconcile request for listener rule configuration", "cfg", listenerRuleConf)

	if listenerRuleConf.DeletionTimestamp == nil || listenerRuleConf.DeletionTimestamp.IsZero() {
		return r.handleUpdate(listenerRuleConf)
	}

	return r.handleDelete(listenerRuleConf)
}

func (r *listenerRuleConfigurationReconciler) handleUpdate(listenerRuleConf *elbv2gw.ListenerRuleConfiguration) error {
	if k8s.HasFinalizer(listenerRuleConf, shared_constants.ListenerRuleConfigurationFinalizer) {
		return nil
	}
	return r.finalizerManager.AddFinalizers(context.Background(), listenerRuleConf, shared_constants.ListenerRuleConfigurationFinalizer)
}

func (r *listenerRuleConfigurationReconciler) handleDelete(listenerRuleConf *elbv2gw.ListenerRuleConfiguration) error {
	if !k8s.HasFinalizer(listenerRuleConf, shared_constants.ListenerRuleConfigurationFinalizer) {
		return nil
	}

	inUse, err := routeutils.IsListenerRuleConfigInUse(context.Background(), listenerRuleConf, r.k8sClient)

	if err != nil {
		return fmt.Errorf("skipping finalizer removal due failure to verify if listener rule configuration [%+v] is in use. Error : %w ", k8s.NamespacedName(listenerRuleConf), err)
	}
	// if the listener rule configuration is still in use, we should not delete it
	if inUse {
		return fmt.Errorf("failed to remove finalizers as listener rule configuration [%+v] is still in use", k8s.NamespacedName(listenerRuleConf))
	}
	return r.finalizerManager.RemoveFinalizers(context.Background(), listenerRuleConf, shared_constants.ListenerRuleConfigurationFinalizer)
}

func (r *listenerRuleConfigurationReconciler) SetupWithManager(_ context.Context, mgr ctrl.Manager) (controller.Controller, error) {
	return controller.New(constants.ListenerRuleConfigurationController, mgr, controller.Options{
		MaxConcurrentReconciles: r.workers,
		Reconciler:              r,
	})

}
