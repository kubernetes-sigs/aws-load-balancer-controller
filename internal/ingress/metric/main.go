/*
Copyright 2017 The Kubernetes Authors.

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

package metric

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/class"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric/collectors"
)

// Collector defines the interface for a metric collector
type Collector interface {
	IncReconcileCount()
	IncReconcileErrorCount()

	Start()
	Stop()
}

type collector struct {
	ingressController *collectors.Controller

	registry *prometheus.Registry
}

// NewCollector creates a new metric collector the for ingress controller
func NewCollector(registry *prometheus.Registry) (Collector, error) {
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podNamespace == "" {
		podNamespace = "default"
	}

	podName := os.Getenv("POD_NAME")

	ic := collectors.NewController(podName, podNamespace, class.IngressClass)

	return Collector(&collector{
		ingressController: ic,
		registry:          registry,
	}), nil
}

func (c *collector) IncReconcileCount() {
	c.ingressController.IncReconcileCount()
}

func (c *collector) IncReconcileErrorCount() {
	c.ingressController.IncReconcileErrorCount()
}

func (c *collector) Start() {
	c.registry.MustRegister(c.ingressController)
}

func (c *collector) Stop() {
	c.registry.Unregister(c.ingressController)
}
