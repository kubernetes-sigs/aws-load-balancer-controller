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

package eventhandlers

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const ServiceAnnotationLoadBalancerType = "service.beta.kubernetes.io/aws-load-balancer-type"

func isNLBIPMode(annotations map[string]string) bool {
	if annotations[ServiceAnnotationLoadBalancerType] == "nlb-ip" {
		return true
	}
	return false
}

var logger = log.Log.WithName("eventhandlers").WithName("service")

func NewEnqueueRequestForServiceEvent(k8sCache cache.Cache) handler.EventHandler {
	return &enqueueRequestsForServiceEvent{
		k8sCache: k8sCache,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForServiceEvent)(nil)

type enqueueRequestsForServiceEvent struct {
	k8sCache cache.Cache
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *enqueueRequestsForServiceEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	logger.Info("Create Event")
	h.enqueueImpactedService(e.Object.(*corev1.Service), queue)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *enqueueRequestsForServiceEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	logger.Info("Update Event")
	old := e.ObjectOld.(*corev1.Service)
	new := e.ObjectNew.(*corev1.Service)
	if !reflect.DeepEqual(old, new) {
		h.enqueueImpactedService(new, queue)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *enqueueRequestsForServiceEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	logger.Info("Delete Event")
	//h.enqueueImpactedService(e.Object.(*corev1.Service), queue)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *enqueueRequestsForServiceEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {
	logger.Info("Generic Event")
}

func (h *enqueueRequestsForServiceEvent) enqueueImpactedService(service *corev1.Service, queue workqueue.RateLimitingInterface) {
	// Check if the svc needs to be handled
	if !isNLBIPMode(service.Annotations) {
		return
	}
	logger.Info("Adding service to reconcile queue", "SVC Name", service.Name)
	queue.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: service.Namespace,
			Name:      service.Name,
		},
	})
}
