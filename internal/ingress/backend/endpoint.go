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

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	api "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	labelNodeRoleMaster               = "node-role.kubernetes.io/master"
	labelNodeRoleExcludeBalancer      = "node.kubernetes.io/exclude-from-external-load-balancers"
	labelAlphaNodeRoleExcludeBalancer = "alpha.service-controller.kubernetes.io/exclude-balancer"
	labelEKSComputeType               = "eks.amazonaws.com/compute-type"
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

				podKey := ingress.Namespace + "/" + epAddr.TargetRef.Name
				pod, err := resolver.store.GetPod(podKey)
				if err != nil {
					continue
				}

				if i, ok := targetsMap[pod.Status.PodIP]; ok {
					result[i] = pod
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
		if !IsNodeSuitableAsTrafficProxy(node) {
			continue
		}
		instanceID, err := resolver.store.GetNodeInstanceID(node)
		if err != nil {
			return nil, err
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

	conditionType := api.PodConditionType(fmt.Sprintf("target-health.alb.ingress.k8s.aws/%s_%s_%s", ingress.Name, backend.ServiceName, backend.ServicePort.String()))

	var result []*elbv2.TargetDescription
	for _, epSubset := range eps.Subsets {
		for _, epPort := range epSubset.Ports {
			// servicePort.Name is optional if there is only one port
			if servicePort.Name != "" && servicePort.Name != epPort.Name {
				continue
			}

			addresses := epSubset.Addresses

			// we need to loop over all unready pods to check if the ALB readiness gate is the only condition preventing the pod from being ready;
			// if this is the case, we return the pod as a desired target although its not in `Addresses`
			for _, epAddr := range epSubset.NotReadyAddresses {
				if epAddr.TargetRef == nil || epAddr.TargetRef.Kind != "Pod" {
					continue
				}

				podKey := ingress.Namespace + "/" + epAddr.TargetRef.Name
				pod, err := resolver.store.GetPod(podKey)
				if err != nil {
					continue
				}

				// check if pod has a readiness gate for this ingress and backend
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

				// check if all containers are ready
				for _, condition := range pod.Status.Conditions {
					if condition.Type == api.ContainersReady {
						if condition.Status == api.ConditionTrue {
							addresses = append(addresses, epAddr)
						}
						break
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

// IsNodeSuitableAsTrafficProxy check whether node is suitable as a traffic proxy.
// mimic the logic of serviceController: https://github.com/kubernetes/kubernetes/blob/b6b494b4484b51df8dc6b692fab234573da30ab4/pkg/controller/service/controller.go#L605
func IsNodeSuitableAsTrafficProxy(node *corev1.Node) bool {
	if node.Spec.Unschedulable {
		return false
	}
	if s, ok := node.ObjectMeta.Labels[labelEKSComputeType]; ok && s == "fargate" {
		return false
	}
	for _, label := range []string{labelNodeRoleMaster, labelNodeRoleExcludeBalancer, labelAlphaNodeRoleExcludeBalancer} {
		if _, hasLabel := node.ObjectMeta.Labels[label]; hasLabel {
			return false
		}
	}
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
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
