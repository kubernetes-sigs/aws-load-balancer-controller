package ingress

import (
	"context"
	"github.com/go-logr/logr"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/aws-alb-ingress-controller/controllers/ingress/eventhandlers"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/annotations"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

const controllerName = "ingress"

// NewGroupReconciler constructs new GroupReconciler
func NewGroupReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger) *GroupReconciler {
	annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
	groupLoader := ingress.NewDefaultGroupLoader(client, annotationParser, "alb")
	finalizerManager := ingress.NewDefaultFinalizerManager(client)

	return &GroupReconciler{
		client: client,
		scheme: scheme,
		log:    log,

		groupLoader:      groupLoader,
		finalizerManager: finalizerManager,
	}
}

// GroupReconciler reconciles a ingress group
type GroupReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger

	groupLoader      ingress.GroupLoader
	finalizerManager ingress.FinalizerManager
}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=Ingress,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=Ingress/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=extensions,resources=Ingress,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=extensions,resources=Ingress/status,verbs=get;update;patch
// Reconcile
func (r *GroupReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	groupID := ingress.DecodeGroupIDFromReconcileRequest(req)
	_ = r.log.WithValues("groupID", groupID)
	group, err := r.groupLoader.Load(ctx, groupID)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.finalizerManager.AddGroupFinalizer(ctx, groupID, group.Members...); err != nil {
		return ctrl.Result{}, err
	}

	// TODO: real reconcile logic here
	time.Sleep(10 * time.Second)

	if err := r.finalizerManager.RemoveGroupFinalizer(ctx, groupID, group.InactiveMembers...); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
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
