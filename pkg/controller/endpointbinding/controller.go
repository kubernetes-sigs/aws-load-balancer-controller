package endpointbinding

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/controller/endpointbinding/eventhandlers"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func Initialize(mgr manager.Manager, cloud cloud.Cloud, ebRepo backend.EndpointBindingRepo) error {
	reconciler := newReconciler(mgr, cloud, ebRepo, mgr.GetCache())
	c, err := controller.New("endpoint-binding", mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return err
	}

	if err := watchClusterEvents(c, mgr.GetCache(), ebRepo); err != nil {
		return fmt.Errorf("failed to watch cluster events due to %v", err)
	}

	return nil
}

func watchClusterEvents(c controller.Controller, cache cache.Cache, ebRepo backend.EndpointBindingRepo) error {
	if err := watchEndpointBindingRepo(c, ebRepo); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Endpoints{}}, eventhandlers.NewEnqueueRequestsForEndpointsEvent(ebRepo)); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Node{}}, eventhandlers.NewEnqueueRequestsForNodeEvent(ebRepo, cache)); err != nil {
		return err
	}
	return nil
}

func watchEndpointBindingRepo(c controller.Controller, ebRepo backend.EndpointBindingRepo) error {
	watcher, err := ebRepo.Watch(context.Background())
	if err != nil {
		return err
	}
	ebEventChan := make(chan event.GenericEvent)
	go func() {
		for item := range watcher.ResultChan() {
			objMeta, _ := meta.Accessor(item.Object)
			ebEventChan <- event.GenericEvent{
				Meta:   objMeta,
				Object: item.Object,
			}
		}
		close(ebEventChan)
	}()
	if err := c.Watch(&source.Channel{Source: ebEventChan}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}
	return nil
}
