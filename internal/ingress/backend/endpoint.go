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

package backend

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EndpointResolver resolves the endpoints for specific ingress backend
type EndpointResolver interface {
	Resolve(metav1.Object, *extensions.IngressBackend, string) ([]*elbv2.TargetDescription, error)
}

// NewEndpointResolver constructs a new EndpointResolver
func NewEndpointResolver(store store.Storer, cloud aws.CloudAPI) EndpointResolver {
	return &endpointResolver{
		cloud: cloud,
		store: store,
	}
}

type endpointResolver struct {
	cloud aws.CloudAPI
	store store.Storer
}

func (resolver *endpointResolver) Resolve(meta metav1.Object, backend *extensions.IngressBackend, targetType string) ([]*elbv2.TargetDescription, error) {
	switch targetType {
	case elbv2.TargetTypeEnumInstance:
		return resolver.resolveInstance(meta, backend)
	case elbv2.TargetTypeEnumIp:
		return resolver.resolveIP(meta, backend)
	default:
		return nil, fmt.Errorf("unrecognized targetType: %s", targetType)
	}
}

func (resolver *endpointResolver) resolveInstance(meta metav1.Object, backend *extensions.IngressBackend) ([]*elbv2.TargetDescription, error) {
	service, servicePort, err := findServiceAndPort(resolver.store, meta.GetNamespace(), backend.ServiceName, backend.ServicePort)
	if err != nil {
		return nil, err
	}
	if service.Spec.Type != corev1.ServiceTypeNodePort && service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return nil, fmt.Errorf("%v service is not of type NodePort or LoadBalancer and target-type is instance", service.Name)
	}
	nodePort := servicePort.NodePort

	var result []*elbv2.TargetDescription
	for _, node := range resolver.store.ListNodes() {
		instanceID, err := resolver.store.GetNodeInstanceID(node)
		if err != nil {
			return nil, err
		} else if healthy, err := resolver.cloud.IsNodeHealthy(instanceID); err != nil {
			return nil, err
		} else if !healthy {
			continue
		}
		result = append(result, &elbv2.TargetDescription{
			Id:   aws.String(instanceID),
			Port: aws.Int64(int64(nodePort)),
		})
	}
	return result, nil
}

// TODO: this does not make sense (relationship between service & ingress namespace?
// we do not need these functions to resolve service port
func (resolver *endpointResolver) resolveIP(meta metav1.Object, backend *extensions.IngressBackend) ([]*elbv2.TargetDescription, error) {
	service, servicePort, err := findServiceAndPort(resolver.store, meta.GetNamespace(), backend.ServiceName, backend.ServicePort)
	if err != nil {
		return nil, err
	}
	serviceKey := meta.GetNamespace() + "/" + service.Name
	eps, err := resolver.store.GetServiceEndpoints(serviceKey)
	if err != nil {
		return nil, fmt.Errorf("Unable to find service endpoints for %s: %v", serviceKey, err.Error())
	}

	var result []*elbv2.TargetDescription
	for _, epSubset := range eps.Subsets {
		for _, epPort := range epSubset.Ports {
			// servicePort.Name is optional if there is only one port
			if servicePort.Name != "" && servicePort.Name != epPort.Name {
				continue
			}
			for _, epAddr := range epSubset.Addresses {
				result = append(result, &elbv2.TargetDescription{
					Id:   aws.String(epAddr.IP),
					Port: aws.Int64(int64(epPort.Port)),
				})
			}
		}
	}

	return result, nil
}

// findServiceAndPort returns the service & servicePort by name
func findServiceAndPort(store store.Storer, namespace string, serviceName string, servicePort intstr.IntOrString) (*corev1.Service, *corev1.ServicePort, error) {
	serviceKey := namespace + "/" + serviceName
	service, err := store.GetService(serviceKey)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to find the %s service: %s", serviceKey, err.Error())
	}

	resolvedServicePort, err := k8s.LookupServicePort(service, servicePort)

	return service, resolvedServicePort, err
}
