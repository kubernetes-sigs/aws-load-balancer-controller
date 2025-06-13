package gateway

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/gatewayutils"
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

// NewLoadbalancerConfigurationReconciler constructs a reconciler that responds to loadbalancer configuration changes
func NewLoadbalancerConfigurationReconciler(k8sClient client.Client, eventRecorder record.EventRecorder, controllerConfig config.ControllerConfig, finalizerManager k8s.FinalizerManager, logger logr.Logger) Reconciler {

	return &loadbalancerConfigurationReconciler{
		k8sClient:        k8sClient,
		eventRecorder:    eventRecorder,
		logger:           logger,
		finalizerManager: finalizerManager,
		workers:          controllerConfig.GatewayClassMaxConcurrentReconciles,
	}
}

// loadbalancerConfigurationReconciler reconciles load balancer configurations
type loadbalancerConfigurationReconciler struct {
	k8sClient        client.Client
	logger           logr.Logger
	eventRecorder    record.EventRecorder
	finalizerManager k8s.FinalizerManager
	workers          int
}

func (r *loadbalancerConfigurationReconciler) SetupWatches(_ context.Context, ctrl controller.Controller, mgr ctrl.Manager) error {

	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &elbv2gw.LoadBalancerConfiguration{}, &handler.TypedEnqueueRequestForObject[*elbv2gw.LoadBalancerConfiguration]{})); err != nil {
		return err
	}

	return nil
}

func (r *loadbalancerConfigurationReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	return runtime.HandleReconcileError(r.reconcile(ctx, req), r.logger)
}

func (r *loadbalancerConfigurationReconciler) reconcile(ctx context.Context, req reconcile.Request) error {
	lbConf := &elbv2gw.LoadBalancerConfiguration{}
	if err := r.k8sClient.Get(ctx, req.NamespacedName, lbConf); err != nil {
		return client.IgnoreNotFound(err)
	}

	r.logger.V(1).Info("Found loadbalancer configuration", "cfg", lbConf)

	if lbConf.DeletionTimestamp == nil || lbConf.DeletionTimestamp.IsZero() {
		return r.handleUpdate(lbConf)
	}

	return r.handleDelete(lbConf)
}

func (r *loadbalancerConfigurationReconciler) handleUpdate(lbConf *elbv2gw.LoadBalancerConfiguration) error {
	if k8s.HasFinalizer(lbConf, shared_constants.LoadBalancerConfigurationFinalizer) {
		return nil
	}
	return r.finalizerManager.AddFinalizers(context.Background(), lbConf, shared_constants.LoadBalancerConfigurationFinalizer)
}

func (r *loadbalancerConfigurationReconciler) handleDelete(lbConf *elbv2gw.LoadBalancerConfiguration) error {
	if !k8s.HasFinalizer(lbConf, shared_constants.LoadBalancerConfigurationFinalizer) {
		return nil
	}

	inUse, err := gatewayutils.IsLBConfigInUse(context.Background(), lbConf, r.k8sClient, constants.FullGatewayControllerSet)

	if err != nil {
		return err
	}
	// if the loadbalancer configuration is still in use, we should not delete it
	if inUse {
		return fmt.Errorf("loadbalancer configuration [%+v] is still in use", k8s.NamespacedName(lbConf))
	}
	return r.finalizerManager.RemoveFinalizers(context.Background(), lbConf, shared_constants.LoadBalancerConfigurationFinalizer)
}

func (r *loadbalancerConfigurationReconciler) SetupWithManager(_ context.Context, mgr ctrl.Manager) (controller.Controller, error) {
	return controller.New(constants.LoadBalancerConfigurationController, mgr, controller.Options{
		MaxConcurrentReconciles: r.workers,
		Reconciler:              r,
	})

}
