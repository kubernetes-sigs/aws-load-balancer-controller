package controller

import (
	"context"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler reconciles an single ingress object
type Reconciler struct {
	client   client.Client
	cache    cache.Cache
	recorder record.EventRecorder

	// TODO: move things out of store, and start to rely on functionality provided by client & cache
	store        store.Storer
	lbController lb.NLBController

	metricCollector metric.Collector
}

// Reconcile will reconcile the aws resources with k8s state of ingress.
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()
	service := &corev1.Service{}
	if err := r.cache.Get(ctx, request.NamespacedName, service); err != nil {
		if !errors.IsNotFound(err) {
			r.metricCollector.IncReconcileErrorCount(request.NamespacedName.String())
			return reconcile.Result{}, err
		}

		if err := r.deleteService(ctx, request.NamespacedName); err != nil {
			r.metricCollector.IncReconcileErrorCount(request.NamespacedName.String())
			return reconcile.Result{}, err
		}

		r.metricCollector.IncReconcileCount()
		return reconcile.Result{}, nil
	}

	if err := r.reconcileIngress(ctx, request.NamespacedName, service); err != nil {
		r.metricCollector.IncReconcileErrorCount(request.NamespacedName.String())
		return reconcile.Result{}, err
	}

	r.metricCollector.IncReconcileCount()
	return reconcile.Result{}, nil
}

func (r *Reconciler) reconcileIngress(ctx context.Context, serviceKey types.NamespacedName, service *corev1.Service) error {
	ctx = r.buildReconcileContext(ctx, serviceKey, service)
	lbInfo, err := r.lbController.Reconcile(ctx, service)
	if err != nil {
		return err
	}
	if err := r.updateServiceStatus(ctx, service, lbInfo); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) deleteService(ctx context.Context, serviceKey types.NamespacedName) error {
	ctx = r.buildReconcileContext(ctx, serviceKey, nil)
	if err := r.lbController.Delete(ctx, serviceKey); err != nil {
		return err
	}
	return nil
}

func (r *Reconciler) updateServiceStatus(ctx context.Context, service *corev1.Service, lbInfo *lb.LoadBalancer) error {
	if len(service.Status.LoadBalancer.Ingress) != 1 ||
		service.Status.LoadBalancer.Ingress[0].IP != "" ||
		service.Status.LoadBalancer.Ingress[0].Hostname != lbInfo.DNSName {
		service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
			{
				Hostname: lbInfo.DNSName,
			},
		}
		return r.client.Status().Update(ctx, service)
	}
	return nil
}

func (r *Reconciler) buildReconcileContext(ctx context.Context, ingressKey types.NamespacedName, service *corev1.Service) context.Context {
	ctx = albctx.SetLogger(ctx, log.New(ingressKey.String()))
	if service != nil {
		ctx = albctx.SetEventf(ctx, func(eventType string, reason string, messageFmt string, args ...interface{}) {
			r.recorder.Eventf(service, eventType, reason, messageFmt, args...)
		})
	}
	return ctx
}
