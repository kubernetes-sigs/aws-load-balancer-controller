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
	"time"

	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	agadeploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/aga/eventhandlers"
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
	agastatus "sigs.k8s.io/aws-load-balancer-controller/pkg/status/aga"
	gwclientset "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

const (
	controllerName = "globalAccelerator"
	agaTagPrefix   = "aga.k8s.aws"

	// the groupVersion of used GlobalAccelerator resource.
	agaResourcesGroupVersion = "aga.k8s.aws/v1beta1"
	globalAcceleratorKind    = "GlobalAccelerator"

	// Requeue constants for provisioning state monitoring
	requeueMessage          = "Monitoring provisioning state"
	statusUpdateRequeueTime = 1 * time.Minute

	// Metric stage constants
	MetricStageFetchGlobalAccelerator     = "fetch_globalAccelerator"
	MetricStageAddFinalizers              = "add_finalizers"
	MetricStageBuildModel                 = "build_model"
	MetricStageDeployStack                = "deploy_stack"
	MetricStageReconcileGlobalAccelerator = "reconcile_globalaccelerator"

	// Metric error constants
	MetricErrorAddFinalizers              = "add_finalizers_error"
	MetricErrorRemoveFinalizers           = "remove_finalizers_error"
	MetricErrorBuildModel                 = "build_model_error"
	MetricErrorDeployStack                = "deploy_stack_error"
	MetricErrorReconcileGlobalAccelerator = "reconcile_globalaccelerator_error"
)

// NewGlobalAcceleratorReconciler constructs new globalAcceleratorReconciler
func NewGlobalAcceleratorReconciler(k8sClient client.Client, eventRecorder record.EventRecorder, finalizerManager k8s.FinalizerManager, config config.ControllerConfig, cloud services.Cloud, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector, reconcileCounters *metricsutil.ReconcileCounters) *globalAcceleratorReconciler {

	// Create tracking provider
	trackingProvider := tracking.NewDefaultProvider(agaTagPrefix, config.ClusterName, tracking.WithRegion(cloud.Region()))

	// Create model builder
	agaModelBuilder := aga.NewDefaultModelBuilder(
		k8sClient,
		eventRecorder,
		trackingProvider,
		config.FeatureGates,
		config.ClusterName,
		cloud.Region(),
		config.DefaultTags,
		config.ExternalManagedTags,
		logger.WithName("aga-model-builder"),
		metricsCollector,
	)

	// Create stack marshaller
	stackMarshaller := deploy.NewDefaultStackMarshaller()

	// Create AGA stack deployer
	stackDeployer := agadeploy.NewDefaultStackDeployer(cloud, config, trackingProvider, logger.WithName("aga-stack-deployer"), metricsCollector, controllerName)

	// Create status updater
	statusUpdater := agastatus.NewStatusUpdater(k8sClient, logger)

	// Create reference tracker for endpoint tracking
	referenceTracker := aga.NewReferenceTracker(logger.WithName("reference-tracker"))

	// Create DNS resolver
	dnsResolver, err := aga.NewDNSResolver(cloud.ELBV2())
	if err != nil {
		logger.Error(err, "Failed to create DNS resolver")
	}

	// Create unified endpoint loader
	endpointLoader := aga.NewEndpointLoader(k8sClient, dnsResolver, logger.WithName("endpoint-loader"))

	return &globalAcceleratorReconciler{
		k8sClient:        k8sClient,
		eventRecorder:    eventRecorder,
		finalizerManager: finalizerManager,
		logger:           logger,
		modelBuilder:     agaModelBuilder,
		stackMarshaller:  stackMarshaller,
		stackDeployer:    stackDeployer,
		statusUpdater:    statusUpdater,
		metricsCollector: metricsCollector,
		reconcileTracker: reconcileCounters.IncrementAGA,

		// Components for endpoint reference tracking
		referenceTracker: referenceTracker,
		dnsResolver:      dnsResolver,

		// Unified endpoint loader
		endpointLoader: endpointLoader,

		maxConcurrentReconciles:    config.GlobalAcceleratorMaxConcurrentReconciles,
		maxExponentialBackoffDelay: config.GlobalAcceleratorMaxExponentialBackoffDelay,
	}
}

// globalAcceleratorReconciler reconciles a GlobalAccelerator object
type globalAcceleratorReconciler struct {
	k8sClient        client.Client
	eventRecorder    record.EventRecorder
	finalizerManager k8s.FinalizerManager
	modelBuilder     aga.ModelBuilder
	stackMarshaller  deploy.StackMarshaller
	stackDeployer    agadeploy.StackDeployer
	statusUpdater    agastatus.StatusUpdater
	logger           logr.Logger
	metricsCollector lbcmetrics.MetricCollector
	reconcileTracker func(namespaceName ktypes.NamespacedName)

	// Components for endpoint reference tracking
	referenceTracker *aga.ReferenceTracker
	dnsResolver      *aga.DNSResolver

	// Unified endpoint loader
	endpointLoader aga.EndpointLoader

	// Resources manager for dedicated endpoint resource watchers
	endpointResourcesManager aga.EndpointResourcesManager

	// Event channels for dedicated watchers
	serviceEventChan chan event.GenericEvent
	ingressEventChan chan event.GenericEvent
	gatewayEventChan chan event.GenericEvent

	maxConcurrentReconciles    int
	maxExponentialBackoffDelay time.Duration
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

	reconcileResourceFn := func() {
		err = r.reconcileGlobalAcceleratorResources(ctx, ga)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, MetricStageReconcileGlobalAccelerator, reconcileResourceFn)
	if err != nil {
		return ctrlerrors.NewErrorWithMetrics(controllerName, MetricErrorReconcileGlobalAccelerator, err, r.metricsCollector)
	}
	return nil
}

func (r *globalAcceleratorReconciler) cleanupGlobalAccelerator(ctx context.Context, ga *agaapi.GlobalAccelerator) error {
	if k8s.HasFinalizer(ga, shared_constants.GlobalAcceleratorFinalizer) {
		// Clean up references in the reference tracker
		gaKey := k8s.NamespacedName(ga)
		r.referenceTracker.RemoveGA(gaKey)

		// Clean up resource watches
		r.endpointResourcesManager.RemoveGA(gaKey)

		// TODO: Implement cleanup logic for AWS Global Accelerator resources (Only cleaning up accelerator for now)
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
	r.logger.Info("Reconciling GlobalAccelerator resources", "globalAccelerator", k8s.NamespacedName(ga))

	// Get all endpoints from GA
	endpoints := aga.GetAllEndpointsFromGA(ga)

	// Track referenced endpoints
	r.referenceTracker.UpdateReferencesForGA(ga, endpoints)

	// Update resource watches with the endpointResourcesManager
	r.endpointResourcesManager.MonitorEndpointResources(ga, endpoints)

	// Validate and load endpoint status using the endpoint loader
	_, fatalErrors := r.endpointLoader.LoadEndpoints(ctx, ga, endpoints)
	if len(fatalErrors) > 0 {
		err := fmt.Errorf("failed to load endpoints: %v", fatalErrors[0])
		r.eventRecorder.Event(ga, corev1.EventTypeWarning, k8s.GlobalAcceleratorEventReasonFailedEndpointLoad, fmt.Sprintf("Failed to reconcile due to %v", err))
		r.logger.Error(err, fmt.Sprintf("fatal error loading endpoints for %v", k8s.NamespacedName(ga)))
		// Handle other endpoint loading errors
		if statusErr := r.statusUpdater.UpdateStatusFailure(ctx, ga, agadeploy.EndpointLoadFailed, err.Error()); statusErr != nil {
			r.logger.Error(statusErr, "Failed to update GlobalAccelerator status after endpoint load failure")
		}
		return err
	}

	var stack core.Stack
	var accelerator *agamodel.Accelerator
	var err error
	buildModelFn := func() {
		stack, accelerator, err = r.buildModel(ctx, ga)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, MetricStageBuildModel, buildModelFn)
	if err != nil {
		r.eventRecorder.Event(ga, corev1.EventTypeWarning, k8s.GatewayEventReasonFailedBuildModel, fmt.Sprintf("Failed to build model: %v", err))
		r.logger.Error(err, fmt.Sprintf("Failed to build model for: %v", k8s.NamespacedName(ga)))
		// Update status to indicate model building failure
		if statusErr := r.statusUpdater.UpdateStatusFailure(ctx, ga, agadeploy.ModelBuildFailed, fmt.Sprintf("Failed to build model: %v", err)); statusErr != nil {
			r.logger.Error(statusErr, "Failed to update GlobalAccelerator status after model build failure")
		}
		return ctrlerrors.NewErrorWithMetrics(controllerName, MetricErrorBuildModel, err, r.metricsCollector)
	}

	// Deploy the stack to create/update AWS Global Accelerator resources
	deployStackFn := func() {
		err = r.stackDeployer.Deploy(ctx, stack, r.metricsCollector, controllerName)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, MetricStageDeployStack, deployStackFn)
	if err != nil {
		r.eventRecorder.Event(ga, corev1.EventTypeWarning, k8s.GlobalAcceleratorEventReasonFailedDeploy, fmt.Sprintf("Failed to deploy stack due to %v", err))
		r.logger.Error(err, fmt.Sprintf("Failed to deploy stack for: %v", k8s.NamespacedName(ga)))
		// Update status to indicate deployment failure
		if statusErr := r.statusUpdater.UpdateStatusFailure(ctx, ga, agadeploy.DeploymentFailed, fmt.Sprintf("Failed to deploy stack: %v", err)); statusErr != nil {
			r.logger.Error(statusErr, "Failed to update GlobalAccelerator status after deployment failure")
		}

		return ctrlerrors.NewErrorWithMetrics(controllerName, MetricErrorDeployStack, err, r.metricsCollector)
	}

	r.logger.Info("Successfully deployed GlobalAccelerator stack", "stackID", stack.StackID())

	// Update GlobalAccelerator status after successful deployment
	requeueNeeded, err := r.statusUpdater.UpdateStatusSuccess(ctx, ga, accelerator)
	if err != nil {
		r.eventRecorder.Event(ga, corev1.EventTypeWarning, k8s.GlobalAcceleratorEventReasonFailedUpdateStatus, fmt.Sprintf("Failed update status due to %v", err))
		return err
	}
	if requeueNeeded {
		return ctrlerrors.NewRequeueNeededAfter(requeueMessage, statusUpdateRequeueTime)
	}

	r.eventRecorder.Event(ga, corev1.EventTypeNormal, k8s.GlobalAcceleratorEventReasonSuccessfullyReconciled, "Successfully reconciled")

	return nil
}

func (r *globalAcceleratorReconciler) cleanupGlobalAcceleratorResources(ctx context.Context, ga *agaapi.GlobalAccelerator) error {
	r.logger.Info("Cleaning up GlobalAccelerator resources", "globalAccelerator", k8s.NamespacedName(ga))

	// Our enhanced AcceleratorManager now handles deletion of listeners before accelerator.
	// TODO: This will be enhanced to delete endpoint groups and endpoints
	// before deleting listeners and accelerator (when those features are implemented)
	// 1. Find the accelerator ARN from the CRD status
	if ga.Status.AcceleratorARN == nil {
		r.logger.Info("No accelerator ARN found in status, nothing to clean up", "globalAccelerator", k8s.NamespacedName(ga))
		return nil
	}

	acceleratorARN := *ga.Status.AcceleratorARN
	if acceleratorARN == "" {
		r.logger.Info("Empty accelerator ARN in status, nothing to clean up", "globalAccelerator", k8s.NamespacedName(ga))
		return nil
	}

	// 2. Delete the accelerator using accelerator delete manager
	acceleratorManager := r.stackDeployer.GetAcceleratorManager()
	r.logger.Info("Deleting accelerator", "acceleratorARN", acceleratorARN, "globalAccelerator", k8s.NamespacedName(ga))

	// Initialize reference to existing accelerator for deletion
	acceleratorWithTags := agadeploy.AcceleratorWithTags{
		Accelerator: &types.Accelerator{
			AcceleratorArn: &acceleratorARN,
		},
		Tags: nil,
	}

	if err := acceleratorManager.Delete(ctx, acceleratorWithTags); err != nil {
		// Check if it's an AcceleratorNotDisabledError
		var notDisabledErr *agadeploy.AcceleratorNotDisabledError
		if errors.As(err, &notDisabledErr) {
			// Update status to indicate we're waiting for the accelerator to be disabled
			if updateErr := r.statusUpdater.UpdateStatusDeletion(ctx, ga); updateErr != nil {
				r.logger.Error(updateErr, "Failed to update status during accelerator deletion")
			}
			return ctrlerrors.NewRequeueNeeded("Waiting for accelerator to be disabled")
		}

		// Any other error
		r.logger.Error(err, "Failed to delete accelerator", "acceleratorARN", acceleratorARN, "globalAccelerator", k8s.NamespacedName(ga))
		return fmt.Errorf("failed to delete accelerator %s: %w", acceleratorARN, err)
	}

	r.logger.Info("Successfully cleaned up all GlobalAccelerator resources", "globalAccelerator", k8s.NamespacedName(ga))
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

	// Create event channels for dedicated watchers
	r.serviceEventChan = make(chan event.GenericEvent)
	r.ingressEventChan = make(chan event.GenericEvent)
	r.gatewayEventChan = make(chan event.GenericEvent)

	// Initialize Gateway API client using the same config
	gwClient, err := gwclientset.NewForConfig(mgr.GetConfig())
	if err != nil {
		r.logger.Error(err, "Failed to create Gateway API client")
		return err
	}

	// Initialize the endpoint resources manager with clients
	r.endpointResourcesManager = aga.NewEndpointResourcesManager(
		clientSet,
		gwClient,
		r.serviceEventChan,
		r.ingressEventChan,
		r.gatewayEventChan,
		r.logger.WithName("endpoint-resources-manager"),
	)

	if err := r.setupIndexes(ctx, mgr.GetFieldIndexer()); err != nil {
		return err
	}

	// Set up the controller builder
	ctrl, err := ctrl.NewControllerManagedBy(mgr).
		For(&agaapi.GlobalAccelerator{}).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.maxConcurrentReconciles,
			RateLimiter:             workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](5*time.Second, r.maxExponentialBackoffDelay),
		}).
		Build(r)

	if err != nil {
		return err
	}

	// Setup watches for resource events
	if err := r.setupGlobalAcceleratorWatches(ctrl); err != nil {
		return err
	}

	return nil
}

// setupGlobalAcceleratorWatches sets up watches for resources that can trigger reconciliation of GlobalAccelerator objects
func (r *globalAcceleratorReconciler) setupGlobalAcceleratorWatches(c controller.Controller) error {
	loggerPrefix := r.logger.WithName("eventHandlers")

	// Create handlers for our dedicated watchers
	serviceHandler := eventhandlers.NewEnqueueRequestsForResourceEvent(
		aga.ServiceResourceType,
		r.referenceTracker,
		loggerPrefix.WithName("service-handler"),
	)

	ingressHandler := eventhandlers.NewEnqueueRequestsForResourceEvent(
		aga.IngressResourceType,
		r.referenceTracker,
		loggerPrefix.WithName("ingress-handler"),
	)

	gatewayHandler := eventhandlers.NewEnqueueRequestsForResourceEvent(
		aga.GatewayResourceType,
		r.referenceTracker,
		loggerPrefix.WithName("gateway-handler"),
	)

	// Add watches using the channel sources with event handlers
	if err := c.Watch(source.Channel(r.serviceEventChan, serviceHandler)); err != nil {
		return err
	}

	if err := c.Watch(source.Channel(r.ingressEventChan, ingressHandler)); err != nil {
		return err
	}

	if err := c.Watch(source.Channel(r.gatewayEventChan, gatewayHandler)); err != nil {
		return err
	}

	return nil
}

func (r *globalAcceleratorReconciler) setupIndexes(ctx context.Context, fieldIndexer client.FieldIndexer) error {
	// TODO: Add field indexes for efficient lookups
	return nil
}
