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

package collectors

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	operation = []string{"namespace", "class"}
)

// Controller defines base metrics about the ingress controller
type Controller struct {
	prometheus.Collector

	reconcileOperation       *prometheus.CounterVec
	reconcileOperationErrors *prometheus.CounterVec
	managedIngresses         *prometheus.GaugeVec

	labels prometheus.Labels
}

// NewController creates a new prometheus collector for the
// Ingress controller operations
func NewController(pod, namespace, class string) *Controller {
	cm := &Controller{
		labels: prometheus.Labels{
			"namespace": namespace,
			"class":     class,
		},

		reconcileOperation: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: PrometheusNamespace,
				Name:      "success",
				Help:      `Cumulative number of Ingress controller reconcile operations`,
			},
			operation,
		),
		reconcileOperationErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: PrometheusNamespace,
				Name:      "errors",
				Help:      `Cumulative number of Ingress controller errors during reconcile operations`,
			},
			operation,
		),
		managedIngresses: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: PrometheusNamespace,
				Name:      "managed_ingresses",
				Help:      `Total number of ingresses managed by the controller`,
			},
			operation,
		),
	}

	return cm
}

// IncReconcileCount increment the reconcile counter
func (cm *Controller) IncReconcileCount() {
	cm.reconcileOperation.With(cm.labels).Inc()
}

// IncReconcileErrorCount increment the reconcile error counter
func (cm *Controller) IncReconcileErrorCount() {
	cm.reconcileOperationErrors.With(cm.labels).Inc()
}

// SetManagedIngresses sets the number of managed ingresses
func (cm *Controller) SetManagedIngresses(namespace string, cnt float64) {
	l := cm.labels
	l["namespace"] = namespace
	cm.managedIngresses.With(l).Set(cnt)
}

// Describe implements prometheus.Collector
func (cm Controller) Describe(ch chan<- *prometheus.Desc) {
	cm.reconcileOperation.Describe(ch)
	cm.reconcileOperationErrors.Describe(ch)
	cm.managedIngresses.Describe(ch)
}

// Collect implements the prometheus.Collector interface.
func (cm Controller) Collect(ch chan<- prometheus.Metric) {
	cm.reconcileOperation.Collect(ch)
	cm.reconcileOperationErrors.Collect(ch)
	cm.managedIngresses.Collect(ch)
}

func deleteConstants(labels prometheus.Labels) {
	delete(labels, "controller_namespace")
	delete(labels, "controller_class")
	delete(labels, "controller_pod")
}
