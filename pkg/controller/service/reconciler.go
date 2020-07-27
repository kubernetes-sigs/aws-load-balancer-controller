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
limitations under the License}.
*/

package service

import (
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/algorithm"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build/nlb"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const LoadBalancerFinalizer = "service.k8s.aws/load-balancer-finalizer"

func newReconciler(mgr manager.Manager, cloud cloud.Cloud, ebRepo backend.EndpointBindingRepo, cache cache.Cache,
	annotationParser k8s.AnnotationParser, deployer deploy.Deployer) reconcile.Reconciler {
	return &ReconcileService{
		client:           mgr.GetClient(),
		deployer:         deployer,
		cloud:            cloud,
		cache:            cache,
		annotationParser: annotationParser,
	}
}

var _ reconcile.Reconciler = &ReconcileService{}

// ReconcileService reconciles a Service object
type ReconcileService struct {
	cloud            cloud.Cloud
	cache            cache.Cache
	client           client.Client
	deployer         deploy.Deployer
	annotationParser k8s.AnnotationParser
}

func (r *ReconcileService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()
	logging.FromContext(ctx).Info("start reconcile", "request", request)

	svc := &corev1.Service{}
	deleting := false
	if err := r.client.Get(ctx, request.NamespacedName, svc); err != nil {
		if k8serr.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if svc.DeletionTimestamp.IsZero() {
		if !algorithm.ContainsString(svc.Finalizers, LoadBalancerFinalizer) {
			svc.Finalizers = append(svc.Finalizers, LoadBalancerFinalizer)
			// TODO: Use the client.Patch method instead
			if err := r.client.Update(ctx, svc); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		deleting = true
	}
	builder := nlb.NewServiceBuilder(r.cloud, r.cache, svc, request.NamespacedName, r.annotationParser)
	model, err := builder.Build(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	payload, err := json.Marshal(model)
	if err != nil {
		return reconcile.Result{}, err
	}
	logging.FromContext(ctx).Info("successfully built model", "payload", string(payload))
	// Deploy

	if err := r.deployer.Deploy(ctx, &model); err != nil {
		return reconcile.Result{}, err
	}
	logging.FromContext(ctx).Info("successfully deployed model", "request", request)

	// Update Status
	if model.LoadBalancer != nil {
		r.updateServiceStatus(ctx, svc, model.LoadBalancer.Status.DNSName)
	}

	if deleting {
		if algorithm.ContainsString(svc.Finalizers, LoadBalancerFinalizer) {
			svc.Finalizers = algorithm.RemoveString(svc.Finalizers, LoadBalancerFinalizer)
			// TODO: Use the client.Patch instead
			if err := r.client.Update(ctx, svc); err != nil {
				return reconcile.Result{}, err
			}
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileService) updateServiceStatus(ctx context.Context, svc *corev1.Service, lbDNS string) error {
	if len(svc.Status.LoadBalancer.Ingress) != 1 || svc.Status.LoadBalancer.Ingress[0].IP != "" || svc.Status.LoadBalancer.Ingress[0].Hostname != lbDNS {
		svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
			{
				Hostname: lbDNS,
			},
		}
		if err := r.client.Status().Update(ctx, svc); err != nil {
			return errors.Wrapf(err, "failed to update service:%v", svc)
		}
		return r.client.Status().Update(ctx, svc)
	}
	return nil
}
