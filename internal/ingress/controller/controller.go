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
	"fmt"
	"time"

	apiv1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albingress"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/class"
	albannotations "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/annotations"
)

const (
	rootLocation = "/"
)

// Configuration contains all the settings required by an Ingress controller
type Configuration struct {
	APIServerHost  string
	KubeConfigFile string
	Client         clientset.Interface

	ResyncPeriod time.Duration

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

// syncIngress collects all the pieces required to assemble the NGINX
// configuration file and passes the resulting data structures to the backend
// (OnUpdate) when a reload is deemed necessary.
func (c *ALBController) syncIngress(interface{}) error {
	c.syncRateLimiter.Accept()

	if c.syncQueue.IsShuttingDown() {
		return nil
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.metricCollector.IncReconcileCount()

	annotationFactory := albannotations.NewValidatingAnnotationFactory(&albannotations.NewValidatingAnnotationFactoryOptions{
		Validator:   albannotations.NewConcreteValidator(),
		ClusterName: &c.cfg.ClusterName,
	})

	newIngresses := albingress.NewALBIngressesFromIngresses(&albingress.NewALBIngressesFromIngressesOptions{
		Recorder:              c.recorder,
		ClusterName:           c.cfg.ClusterName,
		ALBNamePrefix:         c.cfg.ALBNamePrefix,
		Ingresses:             c.store.ListIngresses(),
		ALBIngresses:          c.runningConfig.Ingresses,
		IngressClass:          class.IngressClass,
		DefaultIngressClass:   class.DefaultClass,
		GetServiceNodePort:    c.GetServiceNodePort,
		GetServiceAnnotations: c.GetServiceAnnotations,
		TargetsFunc:           c.GetTargets,
		AnnotationFactory:     annotationFactory,
		Resources:             c.runningConfig.Resources,
	})

	// // Update the prometheus gauge
	// ingressesByNamespace := map[string]int{}
	// logger.Debugf("Ingress count: %d", len(newIngresses))
	// for _, ingress := range newIngresses {
	// 	ingressesByNamespace[ingress.Namespace()]++
	// }

	// for ns, count := range ingressesByNamespace {
	// 	albprom.ManagedIngresses.With(
	// 		prometheus.Labels{"namespace": ns}).Set(float64(count))
	// }

	// Sync the state, resulting in creation, modify, delete, or no action, for every ALBIngress
	// instance known to the ALBIngress controller.
	removedIngresses := c.runningConfig.Ingresses.RemovedIngresses(newIngresses)

	// Update the list of ALBIngresses known to the ALBIngress controller to the newly generated list.
	c.runningConfig.Ingresses = newIngresses

	// // Reconcile the states
	removedIngresses.Reconcile()
	c.runningConfig.Ingresses.Reconcile()

	// err := c.OnUpdate(*pcfg)
	// if err != nil {
	// 	c.metricCollector.IncReloadErrorCount()
	// 	// c.metricCollector.ConfigSuccess(hash, false)
	// 	glog.Errorf("Unexpected failure reloading the backend:\n%v", err)
	// 	return err
	// }

	// c.metricCollector.ConfigSuccess(hash, true)
	// ri := getRemovedIngresses(c.runningConfig, pcfg)
	// re := getRemovedHosts(c.runningConfig, pcfg)
	// c.metricCollector.RemoveMetrics(ri, re)

	return nil
}

// GetServiceNodePort returns the nodeport for a given Kubernetes service
func (c *ALBController) GetServiceNodePort(serviceKey, serviceType string, backendPort int32) (*int64, error) {
	// Verify the service (namespace/service-name) exists in Kubernetes.
	item, err := c.store.GetService(serviceKey)
	if err != nil {
		return nil, fmt.Errorf("Unable to find the %v service: %s", serviceKey, err.Error())
	}

	switch serviceType {
	case "instance":
		// Verify the service type is Node port.
		if item.Spec.Type != apiv1.ServiceTypeNodePort {
			return nil, fmt.Errorf("%v service is not of type NodePort", serviceKey)
		}
		// Return the node port for the desired service port.
		for _, p := range item.Spec.Ports {
			if p.Port == backendPort {
				return aws.Int64(int64(p.NodePort)), nil
			}
		}
	case "pod":
		// Return the target port for the desired service port
		for _, p := range item.Spec.Ports {
			if p.Port == backendPort {
				return aws.Int64(int64(p.TargetPort.IntVal)), nil
			}
		}
	}

	return nil, fmt.Errorf("Unable to find a port defined in the %v service", serviceKey)
}

// GetServiceAnnotations returns the parsed annotations for a given Kubernetes service
func (c *ALBController) GetServiceAnnotations(namespace, serviceName string) (*map[string]string, error) {
	serviceKey := fmt.Sprintf("%s/%s", namespace, serviceName)

	// Verify the service (namespace/service-name) exists in Kubernetes.
	item, err := c.store.GetService(serviceKey)
	if err != nil {
		return nil, fmt.Errorf("Unable to find the %v service: %s", serviceKey, err.Error())
	}

	return &item.Annotations, nil
}

// GetTargets returns a list of the cluster node external ids
func (c *ALBController) GetTargets(mode *string, namespace string, svc string, port *int64) albelbv2.TargetDescriptions {
	var result albelbv2.TargetDescriptions

	if *mode == "instance" {
		for _, node := range c.store.ListNodes() {
			result = append(result,
				&elbv2.TargetDescription{
					Id:   aws.String(node.Spec.DoNotUse_ExternalID), // Need to deal with this: https://github.com/kubernetes/kubernetes/pull/61877
					Port: port,
				})
		}
	}

	if *mode == "pod" {
		eps, err := c.store.GetServiceEndpoints(namespace + "/" + svc)
		if err != nil {
			glog.Errorf("Unable to find service endpoints for %s/%s", namespace, svc)
			return nil
		}

		for _, subset := range eps.Subsets {
			for _, addr := range subset.Addresses {
				for _, port := range subset.Ports {
					result = append(result, &elbv2.TargetDescription{
						Id:   aws.String(addr.IP),
						Port: aws.Int64(int64(port.Port)),
					})
				}
			}
		}
	}

	return result.Sorted()
}
