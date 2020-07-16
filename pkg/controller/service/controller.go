/*
Copyright 2020 The Kubernetes Authors.

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

package service

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/controller/service/eventhandlers"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func Initialize(mgr manager.Manager, cloud cloud.Cloud, ebRepo backend.EndpointBindingRepo, config ingress.Config) error {
	annotationParser := k8s.NewSuffixAnnotationParser(config.AnnotationPrefix)
	modelBuilder := build.NewBuilder(cloud, mgr.GetCache(), annotationParser, config)
	deployer := deploy.NewDeployer(cloud, ebRepo)

	reconciler := newReconciler(mgr, cloud, ebRepo, mgr.GetCache(), modelBuilder, deployer)
	c, err := controller.New("service-controller", mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return err
	}
	if err := watchClusterEvents(c, mgr.GetCache()); err != nil {
		return fmt.Errorf("failed to watch cluster events due to %v", err)
	}
	return nil
}

func watchClusterEvents(c controller.Controller, cache cache.Cache) error {
	svcEventHandler := eventhandlers.NewEnqueueRequestForServiceEvent(cache)
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, svcEventHandler); err != nil {
		return err
	}
	return nil
}
