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
	"strings"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	api "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EndpointResolver resolves the endpoints for specific ingress backend
type EndpointResolver interface {
	Resolve(*extensions.Ingress, *extensions.IngressBackend, string) ([]*elbv2.TargetDescription, error)
	ReverseResolve(*extensions.Ingress, *extensions.IngressBackend, []*elbv2.TargetDescription) ([]*corev1.Pod, error)
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

func (resolver *endpointResolver) Resolve(ingress *extensions.Ingress, backend *extensions.IngressBackend, targetType string) ([]*elbv2.TargetDescription, error) {
	if targetType == elbv2.TargetTypeEnumInstance {
		return resolver.resolveInstance(ingress, backend)
	}
	return resolver.resolveIP(ingress, backend)
}

// For each item in the targets slice, returns the corresponding pod in the result slice at the same index. The result slice is exactly as long as the input slice.
func (resolver *endpointResolver) ReverseResolve(ingress *extensions.Ingress, backend *extensions.IngressBackend, targets []*elbv2.TargetDescription) ([]*corev1.Pod, error) {
	service, servicePort, err := findServiceAndPort(resolver.store, ingress.Namespace, backend.ServiceName, backend.ServicePort)
	if err != nil {
		return nil, err
	}
	serviceKey := ingress.Namespace + "/" + service.Name
	eps, err := resolver.store.GetServiceEndpoints(serviceKey)
	if err != nil {
		return nil, fmt.Errorf("Unable to find service endpoints for %s: %v", serviceKey, err.Error())
	}

	podMap := map[string]*corev1.Pod{}
	pods := resolver.store.GetServicePods(service.Spec.Selector)
	for _, pod := range pods {
		podMap[pod.Name] = pod
	}

	targetsMap := map[string]int{}
	for i, target := range targets {
		targetsMap[aws.StringValue(target.Id)] = i
	}

	result := make([]*corev1.Pod, len(targets))
	for _, epSubset := range eps.Subsets {
		for _, epPort := range epSubset.Ports {
			// servicePort.Name is optional if there is only one port
			if servicePort.Name != "" && servicePort.Name != epPort.Name {
				continue
			}
			for _, epAddr := range append(epSubset.Addresses, epSubset.NotReadyAddresses...) {
				if epAddr.TargetRef == nil || epAddr.TargetRef.Kind != "Pod" {
					continue
				}

				pod, ok := podMap[epAddr.TargetRef.Name]
				if !ok {
					continue
				}

				for i, target := range targets {
					if *target.Id == pod.Status.PodIP {
						result[i] = pod
					}
				}
			}
		}
	}
	return result, nil
}

func (resolver *endpointResolver) resolveInstance(ingress *extensions.Ingress, backend *extensions.IngressBackend) ([]*elbv2.TargetDescription, error) {
	service, servicePort, err := findServiceAndPort(resolver.store, ingress.Namespace, backend.ServiceName, backend.ServicePort)
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

func (resolver *endpointResolver) resolveIP(ingress *extensions.Ingress, backend *extensions.IngressBackend) ([]*elbv2.TargetDescription, error) {
	service, servicePort, err := findServiceAndPort(resolver.store, ingress.Namespace, backend.ServiceName, backend.ServicePort)
	if err != nil {
		return nil, err
	}
	serviceKey := ingress.Namespace + "/" + service.Name
	eps, err := resolver.store.GetServiceEndpoints(serviceKey)
	if err != nil {
		return nil, fmt.Errorf("Unable to find service endpoints for %s: %v", serviceKey, err.Error())
	}

	podMap := map[string]*corev1.Pod{}
	pods := resolver.store.GetServicePods(service.Spec.Selector)
	for _, pod := range pods {
		podMap[pod.Name] = pod
	}
	conditionTypePrefix := fmt.Sprintf("target-health.%s/", parser.AnnotationsPrefix)
	conditionType := api.PodConditionType(fmt.Sprintf("%s%s_%s_%s", conditionTypePrefix, ingress.Name, service.Name, backend.ServicePort.String()))

	var result []*elbv2.TargetDescription
	for _, epSubset := range eps.Subsets {
		for _, epPort := range epSubset.Ports {
			// servicePort.Name is optional if there is only one port
			if servicePort.Name != "" && servicePort.Name != epPort.Name {
				continue
			}
			addresses := epSubset.Addresses
			if service.Spec.PublishNotReadyAddresses {
				addresses = append(addresses, epSubset.NotReadyAddresses...)
			} else {
				// if `PublishNotReadyAddresses` is not set, we need to loop over all unready pods to check if the ALB readiness gate is the only condition preventing the pod from being ready;
				// if this is the case, we return the pod as a desired target although its not in `Addresses`
				for _, epAddr := range epSubset.NotReadyAddresses {
					pod, ok := podMap[epAddr.TargetRef.Name]
					if !ok {
						continue
					}

					// check if pod has a readiness gate for this ingress and service
					found := false
					for _, gate := range pod.Spec.ReadinessGates {
						if gate.ConditionType == conditionType {
							found = true
							break
						}
					}
					if !found {
						continue
					}

					// check if all other conditions are fulfilled
					allConditionsFulfilled := true
					for _, condition := range pod.Status.Conditions {
						// TODO: consider:
						// * should we check if there are conditions for all readiness gates in the spec?
						//   (maybe other controllers didn't add their condition yet)
						// * should we care at all if other readiness gates are unfulfilled?

						if condition.Type == api.PodReady || strings.HasPrefix(string(condition.Type), conditionTypePrefix) {
							// we don't look at conditions of other ingresses/services
							continue
						}
						if condition.Status != "True" {
							allConditionsFulfilled = false
							break
						}
					}
					if allConditionsFulfilled {
						addresses = append(addresses, epAddr)
					}
				}
			}
			for _, epAddr := range addresses {
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
