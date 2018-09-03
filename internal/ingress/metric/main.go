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
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/class"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric/collectors"
)

// Collector defines the interface for a metric collector
type Collector interface {
	IncReconcileCount()
	IncReconcileErrorCount(string)
	SetManagedIngresses(map[string]int)

	IncAPIRequestCount(prometheus.Labels)
	ObserveAPIRequest(prometheus.Labels, time.Time)
	IncAPIErrorCount(prometheus.Labels)
	IncAPIRetryCount(prometheus.Labels)

	RemoveMetrics(string)

	Start()
	Stop()
}

type collector struct {
	ingressController *collectors.Controller
	awsAPIController  *collectors.AWSAPIController

	registry *prometheus.Registry
}

// NewCollector creates a new metric collector the for ingress controller
func NewCollector(registry *prometheus.Registry) (Collector, error) {
	ic := collectors.NewController(class.IngressClass)
	ac := collectors.NewAWSAPIController()

	return Collector(&collector{
		ingressController: ic,
		awsAPIController:  ac,
		registry:          registry,
	}), nil
}

func (c *collector) IncReconcileCount() {
	c.ingressController.IncReconcileCount()
}

func (c *collector) IncReconcileErrorCount(s string) {
	c.ingressController.IncReconcileErrorCount(s)
}

func (c *collector) SetManagedIngresses(i map[string]int) {
	c.ingressController.SetManagedIngresses(i, c.registry)
}

func (c *collector) IncAPIRequestCount(l prometheus.Labels) {
	c.awsAPIController.IncAPIRequestCount(l)
}

func (c *collector) ObserveAPIRequest(l prometheus.Labels, start time.Time) {
	c.awsAPIController.ObserveAPIRequest(l, start)
}

func (c *collector) IncAPIErrorCount(l prometheus.Labels) {
	c.awsAPIController.IncAPIErrorCount(l)
}

func (c *collector) IncAPIRetryCount(l prometheus.Labels) {
	c.awsAPIController.IncAPIRetryCount(l)
}

func (c *collector) RemoveMetrics(ingressName string) {
	c.ingressController.RemoveMetrics(ingressName)
}

func (c *collector) Start() {
	c.registry.MustRegister(c.ingressController)
	c.registry.MustRegister(c.awsAPIController)
}

func (c *collector) Stop() {
	c.registry.Unregister(c.ingressController)
	c.registry.Unregister(c.awsAPIController)
}
