package controller

import (
	"fmt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/generator"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/ls"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/rs"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/sg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albwafregional"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/backend"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/handlers"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func Initialize(config *config.Configuration, mgr manager.Manager, mc metric.Collector) error {
	reconciler, err := newReconciler(config, mgr, mc)
	if err != nil {
		return err
	}
	c, err := controller.New("alb-ingress-controller", mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return err
	}
	if err := config.BindDynamicSettings(mgr, c, albec2.EC2svc); err != nil {
		return err
	}

	if err := watchClusterEvents(c, mgr.GetCache(), config.IngressClass); err != nil {
		return fmt.Errorf("failed to watch cluster events due to %v", err)
	}

	return nil
}

func newReconciler(config *config.Configuration, mgr manager.Manager, mc metric.Collector) (reconcile.Reconciler, error) {
	store, err := store.New(mgr, config)
	if err != nil {
		return nil, err
	}
	nameTagGenerator := generator.NewNameTagGenerator(*config)
	tagsController := tags.NewController(albec2.EC2svc, albelbv2.ELBV2svc, albrgt.RGTsvc)
	endpointResolver := backend.NewEndpointResolver(store, albec2.EC2svc)
	tgGroupController := tg.NewGroupController(albelbv2.ELBV2svc, albrgt.RGTsvc, store, nameTagGenerator, tagsController, endpointResolver)
	rsController := rs.NewController(albelbv2.ELBV2svc)
	lsGroupController := ls.NewGroupController(store, albelbv2.ELBV2svc, rsController)
	sgAssociationController := sg.NewAssociationController(store, albec2.EC2svc, albelbv2.ELBV2svc)
	lbController := lb.NewController(albelbv2.ELBV2svc, albrgt.RGTsvc, albwafregional.WAFRegionalsvc, store,
		nameTagGenerator, tgGroupController, lsGroupController, sgAssociationController)

	return &Reconciler{
		client:          mgr.GetClient(),
		cache:           mgr.GetCache(),
		recorder:        mgr.GetRecorder("alb-ingress-controller"),
		store:           store,
		lbController:    lbController,
		metricCollector: mc,
	}, nil
}

func watchClusterEvents(c controller.Controller, cache cache.Cache, ingressClass string) error {
	if err := c.Watch(&source.Kind{Type: &extensions.Ingress{}}, &handlers.EnqueueRequestsForIngressEvent{
		IngressClass: ingressClass,
	}); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handlers.EnqueueRequestsForServiceEvent{
		IngressClass: ingressClass,
		Cache:        cache,
	}); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Endpoints{}}, &handlers.EnqueueRequestsForEndpointsEvent{
		IngressClass: ingressClass,
		Cache:        cache,
	}); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Node{}}, &handlers.EnqueueRequestsForNodeEvent{
		IngressClass: ingressClass,
		Cache:        cache,
	}); err != nil {
		return err
	}
	return nil
}
