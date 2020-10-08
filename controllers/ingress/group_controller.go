package ingress

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/ingress/eventhandlers"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	networkingpkg "sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	ingressTagPrefix        = "ingress.k8s.aws"
	ingressAnnotationPrefix = "alb.ingress.kubernetes.io"
	controllerName          = "ingress"
)

// NewGroupReconciler constructs new GroupReconciler
func NewGroupReconciler(cloud aws.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder,
	networkingSGManager networkingpkg.SecurityGroupManager, networkingSGReconciler networkingpkg.SecurityGroupReconciler, clusterName string,
	subnetsResolver networkingpkg.SubnetsResolver, logger logr.Logger) *groupReconciler {
	annotationParser := annotations.NewSuffixAnnotationParser(ingressAnnotationPrefix)
	authConfigBuilder := ingress.NewDefaultAuthConfigBuilder(annotationParser)
	enhancedBackendBuilder := ingress.NewDefaultEnhancedBackendBuilder(annotationParser)
	referenceIndexer := ingress.NewDefaultReferenceIndexer(enhancedBackendBuilder, authConfigBuilder, logger)
	modelBuilder := ingress.NewDefaultModelBuilder(k8sClient, eventRecorder,
		cloud.EC2(), cloud.ACM(),
		annotationParser, subnetsResolver,
		authConfigBuilder, enhancedBackendBuilder,
		cloud.VpcID(), clusterName, logger)
	stackMarshaller := deploy.NewDefaultStackMarshaller()
	stackDeployer := deploy.NewDefaultStackDeployer(cloud, k8sClient, networkingSGManager, networkingSGReconciler, clusterName, ingressTagPrefix, logger)
	groupLoader := ingress.NewDefaultGroupLoader(k8sClient, annotationParser, "alb")
	k8sFinalizerManager := k8s.NewDefaultFinalizerManager(k8sClient, logger)
	finalizerManager := ingress.NewDefaultFinalizerManager(k8sFinalizerManager)

	return &groupReconciler{
		k8sClient:        k8sClient,
		eventRecorder:    eventRecorder,
		groupLoader:      groupLoader,
		finalizerManager: finalizerManager,
		referenceIndexer: referenceIndexer,
		modelBuilder:     modelBuilder,
		stackMarshaller:  stackMarshaller,
		stackDeployer:    stackDeployer,
		logger:           logger,
	}
}

// GroupReconciler reconciles a ingress group
type groupReconciler struct {
	k8sClient        client.Client
	eventRecorder    record.EventRecorder
	referenceIndexer ingress.ReferenceIndexer
	modelBuilder     ingress.ModelBuilder
	stackMarshaller  deploy.StackMarshaller
	stackDeployer    deploy.StackDeployer

	groupLoader      ingress.GroupLoader
	finalizerManager ingress.FinalizerManager
	logger           logr.Logger
}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=extensions,resources=ingresses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=extensions,resources=ingresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;update;patch;create;delete

// Reconcile
func (r *groupReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	ingGroupID := ingress.DecodeGroupIDFromReconcileRequest(req)
	_ = r.logger.WithValues("groupID", ingGroupID)
	ingGroup, err := r.groupLoader.Load(ctx, ingGroupID)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.finalizerManager.AddGroupFinalizer(ctx, ingGroupID, ingGroup.Members...); err != nil {
		return ctrl.Result{}, err
	}
	stack, lb, err := r.modelBuilder.Build(ctx, ingGroup)
	if err != nil {
		return ctrl.Result{}, err
	}
	stackJSON, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.logger.Info("successfully built model", "model", stackJSON)
	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		return ctrl.Result{}, err
	}
	r.logger.Info("successfully deployed model")

	if len(ingGroup.Members) > 0 {
		lbDNS, err := lb.DNSName().Resolve(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := r.updateIngressGroupStatus(ctx, ingGroup, lbDNS); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.finalizerManager.RemoveGroupFinalizer(ctx, ingGroupID, ingGroup.InactiveMembers...); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *groupReconciler) updateIngressGroupStatus(ctx context.Context, ingGroup ingress.Group, lbDNS string) error {
	for _, ing := range ingGroup.Members {
		if err := r.updateIngressStatus(ctx, ing, lbDNS); err != nil {
			return err
		}
	}
	return nil
}

func (r *groupReconciler) updateIngressStatus(ctx context.Context, ing *networking.Ingress, lbDNS string) error {
	if len(ing.Status.LoadBalancer.Ingress) != 1 ||
		ing.Status.LoadBalancer.Ingress[0].IP != "" ||
		ing.Status.LoadBalancer.Ingress[0].Hostname != lbDNS {
		ingOld := ing.DeepCopy()
		ing.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
			{
				Hostname: lbDNS,
			},
		}
		if err := r.k8sClient.Status().Patch(ctx, ing, client.MergeFrom(ingOld)); err != nil {
			return errors.Wrapf(err, "failed to update ingress status: %v", k8s.NamespacedName(ing))
		}
	}
	return nil
}

func (r *groupReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	c, err := controller.New(controllerName, mgr, controller.Options{
		MaxConcurrentReconciles: 3,
		Reconciler:              r,
	})
	if err != nil {
		return err
	}
	if err := r.setupIndexes(ctx, mgr.GetFieldIndexer()); err != nil {
		return err
	}
	if err := r.setupWatches(ctx, c); err != nil {
		return err
	}
	return nil
}

func (r *groupReconciler) setupIndexes(ctx context.Context, fieldIndexer client.FieldIndexer) error {
	if err := fieldIndexer.IndexField(ctx, &networking.Ingress{}, ingress.IndexKeyServiceRefName,
		func(obj runtime.Object) []string {
			return r.referenceIndexer.BuildServiceRefIndexes(context.Background(), obj.(*networking.Ingress))
		},
	); err != nil {
		return err
	}
	if err := fieldIndexer.IndexField(ctx, &networking.Ingress{}, ingress.IndexKeySecretRefName,
		func(obj runtime.Object) []string {
			return r.referenceIndexer.BuildSecretRefIndexes(context.Background(), obj.(*networking.Ingress))
		},
	); err != nil {
		return err
	}
	if err := fieldIndexer.IndexField(ctx, &corev1.Service{}, ingress.IndexKeySecretRefName,
		func(obj runtime.Object) []string {
			return r.referenceIndexer.BuildSecretRefIndexes(context.Background(), obj.(*corev1.Service))
		},
	); err != nil {
		return err
	}
	return nil
}

func (r *groupReconciler) setupWatches(_ context.Context, c controller.Controller) error {
	ingEventChan := make(chan event.GenericEvent)
	svcEventChan := make(chan event.GenericEvent)
	ingEventHandler := eventhandlers.NewEnqueueRequestsForIngressEvent(r.groupLoader, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("ingress"))
	svcEventHandler := eventhandlers.NewEnqueueRequestsForServiceEvent(ingEventChan, r.k8sClient, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("service"))
	secretEventHandler := eventhandlers.NewEnqueueRequestsForSecretEvent(ingEventChan, svcEventChan, r.k8sClient, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("secret"))

	if err := c.Watch(&source.Channel{Source: ingEventChan}, ingEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Channel{Source: svcEventChan}, svcEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &networking.Ingress{}}, ingEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, svcEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, secretEventHandler); err != nil {
		return err
	}
	return nil
}
