package service

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/service/eventhandlers"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/service/nlb"
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
	config config.ControllerConfig, logger logr.Logger) *serviceReconciler {

	annotationParser := annotations.NewSuffixAnnotationParser(serviceAnnotationPrefix)
	modelBuilder := nlb.NewDefaultModelBuilder(annotationParser, subnetsResolver, config.ClusterName)
	stackMarshaller := deploy.NewDefaultStackMarshaller()
	stackDeployer := deploy.NewDefaultStackDeployer(cloud, k8sClient, networkingSGManager, networkingSGReconciler, config, serviceTagPrefix, logger)
	return &serviceReconciler{
		k8sClient:        k8sClient,
		eventRecorder:    eventRecorder,
		finalizerManager: finalizerManager,
		annotationParser: annotationParser,

		modelBuilder:    modelBuilder,
		stackMarshaller: stackMarshaller,
		stackDeployer:   stackDeployer,
		logger:          logger,

		maxConcurrentReconciles: config.ServiceMaxConcurrentReconciles,
	}
}

type serviceReconciler struct {
	k8sClient        client.Client
	eventRecorder    record.EventRecorder
	finalizerManager k8s.FinalizerManager
	annotationParser annotations.Parser

	modelBuilder    nlb.ModelBuilder
	stackMarshaller deploy.StackMarshaller
	stackDeployer   deploy.StackDeployer
	logger          logr.Logger

	maxConcurrentReconciles int
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=services/status,verbs=update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *serviceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	return runtime.HandleReconcileError(r.reconcile(req), r.logger)
}

func (r *serviceReconciler) reconcile(req ctrl.Request) error {
	ctx := context.Background()
	svc := &corev1.Service{}
	if err := r.k8sClient.Get(ctx, req.NamespacedName, svc); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !svc.DeletionTimestamp.IsZero() {
		return r.cleanupLoadBalancerResources(ctx, svc)
	}
	return r.reconcileLoadBalancerResources(ctx, svc)
}

func (r *serviceReconciler) buildAndDeployModel(ctx context.Context, svc *corev1.Service) (core.Stack, *elbv2model.LoadBalancer, error) {
	stack, lb, err := r.modelBuilder.Build(ctx, svc)
	if err != nil {
		return nil, nil, err
	}
	stackJSON, err := r.stackMarshaller.Marshal(stack)
	r.logger.Info("successfully built model", "model", stackJSON)

	if err = r.stackDeployer.Deploy(ctx, stack); err != nil {
		return nil, nil, err
	}
	r.logger.Info("successfully deployed model", "service", k8s.NamespacedName(svc))

	return stack, lb, nil
}

func (r *serviceReconciler) reconcileLoadBalancerResources(ctx context.Context, svc *corev1.Service) error {
	if err := r.finalizerManager.AddFinalizers(ctx, svc, serviceFinalizer); err != nil {
		return err
	}
	_, lb, err := r.buildAndDeployModel(ctx, svc)
	if err != nil {
		return err
	}
	lbDNS, err := lb.DNSName().Resolve(ctx)
	if err != nil {
		return err
	}

	if err = r.updateServiceStatus(ctx, lbDNS, svc); err != nil {
		return err
	}
	return nil
}

func (r *serviceReconciler) cleanupLoadBalancerResources(ctx context.Context, svc *corev1.Service) error {
	if k8s.HasFinalizer(svc, serviceFinalizer) {
		_, _, err := r.buildAndDeployModel(ctx, svc)
		if err != nil {
			return err
		}
		if err := r.finalizerManager.RemoveFinalizers(ctx, svc, serviceFinalizer); err != nil {
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
	svcEventHandler := eventhandlers.NewEnqueueRequestForServiceEvent(r.eventRecorder, r.annotationParser,
		r.logger.WithName("eventHandlers").WithName("service"))
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, svcEventHandler); err != nil {
		return err
	}
	return nil
}
