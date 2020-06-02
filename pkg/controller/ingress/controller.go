/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ingress

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/controller/ingress/eventhandlers"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func Initialize(mgr manager.Manager, cloud cloud.Cloud, ebRepo backend.EndpointBindingRepo, config ingress.Config) error {
	annotationParser := k8s.NewSuffixAnnotationParser(config.AnnotationPrefix)
	ingGroupBuilder := ingress.NewGroupBuilder(mgr.GetCache(), annotationParser, config.IngressClass)
	modelBuilder := build.NewBuilder(cloud, mgr.GetCache(), annotationParser, config)
	modelDeployer := deploy.NewDeployer(cloud, ebRepo)

	reconciler := newReconciler(mgr, cloud, config, ingGroupBuilder, modelBuilder, modelDeployer)
	c, err := controller.New("alb-ingress-controller", mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return err
	}

	ingressChan := make(chan event.GenericEvent)
	serviceChan := make(chan event.GenericEvent)
	if err := watchClusterEvents(c, mgr.GetCache(), config.IngressClass, ingGroupBuilder, ingressChan, serviceChan); err != nil {
		return fmt.Errorf("failed to watch cluster events due to %v", err)
	}

	return nil
}

func watchClusterEvents(c controller.Controller, cache cache.Cache, ingressClass string, ingGroupBuilder ingress.GroupBuilder,
	ingressChan <-chan event.GenericEvent, serviceChan <-chan event.GenericEvent) error {
	ingEventHandler := eventhandlers.NewEnqueueRequestsForIngressEvent(ingGroupBuilder, ingressClass)
	nodeEventHandler := eventhandlers.NewEnqueueRequestsForNodeEvent(ingGroupBuilder, ingressClass, cache)
	if err := c.Watch(&source.Kind{Type: &extensions.Ingress{}}, ingEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Channel{Source: ingressChan}, ingEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Node{}}, nodeEventHandler); err != nil {
		return err
	}
	return nil
}
