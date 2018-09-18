/*
Copyright 2015 The Kubernetes Authors.

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

package controller

import (
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albingress"
)

func (c *ALBController) syncIngress(interface{}) error {
	c.syncRateLimiter.Accept()

	if c.syncQueue.IsShuttingDown() {
		return nil
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.metricCollector.IncReconcileCount()

	newIngresses := albingress.NewALBIngressesFromIngresses(&albingress.NewALBIngressesFromIngressesOptions{
		Recorder:     c.recorder,
		Store:        c.store,
		ALBIngresses: c.runningConfig.Ingresses,
		Metric:       c.metricCollector,
	})

	// Update the prometheus gauge
	c.metricCollector.SetManagedIngresses(newIngresses.IngressesByNamespace())

	// Sync the state, resulting in creation, modify, delete, or no action, for every ALBIngress
	// instance known to the ALBIngress controller.
	removedIngresses := c.runningConfig.Ingresses.RemovedIngresses(newIngresses)

	// Update the list of ALBIngresses known to the ALBIngress controller to the newly generated list.
	c.runningConfig.Ingresses = newIngresses

	// Reconcile the states
	removedIngresses.Reconcile(c.metricCollector, c.sgAssociationController)
	for _, i := range removedIngresses {
		c.metricCollector.RemoveMetrics(i.ID())
	}
	c.runningConfig.Ingresses.Reconcile(c.metricCollector, c.sgAssociationController)

	// TODO check for per-namespace errors and increment prometheus metric

	return nil
}
