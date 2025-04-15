package service

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/service/eventhandlers"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	errmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/error"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	metricsutil "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/util"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/service"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const (
	serviceFinalizer        = "service.k8s.aws/resources"
	serviceTagPrefix        = "service.k8s.aws"
	serviceAnnotationPrefix = "service.beta.kubernetes.io"
	controllerName          = "service"
)

func NewServiceReconciler(cloud services.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder,
	finalizerManager k8s.FinalizerManager, networkingSGManager networking.SecurityGroupManager,
	networkingSGReconciler networking.SecurityGroupReconciler, subnetsResolver networking.SubnetsResolver,
	vpcInfoProvider networking.VPCInfoProvider, elbv2TaggingManager elbv2deploy.TaggingManager, controllerConfig config.ControllerConfig,
	backendSGProvider networking.BackendSGProvider, sgResolver networking.SecurityGroupResolver, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector, reconcileCounters *metricsutil.ReconcileCounters) *serviceReconciler {

	annotationParser := annotations.NewSuffixAnnotationParser(serviceAnnotationPrefix)
	trackingProvider := tracking.NewDefaultProvider(serviceTagPrefix, controllerConfig.ClusterName)
	serviceUtils := service.NewServiceUtils(annotationParser, serviceFinalizer, controllerConfig.ServiceConfig.LoadBalancerClass, controllerConfig.FeatureGates)
	modelBuilder := service.NewDefaultModelBuilder(annotationParser, subnetsResolver, vpcInfoProvider, cloud.VpcID(), trackingProvider,
		elbv2TaggingManager, cloud.EC2(), controllerConfig.FeatureGates, controllerConfig.ClusterName, controllerConfig.DefaultTags, controllerConfig.ExternalManagedTags,
		controllerConfig.DefaultSSLPolicy, controllerConfig.DefaultTargetType, controllerConfig.DefaultLoadBalancerScheme, controllerConfig.FeatureGates.Enabled(config.EnableIPTargetType), serviceUtils,
		backendSGProvider, sgResolver, controllerConfig.EnableBackendSecurityGroup, controllerConfig.EnableManageBackendSecurityGroupRules, controllerConfig.DisableRestrictedSGRules, logger, metricsCollector)
	stackMarshaller := deploy.NewDefaultStackMarshaller()
	stackDeployer := deploy.NewDefaultStackDeployer(cloud, k8sClient, networkingSGManager, networkingSGReconciler, elbv2TaggingManager, controllerConfig, serviceTagPrefix, logger, metricsCollector, controllerName)
	return &serviceReconciler{
		k8sClient:         k8sClient,
		eventRecorder:     eventRecorder,
		finalizerManager:  finalizerManager,
		annotationParser:  annotationParser,
		loadBalancerClass: controllerConfig.ServiceConfig.LoadBalancerClass,
		serviceUtils:      serviceUtils,
		backendSGProvider: backendSGProvider,

		modelBuilder:    modelBuilder,
		stackMarshaller: stackMarshaller,
		stackDeployer:   stackDeployer,
		logger:          logger,

		maxConcurrentReconciles: controllerConfig.ServiceMaxConcurrentReconciles,
		metricsCollector:        metricsCollector,
		reconcileCounters:       reconcileCounters,
	}
}

type serviceReconciler struct {
	k8sClient         client.Client
	eventRecorder     record.EventRecorder
	finalizerManager  k8s.FinalizerManager
	annotationParser  annotations.Parser
	loadBalancerClass string
	serviceUtils      service.ServiceUtils
	backendSGProvider networking.BackendSGProvider

	modelBuilder      service.ModelBuilder
	stackMarshaller   deploy.StackMarshaller
	stackDeployer     deploy.StackDeployer
	logger            logr.Logger
	metricsCollector  lbcmetrics.MetricCollector
	reconcileCounters *metricsutil.ReconcileCounters

	maxConcurrentReconciles int
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=services/status,verbs=update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *serviceReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	r.reconcileCounters.IncrementService(req.NamespacedName)
	return runtime.HandleReconcileError(r.reconcile(ctx, req), r.logger)
}

func (r *serviceReconciler) reconcile(ctx context.Context, req reconcile.Request) error {
	svc := &corev1.Service{}
	var err error
	fetchServiceFn := func() {
		err = r.k8sClient.Get(ctx, req.NamespacedName, svc)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "fetch_service", fetchServiceFn)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	var stack core.Stack
	var lb *elbv2model.LoadBalancer
	var backendSGRequired bool
	buildModelFn := func() {
		stack, lb, backendSGRequired, err = r.buildModel(ctx, svc)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "build_model", buildModelFn)
	if err != nil {
		return errmetrics.NewErrorWithMetrics(controllerName, "build_model_error", err, r.metricsCollector)
	}

	if lb == nil {
		cleanupLoadBalancerFn := func() {
			err = r.cleanupLoadBalancerResources(ctx, svc, stack)
		}
		r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "cleanup_load_balancer", cleanupLoadBalancerFn)
		if err != nil {
			return errmetrics.NewErrorWithMetrics(controllerName, "cleanup_load_balancer_error", err, r.metricsCollector)
		}
	}
	return r.reconcileLoadBalancerResources(ctx, svc, stack, lb, backendSGRequired)
}

func (r *serviceReconciler) buildModel(ctx context.Context, svc *corev1.Service) (core.Stack, *elbv2model.LoadBalancer, bool, error) {
	stack, lb, backendSGRequired, err := r.modelBuilder.Build(ctx, svc, r.metricsCollector)
	if err != nil {
		r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, false, err
	}
	stackJSON, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, false, err
	}
	r.logger.Info("successfully built model", "model", stackJSON)
	return stack, lb, backendSGRequired, nil
}

func (r *serviceReconciler) deployModel(ctx context.Context, svc *corev1.Service, stack core.Stack) error {
	if err := r.stackDeployer.Deploy(ctx, stack, r.metricsCollector, "service"); err != nil {
		var requeueNeededAfter *runtime.RequeueNeededAfter
		if errors.As(err, &requeueNeededAfter) {
			return err
		}
		r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %v", err))
		return err
	}
	r.logger.Info("successfully deployed model", "service", k8s.NamespacedName(svc))

	return nil
}

func (r *serviceReconciler) reconcileLoadBalancerResources(ctx context.Context, svc *corev1.Service, stack core.Stack,
	lb *elbv2model.LoadBalancer, backendSGRequired bool) error {

	var err error
	addFinalizersFn := func() {
		err = r.finalizerManager.AddFinalizers(ctx, svc, serviceFinalizer)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "add_finalizers", addFinalizersFn)
	if err != nil {
		r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return errmetrics.NewErrorWithMetrics(controllerName, "add_finalizers_error", err, r.metricsCollector)
	}

	deployModelFn := func() {
		err = r.deployModel(ctx, svc, stack)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "deploy_model", deployModelFn)
	if err != nil {
		return errmetrics.NewErrorWithMetrics(controllerName, "deploy_model_error", err, r.metricsCollector)
	}

	var lbDNS string
	dnsResolveFn := func() {
		lbDNS, err = lb.DNSName().Resolve(ctx)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "DNS_resolve", dnsResolveFn)
	if err != nil {
		return errmetrics.NewErrorWithMetrics(controllerName, "dns_resolve_error", err, r.metricsCollector)
	}

	if !backendSGRequired {
		if err := r.backendSGProvider.Release(ctx, networking.ResourceTypeService, []types.NamespacedName{k8s.NamespacedName(svc)}); err != nil {
			return errmetrics.NewErrorWithMetrics(controllerName, "release_auto_generated_backend_sg_error", err, r.metricsCollector)
		}
	}

	updateStatusFn := func() {
		err = r.updateServiceStatus(ctx, lbDNS, svc)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "update_status", updateStatusFn)
	if err != nil {
		r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedUpdateStatus, fmt.Sprintf("Failed update status due to %v", err))
		return errmetrics.NewErrorWithMetrics(controllerName, "update_status_error", err, r.metricsCollector)
	}
	r.eventRecorder.Event(svc, corev1.EventTypeNormal, k8s.ServiceEventReasonSuccessfullyReconciled, "Successfully reconciled")
	return nil
}

func (r *serviceReconciler) cleanupLoadBalancerResources(ctx context.Context, svc *corev1.Service, stack core.Stack) error {
	if k8s.HasFinalizer(svc, serviceFinalizer) {
		err := r.deployModel(ctx, svc, stack)
		if err != nil {
			return err
		}
		if err := r.backendSGProvider.Release(ctx, networking.ResourceTypeService, []types.NamespacedName{k8s.NamespacedName(svc)}); err != nil {
			return err
		}
		if err = r.cleanupServiceStatus(ctx, svc); err != nil {
			r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedCleanupStatus, fmt.Sprintf("Failed update status due to %v", err))
			return err
		}
		if err := r.finalizerManager.RemoveFinalizers(ctx, svc, serviceFinalizer); err != nil {
			r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedRemoveFinalizer, fmt.Sprintf("Failed remove finalizer due to %v", err))
			return err
		}
	}
	return nil
}

func (r *serviceReconciler) updateServiceStatus(ctx context.Context, lbDNS string, svc *corev1.Service) error {
	if len(svc.Status.LoadBalancer.Ingress) != 1 ||
		svc.Status.LoadBalancer.Ingress[0].IP != "" ||
		svc.Status.LoadBalancer.Ingress[0].Hostname != lbDNS {
		svcOld := svc.DeepCopy()
		svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
			{
				Hostname: lbDNS,
			},
		}
		if err := r.k8sClient.Status().Patch(ctx, svc, client.MergeFrom(svcOld)); err != nil {
			return errors.Wrapf(err, "failed to update service status: %v", k8s.NamespacedName(svc))
		}
	}
	return nil
}

func (r *serviceReconciler) cleanupServiceStatus(ctx context.Context, svc *corev1.Service) error {
	svcOld := svc.DeepCopy()
	svc.Status.LoadBalancer = corev1.LoadBalancerStatus{}
	if err := r.k8sClient.Status().Patch(ctx, svc, client.MergeFrom(svcOld)); err != nil {
		return errors.Wrapf(err, "failed to cleanup service status: %v", k8s.NamespacedName(svc))
	}
	return nil
}

func (r *serviceReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	svcEventHandler := eventhandlers.NewEnqueueRequestForServiceEvent(r.eventRecorder,
		r.serviceUtils, r.logger.WithName("eventHandlers").WithName("service"))

	return ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		Watches(&corev1.Service{}, svcEventHandler).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.maxConcurrentReconciles,
		}).
		Complete(r)
}
