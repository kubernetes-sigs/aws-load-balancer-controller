package service

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/service/eventhandlers"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/service"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	serviceFinalizer        = "service.k8s.aws/resources"
	serviceTagPrefix        = "service.k8s.aws"
	serviceAnnotationPrefix = "service.beta.kubernetes.io"
	controllerName          = "service"
)

func NewServiceReconciler(cloud aws.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder,
	finalizerManager k8s.FinalizerManager, networkingSGManager networking.SecurityGroupManager,
	networkingSGReconciler networking.SecurityGroupReconciler, subnetsResolver networking.SubnetsResolver,
	vpcInfoProvider networking.VPCInfoProvider, controllerConfig config.ControllerConfig, logger logr.Logger) *serviceReconciler {

	annotationParser := annotations.NewSuffixAnnotationParser(serviceAnnotationPrefix)
	trackingProvider := tracking.NewDefaultProvider(serviceTagPrefix, controllerConfig.ClusterName)
	elbv2TaggingManager := elbv2.NewDefaultTaggingManager(cloud.ELBV2(), cloud.VpcID(), controllerConfig.FeatureGates, logger)
	serviceUtils := service.NewServiceUtils(annotationParser, serviceFinalizer, controllerConfig.ServiceConfig.LoadBalancerClass, controllerConfig.FeatureGates)
	modelBuilder := service.NewDefaultModelBuilder(annotationParser, subnetsResolver, vpcInfoProvider, cloud.VpcID(), trackingProvider,
		elbv2TaggingManager, controllerConfig.FeatureGates, controllerConfig.ClusterName, controllerConfig.DefaultTags, controllerConfig.ExternalManagedTags, controllerConfig.DefaultSSLPolicy, controllerConfig.FeatureGates.Enabled(config.EnableIPTargetType), serviceUtils)
	stackMarshaller := deploy.NewDefaultStackMarshaller()
	stackDeployer := deploy.NewDefaultStackDeployer(cloud, k8sClient, networkingSGManager, networkingSGReconciler, controllerConfig, serviceTagPrefix, logger)
	return &serviceReconciler{
		k8sClient:         k8sClient,
		eventRecorder:     eventRecorder,
		finalizerManager:  finalizerManager,
		annotationParser:  annotationParser,
		loadBalancerClass: controllerConfig.ServiceConfig.LoadBalancerClass,
		serviceUtils:      serviceUtils,

		modelBuilder:    modelBuilder,
		stackMarshaller: stackMarshaller,
		stackDeployer:   stackDeployer,
		logger:          logger,

		maxConcurrentReconciles: controllerConfig.ServiceMaxConcurrentReconciles,
	}
}

type serviceReconciler struct {
	k8sClient         client.Client
	eventRecorder     record.EventRecorder
	finalizerManager  k8s.FinalizerManager
	annotationParser  annotations.Parser
	loadBalancerClass string
	serviceUtils      service.ServiceUtils

	modelBuilder    service.ModelBuilder
	stackMarshaller deploy.StackMarshaller
	stackDeployer   deploy.StackDeployer
	logger          logr.Logger

	maxConcurrentReconciles int
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=services/status,verbs=update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *serviceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return runtime.HandleReconcileError(r.reconcile(ctx, req), r.logger)
}

func (r *serviceReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	svc := &corev1.Service{}
	if err := r.k8sClient.Get(ctx, req.NamespacedName, svc); err != nil {
		return client.IgnoreNotFound(err)
	}
	stack, lb, err := r.buildModel(ctx, svc)
	if err != nil {
		return err
	}
	if lb == nil {
		return r.cleanupLoadBalancerResources(ctx, svc, stack)
	}
	return r.reconcileLoadBalancerResources(ctx, svc, stack, lb)
}

func (r *serviceReconciler) buildModel(ctx context.Context, svc *corev1.Service) (core.Stack, *elbv2model.LoadBalancer, error) {
	stack, lb, err := r.modelBuilder.Build(ctx, svc)
	if err != nil {
		r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, err
	}
	stackJSON, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, err
	}
	r.logger.Info("successfully built model", "model", stackJSON)
	return stack, lb, nil
}

func (r *serviceReconciler) deployModel(ctx context.Context, svc *corev1.Service, stack core.Stack) error {
	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %v", err))
		return err
	}
	r.logger.Info("successfully deployed model", "service", k8s.NamespacedName(svc))

	return nil
}

func (r *serviceReconciler) reconcileLoadBalancerResources(ctx context.Context, svc *corev1.Service, stack core.Stack, lb *elbv2model.LoadBalancer) error {
	if err := r.finalizerManager.AddFinalizers(ctx, svc, serviceFinalizer); err != nil {
		r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return err
	}
	err := r.deployModel(ctx, svc, stack)
	if err != nil {
		return err
	}
	lbDNS, err := lb.DNSName().Resolve(ctx)
	if err != nil {
		return err
	}

	if err = r.updateServiceStatus(ctx, lbDNS, svc); err != nil {
		r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedUpdateStatus, fmt.Sprintf("Failed update status due to %v", err))
		return err
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
	c, err := controller.New(controllerName, mgr, controller.Options{
		MaxConcurrentReconciles: r.maxConcurrentReconciles,
		Reconciler:              r,
	})
	if err != nil {
		return err
	}
	if err := r.setupWatches(ctx, c); err != nil {
		return err
	}
	return nil
}

func (r *serviceReconciler) setupWatches(_ context.Context, c controller.Controller) error {
	svcEventHandler := eventhandlers.NewEnqueueRequestForServiceEvent(r.eventRecorder,
		r.serviceUtils, r.logger.WithName("eventHandlers").WithName("service"))
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, svcEventHandler); err != nil {
		return err
	}
	return nil
}
