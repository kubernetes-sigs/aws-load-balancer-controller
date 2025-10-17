/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	ctrlerrors "sigs.k8s.io/aws-load-balancer-controller/pkg/error"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	metricsutil "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/util"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
)

const (
	controllerName = "globalAccelerator"
	agaTagPrefix   = "aga.k8s.aws"

	// the groupVersion of used GlobalAccelerator resource.
	agaResourcesGroupVersion = "aga.k8s.aws/v1beta1"
	globalAcceleratorKind    = "GlobalAccelerator"

	// Metric stage constants
	MetricStageFetchGlobalAccelerator     = "fetch_globalAccelerator"
	MetricStageAddFinalizers              = "add_finalizers"
	MetricStageBuildModel                 = "build_model"
	MetricStageReconcileGlobalAccelerator = "reconcile_globalaccelerator"

	// Metric error constants
	MetricErrorAddFinalizers              = "add_finalizers_error"
	MetricErrorRemoveFinalizers           = "remove_finalizers_error"
	MetricErrorBuildModel                 = "build_model_error"
	MetricErrorReconcileGlobalAccelerator = "reconcile_globalaccelerator_error"
)

// NewGlobalAcceleratorReconciler constructs new globalAcceleratorReconciler
func NewGlobalAcceleratorReconciler(k8sClient client.Client, eventRecorder record.EventRecorder, finalizerManager k8s.FinalizerManager, config config.ControllerConfig, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector, reconcileCounters *metricsutil.ReconcileCounters) *globalAcceleratorReconciler {

	// Create tracking provider
	trackingProvider := tracking.NewDefaultProvider(agaTagPrefix, config.ClusterName)

	// Create model builder
	agaModelBuilder := aga.NewDefaultModelBuilder(
		k8sClient,
		eventRecorder,
		trackingProvider,
		config.FeatureGates,
		config.ClusterName,
		config.DefaultTags,
		config.ExternalManagedTags,
		logger.WithName("aga-model-builder"),
		metricsCollector,
	)

	// Create stack marshaller
	stackMarshaller := deploy.NewDefaultStackMarshaller()

	return &globalAcceleratorReconciler{
		k8sClient:        k8sClient,
		eventRecorder:    eventRecorder,
		finalizerManager: finalizerManager,
		logger:           logger,
		modelBuilder:     agaModelBuilder,
		stackMarshaller:  stackMarshaller,
		metricsCollector: metricsCollector,
		reconcileTracker: reconcileCounters.IncrementAGA,

		maxConcurrentReconciles: config.GlobalAcceleratorMaxConcurrentReconciles,
	}
}

// globalAcceleratorReconciler reconciles a GlobalAccelerator object
type globalAcceleratorReconciler struct {
	k8sClient        client.Client
	eventRecorder    record.EventRecorder
	finalizerManager k8s.FinalizerManager
	modelBuilder     aga.ModelBuilder
	stackMarshaller  deploy.StackMarshaller
	logger           logr.Logger
	metricsCollector lbcmetrics.MetricCollector
	reconcileTracker func(namespaceName types.NamespacedName)

	maxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=aga.k8s.aws,resources=globalaccelerators,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=aga.k8s.aws,resources=globalaccelerators/status,verbs=update;patch
// +kubebuilder:rbac:groups=aga.k8s.aws,resources=globalaccelerators/finalizers,verbs=update;patch
func (r *globalAcceleratorReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	r.reconcileTracker(req.NamespacedName)
	r.logger.V(1).Info("Reconcile request", "name", req.Name)
	err := r.reconcile(ctx, req)
	return runtime.HandleReconcileError(err, r.logger)
}

func (r *globalAcceleratorReconciler) reconcile(ctx context.Context, req reconcile.Request) error {
	ga := &agaapi.GlobalAccelerator{}
	var err error
	fetchGlobalAcceleratorFn := func() {
		err = r.k8sClient.Get(ctx, req.NamespacedName, ga)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, MetricStageFetchGlobalAccelerator, fetchGlobalAcceleratorFn)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	if ga.DeletionTimestamp != nil && !ga.DeletionTimestamp.IsZero() {
		return r.cleanupGlobalAccelerator(ctx, ga)
	}
	return r.reconcileGlobalAccelerator(ctx, ga)
}

func (r *globalAcceleratorReconciler) reconcileGlobalAccelerator(ctx context.Context, ga *agaapi.GlobalAccelerator) error {
	var err error
	finalizerFn := func() {
		if !k8s.HasFinalizer(ga, shared_constants.GlobalAcceleratorFinalizer) {
			err = r.finalizerManager.AddFinalizers(ctx, ga, shared_constants.GlobalAcceleratorFinalizer)
		}
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, MetricStageAddFinalizers, finalizerFn)
	if err != nil {
		r.eventRecorder.Event(ga, corev1.EventTypeWarning, k8s.GlobalAcceleratorEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return ctrlerrors.NewErrorWithMetrics(controllerName, MetricErrorAddFinalizers, err, r.metricsCollector)
	}

	// TODO: Implement GlobalAccelerator resource management
	// This would include:
	// 1. Creating/updating AWS Global Accelerator
	// 2. Managing listeners and endpoint groups
	// 3. Handling endpoint discovery from Services/Ingresses/Gateways
	reconcileResourceFn := func() {
		err = r.reconcileGlobalAcceleratorResources(ctx, ga)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, MetricStageReconcileGlobalAccelerator, reconcileResourceFn)
	if err != nil {
		return ctrlerrors.NewErrorWithMetrics(controllerName, MetricErrorReconcileGlobalAccelerator, err, r.metricsCollector)
	}

	r.eventRecorder.Event(ga, corev1.EventTypeNormal, k8s.GlobalAcceleratorEventReasonSuccessfullyReconciled, "Successfully reconciled")
	return nil
}

func (r *globalAcceleratorReconciler) cleanupGlobalAccelerator(ctx context.Context, ga *agaapi.GlobalAccelerator) error {
	if k8s.HasFinalizer(ga, shared_constants.GlobalAcceleratorFinalizer) {
		// TODO: Implement cleanup logic for AWS Global Accelerator resources
		if err := r.cleanupGlobalAcceleratorResources(ctx, ga); err != nil {
			r.eventRecorder.Event(ga, corev1.EventTypeWarning, k8s.GlobalAcceleratorEventReasonFailedCleanup, fmt.Sprintf("Failed cleanup due to %v", err))
			return err
		}
		if err := r.finalizerManager.RemoveFinalizers(ctx, ga, shared_constants.GlobalAcceleratorFinalizer); err != nil {
			r.eventRecorder.Event(ga, corev1.EventTypeWarning, k8s.GlobalAcceleratorEventReasonFailedRemoveFinalizer, fmt.Sprintf("Failed remove finalizer due to %v", err))
			return ctrlerrors.NewErrorWithMetrics(controllerName, MetricErrorRemoveFinalizers, err, r.metricsCollector)
		}
	}
	return nil
}

func (r *globalAcceleratorReconciler) buildModel(ctx context.Context, ga *agaapi.GlobalAccelerator) (core.Stack, *agamodel.Accelerator, error) {
	stack, accelerator, err := r.modelBuilder.Build(ctx, ga)
	if err != nil {
		r.eventRecorder.Event(ga, corev1.EventTypeWarning, k8s.GlobalAcceleratorEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, err
	}
	stackJSON, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		r.eventRecorder.Event(ga, corev1.EventTypeWarning, k8s.GlobalAcceleratorEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, err
	}
	r.logger.Info("successfully built model", "model", stackJSON)
	return stack, accelerator, nil
}

func (r *globalAcceleratorReconciler) reconcileGlobalAcceleratorResources(ctx context.Context, ga *agaapi.GlobalAccelerator) error {
	r.logger.Info("Reconciling GlobalAccelerator resources", "name", ga.Name, "namespace", ga.Namespace)
	var stack core.Stack
	var accelerator *agamodel.Accelerator
	var err error
	buildModelFn := func() {
		stack, accelerator, err = r.buildModel(ctx, ga)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, MetricStageBuildModel, buildModelFn)
	if err != nil {
		return ctrlerrors.NewErrorWithMetrics(controllerName, MetricErrorBuildModel, err, r.metricsCollector)
	}

	// Log the built model for debugging
	r.logger.Info("Built model successfully", "accelerator", accelerator.ID(), "stackID", stack.StackID())

	// TODO: Implement the deploy phase
	// This would include:
	// 1. Deploy the stack to create/update AWS Global Accelerator resources
	// 2. Update the GlobalAccelerator status with the created resources
	// 3. Handle any deployment errors and update status accordingly

	return nil
}

func (r *globalAcceleratorReconciler) cleanupGlobalAcceleratorResources(ctx context.Context, ga *agaapi.GlobalAccelerator) error {
	// TODO: Implement the actual AWS Global Accelerator resource cleanup
	// This is a placeholder implementation
	r.logger.Info("Cleaning up GlobalAccelerator resources", "name", ga.Name, "namespace", ga.Namespace)
	return nil
}

func (r *globalAcceleratorReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, clientSet *kubernetes.Clientset) error {
	// Check if GlobalAccelerator CRD is available
	resList, err := clientSet.ServerResourcesForGroupVersion(agaResourcesGroupVersion)
	if err != nil {
		r.logger.Info("GlobalAccelerator CRD is not available, skipping controller setup")
		return nil
	}
	globalAcceleratorResourceAvailable := k8s.IsResourceKindAvailable(resList, globalAcceleratorKind)
	if !globalAcceleratorResourceAvailable {
		r.logger.Info("GlobalAccelerator CRD is not available, skipping controller setup")
		return nil
	}

	if err := r.setupIndexes(ctx, mgr.GetFieldIndexer()); err != nil {
		return err
	}

	// TODO: Add event handlers for Services, Ingresses, and Gateways
	// that are referenced by GlobalAccelerator endpoints

	return ctrl.NewControllerManagedBy(mgr).
		For(&agaapi.GlobalAccelerator{}).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.maxConcurrentReconciles,
		}).
		Complete(r)
}

func (r *globalAcceleratorReconciler) setupIndexes(ctx context.Context, fieldIndexer client.FieldIndexer) error {
	// TODO: Add field indexes for efficient lookups
	return nil
}
