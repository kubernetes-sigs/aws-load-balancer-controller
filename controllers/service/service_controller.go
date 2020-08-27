package service

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/aws-alb-ingress-controller/controllers/service/eventhandlers"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/annotations"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	LoadBalancerFinalizer   = "service.k8s.aws/load-balancer-finalizer"
	ServiceAnnotationPrefix = "service.beta.kubernetes.io"
	controllerName          = "service"
)

func NewServiceReconciler(k8sClient client.Client, log logr.Logger) *ServiceReconciler {
	return &ServiceReconciler{
		k8sClient:        k8sClient,
		log:              log,
		annotationParser: annotations.NewSuffixAnnotationParser(ServiceAnnotationPrefix),
		finalizerManager: k8s.NewDefaultFinalizerManager(k8sClient, log),
	}
}

type ServiceReconciler struct {
	k8sClient        client.Client
	log              logr.Logger
	annotationParser annotations.Parser
	finalizerManager k8s.FinalizerManager
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
func (r *ServiceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.log.WithValues("service", req.NamespacedName)
	svc := &corev1.Service{}
	if err := r.k8sClient.Get(ctx, req.NamespacedName, svc); err != nil {
		if k8serr.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if err := r.finalizerManager.AddFinalizers(ctx, svc, LoadBalancerFinalizer); err != nil {
		return reconcile.Result{}, err
	}
	// TODO: Build & Deploy model
	// TODO: Update status

	if !svc.DeletionTimestamp.IsZero() {
		if err := r.finalizerManager.RemoveFinalizers(ctx, svc, LoadBalancerFinalizer); err != nil {
			return reconcile.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) updateServiceStatus(ctx context.Context, svc *corev1.Service, lbDNS string) error {
	if len(svc.Status.LoadBalancer.Ingress) != 1 || svc.Status.LoadBalancer.Ingress[0].IP != "" || svc.Status.LoadBalancer.Ingress[0].Hostname != lbDNS {
		svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
			{
				Hostname: lbDNS,
			},
		}
		if err := r.k8sClient.Status().Update(ctx, svc); err != nil {
			return errors.Wrapf(err, "failed to update service:%v", svc)
		}
		return r.k8sClient.Status().Update(ctx, svc)
	}
	return nil
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c, err := controller.New(controllerName, mgr, controller.Options{
		MaxConcurrentReconciles: 1,
		Reconciler:              r,
	})
	if err != nil {
		return err
	}
	return r.setupWatches(mgr, c)
}

func (r *ServiceReconciler) setupWatches(mgr ctrl.Manager, c controller.Controller) error {
	svcEventHandler := eventhandlers.NewEnqueueRequestForServiceEvent(mgr.GetEventRecorderFor(controllerName), r.annotationParser)
	if err:= c.Watch(&source.Kind{Type: &corev1.Service{}}, svcEventHandler); err != nil {
		return err
	}
	return nil
}
