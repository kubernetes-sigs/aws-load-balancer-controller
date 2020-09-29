package ingress

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
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
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "ingress"

// NewGroupReconciler constructs new GroupReconciler
func NewGroupReconciler(cloud aws.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder,
	networkingSGManager networkingpkg.SecurityGroupManager, networkingSGReconciler networkingpkg.SecurityGroupReconciler, clusterName string,
	subnetsResolver networkingpkg.SubnetsResolver, logger logr.Logger) *GroupReconciler {
	annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
	authConfigBuilder := ingress.NewDefaultAuthConfigBuilder(annotationParser)
	enhancedBackendBuilder := ingress.NewDefaultEnhancedBackendBuilder(annotationParser)
	modelBuilder := ingress.NewDefaultModelBuilder(k8sClient, eventRecorder,
		cloud.EC2(), cloud.ACM(),
		annotationParser, subnetsResolver,
		authConfigBuilder, enhancedBackendBuilder,
		cloud.VpcID(), clusterName, logger)
	stackMarshaller := deploy.NewDefaultStackMarshaller()
	stackDeployer := deploy.NewDefaultStackDeployer(cloud, k8sClient, networkingSGManager, networkingSGReconciler, clusterName, "ingress.k8s.aws", logger)
	groupLoader := ingress.NewDefaultGroupLoader(k8sClient, annotationParser, "alb")
	finalizerManager := ingress.NewDefaultFinalizerManager(k8sClient)

	return &GroupReconciler{
		k8sClient:        k8sClient,
		groupLoader:      groupLoader,
		finalizerManager: finalizerManager,
		modelBuilder:     modelBuilder,
		stackMarshaller:  stackMarshaller,
		stackDeployer:    stackDeployer,
		log:              logger,
	}
}

// GroupReconciler reconciles a ingress group
type GroupReconciler struct {
	k8sClient       client.Client
	modelBuilder    ingress.ModelBuilder
	stackMarshaller deploy.StackMarshaller
	stackDeployer   deploy.StackDeployer

	groupLoader      ingress.GroupLoader
	finalizerManager ingress.FinalizerManager
	log              logr.Logger
}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=extensions,resources=ingresses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=extensions,resources=ingresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;update;patch;create;delete

// Reconcile
func (r *GroupReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	ingGroupID := ingress.DecodeGroupIDFromReconcileRequest(req)
	_ = r.log.WithValues("groupID", ingGroupID)
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
	r.log.Info("successfully built model", "model", stackJSON)
	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		return ctrl.Result{}, err
	}
	r.log.Info("successfully deployed model")

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

func (r *GroupReconciler) updateIngressGroupStatus(ctx context.Context, ingGroup ingress.Group, lbDNS string) error {
	for _, ing := range ingGroup.Members {
		if err := r.updateIngressStatus(ctx, ing, lbDNS); err != nil {
			return err
		}
	}
	return nil
}

func (r *GroupReconciler) updateIngressStatus(ctx context.Context, ing *networking.Ingress, lbDNS string) error {
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

func (r *GroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c, err := controller.New(controllerName, mgr, controller.Options{
		MaxConcurrentReconciles: 1,
		Reconciler:              r,
	})
	if err != nil {
		return err
	}
	return r.setupWatches(mgr, c, r.groupLoader)
}

func (r *GroupReconciler) setupWatches(mgr ctrl.Manager, c controller.Controller, groupLoader ingress.GroupLoader) error {
	ingEventHandler := eventhandlers.NewEnqueueRequestsForIngressEvent(groupLoader, mgr.GetEventRecorderFor(controllerName))
	if err := c.Watch(&source.Kind{Type: &networking.Ingress{}}, ingEventHandler); err != nil {
		return err
	}
	return nil
}
