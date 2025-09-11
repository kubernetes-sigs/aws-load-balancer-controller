package gateway

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/gatewayutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/referencecounter"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// NewTargetGroupConfigurationReconciler constructs a reconciler that responds to targetgroup configuration changes
func NewTargetGroupConfigurationReconciler(k8sClient client.Client, eventRecorder record.EventRecorder, controllerConfig config.ControllerConfig, serviceReferenceCounter referencecounter.ServiceReferenceCounter, finalizerManager k8s.FinalizerManager, logger logr.Logger) Reconciler {

	return &targetgroupConfigurationReconciler{
		k8sClient:               k8sClient,
		eventRecorder:           eventRecorder,
		logger:                  logger,
		finalizerManager:        finalizerManager,
		serviceReferenceCounter: serviceReferenceCounter,
		gwRetrieveFn:            gatewayutils.GetGatewaysManagedByLBController,
		workers:                 controllerConfig.GatewayClassMaxConcurrentReconciles,
	}
}

// targetgroupConfigurationReconciler reconciles target group configurations
type targetgroupConfigurationReconciler struct {
	k8sClient               client.Client
	logger                  logr.Logger
	eventRecorder           record.EventRecorder
	finalizerManager        k8s.FinalizerManager
	serviceReferenceCounter referencecounter.ServiceReferenceCounter

	gwRetrieveFn func(ctx context.Context, k8sClient client.Client, gwController string) ([]*gwv1.Gateway, error)
	workers      int
}

func (r *targetgroupConfigurationReconciler) SetupWatches(_ context.Context, ctrl controller.Controller, mgr ctrl.Manager, _ *kubernetes.Clientset) error {

	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &elbv2gw.TargetGroupConfiguration{}, &handler.TypedEnqueueRequestForObject[*elbv2gw.TargetGroupConfiguration]{})); err != nil {
		return err
	}

	return nil
}

func (r *targetgroupConfigurationReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	return runtime.HandleReconcileError(r.reconcile(ctx, req), r.logger)
}

func (r *targetgroupConfigurationReconciler) reconcile(ctx context.Context, req reconcile.Request) error {
	tgConf := &elbv2gw.TargetGroupConfiguration{}
	if err := r.k8sClient.Get(ctx, req.NamespacedName, tgConf); err != nil {
		return client.IgnoreNotFound(err)
	}

	r.logger.V(1).Info("Found tg configuration", "cfg", tgConf)

	if tgConf.DeletionTimestamp == nil || tgConf.DeletionTimestamp.IsZero() {
		return r.handleUpdate(tgConf)
	}
	return r.handleDelete(tgConf)
}

func (r *targetgroupConfigurationReconciler) handleUpdate(tgConf *elbv2gw.TargetGroupConfiguration) error {
	if k8s.HasFinalizer(tgConf, shared_constants.TargetGroupConfigurationFinalizer) {
		return nil
	}
	return r.finalizerManager.AddFinalizers(context.Background(), tgConf, shared_constants.TargetGroupConfigurationFinalizer)
}

func (r *targetgroupConfigurationReconciler) handleDelete(tgConf *elbv2gw.TargetGroupConfiguration) error {
	if !k8s.HasFinalizer(tgConf, shared_constants.TargetGroupConfigurationFinalizer) {
		return nil
	}

	allGateways := make([]types.NamespacedName, 0)

	for _, c := range constants.FullGatewayControllerSet.UnsortedList() {
		partial, err := r.gwRetrieveFn(context.Background(), r.k8sClient, c)
		if err != nil {
			return err
		}

		for _, gw := range partial {
			allGateways = append(allGateways, k8s.NamespacedName(gw))
		}
	}

	svcReference := types.NamespacedName{
		Namespace: tgConf.Namespace,
		Name:      tgConf.Spec.TargetReference.Name,
	}

	eligibleForRemoval := r.serviceReferenceCounter.IsEligibleForRemoval(svcReference, allGateways)

	// if the targetgroup configuration is still in use, we should not delete it
	if !eligibleForRemoval {
		return fmt.Errorf("targetgroup configuration [%+v] is still in use", k8s.NamespacedName(tgConf))
	}
	return r.finalizerManager.RemoveFinalizers(context.Background(), tgConf, shared_constants.TargetGroupConfigurationFinalizer)
}

func (r *targetgroupConfigurationReconciler) SetupWithManager(_ context.Context, mgr ctrl.Manager) (controller.Controller, error) {
	return controller.New(constants.TargetGroupConfigurationController, mgr, controller.Options{
		MaxConcurrentReconciles: r.workers,
		Reconciler:              r,
	})

}
