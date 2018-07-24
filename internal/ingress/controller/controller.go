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
	"time"

	clientset "k8s.io/client-go/kubernetes"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albingress"
)

// Configuration contains all the settings required by an Ingress controller
type Configuration struct {
	APIServerHost  string
	KubeConfigFile string
	Client         clientset.Interface

	HealthCheckPeriod time.Duration
	ResyncPeriod      time.Duration

	ConfigMapName string

	Namespace string

	DefaultHealthzURL     string
	DefaultSSLCertificate string

	ElectionID string

	HealthzPort int

	ClusterName             string
	ALBNamePrefix           string
	RestrictScheme          bool
	RestrictSchemeNamespace string
	AWSSyncPeriod           time.Duration
	AWSAPIMaxRetries        int
	AWSAPIDebug             bool

	EnableProfiling bool

	SyncRateLimit float32
}

func (c *ALBController) syncIngress(interface{}) error {
	c.syncRateLimiter.Accept()

	if c.syncQueue.IsShuttingDown() {
		return nil
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.metricCollector.IncReconcileCount()

	newIngresses := albingress.NewALBIngressesFromIngresses(&albingress.NewALBIngressesFromIngressesOptions{
		Recorder:      c.recorder,
		ClusterName:   c.cfg.ClusterName,
		ALBNamePrefix: c.cfg.ALBNamePrefix,
		Store:         c.store,
		ALBIngresses:  c.runningConfig.Ingresses,
	})

	// Update the prometheus gauge
	ingressesByNamespace := map[string]int{}
	for _, ingress := range newIngresses {
		ingressesByNamespace[ingress.Namespace()]++
	}

	for ns, count := range ingressesByNamespace {
		c.metricCollector.SetManagedIngresses(ns, float64(count))
	}

	// Sync the state, resulting in creation, modify, delete, or no action, for every ALBIngress
	// instance known to the ALBIngress controller.
	removedIngresses := c.runningConfig.Ingresses.RemovedIngresses(newIngresses)

	// Update the list of ALBIngresses known to the ALBIngress controller to the newly generated list.
	c.runningConfig.Ingresses = newIngresses

	// // Reconcile the states
	removedIngresses.Reconcile()
	c.runningConfig.Ingresses.Reconcile()

	// TODO check for per-namespace errors and increment prometheus metric

	return nil
}
