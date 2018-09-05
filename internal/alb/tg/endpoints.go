/*
Copyright 2018 The Kubernetes Authors.
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

package tg

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// GetIngressBackendTargets returns the targets for specific ingressBackend.
func GetIngressBackendTargets(storer store.Storer, namespace string, backend *extensions.IngressBackend, targetType string) (albelbv2.TargetDescriptions, error) {
	service, servicePort, err := getIngressBackendServiceAndPort(storer, namespace, backend)
	if err != nil {
		return nil, fmt.Errorf("Unable to find the service or servicePort: %s", err.Error())
	}
	if targetType == elbv2.TargetTypeEnumInstance {
		return getTargetsWithModeInstance(storer, namespace, service, servicePort)
	}
	return getTargetsWithModeIP(storer, namespace, service, servicePort)
}

// getTargetsWithModeInstance returns a list of targets for specific service:servicePort under "instance" targetMode
func getTargetsWithModeInstance(storer store.Storer, namespace string, service *corev1.Service, servicePort *corev1.ServicePort) (albelbv2.TargetDescriptions, error) {
	if service.Spec.Type != corev1.ServiceTypeNodePort {
		return nil, fmt.Errorf("%v service is not of type NodePort and target-type is instance", service.Name)
	}
	nodePort := servicePort.NodePort

	var result albelbv2.TargetDescriptions
	for _, node := range storer.ListNodes() {
		instanceID, err := storer.GetNodeInstanceID(node)
		if err != nil {
			return nil, err
		} else if b, err := albec2.EC2svc.IsNodeHealthy(instanceID); err != nil {
			return nil, err
		} else if b != true {
			continue
		}
		result = append(result, &elbv2.TargetDescription{
			Id:   aws.String(instanceID),
			Port: aws.Int64(int64(nodePort)),
		})
	}
	return result.Sorted(), nil
}

// getTargetsWithModeIP returns a list of targets for specific service:servicePort under "instance" targetMode
func getTargetsWithModeIP(storer store.Storer, namespace string, service *corev1.Service, servicePort *corev1.ServicePort) (albelbv2.TargetDescriptions, error) {
	eps, err := storer.GetServiceEndpoints(namespace + "/" + service.Name)
	if err != nil {
		return nil, fmt.Errorf("Unable to find service endpoints for %s/%s: %v", namespace, service.Name, err.Error())
	}

	var result albelbv2.TargetDescriptions
	for _, subset := range eps.Subsets {
		for _, epPort := range subset.Ports {
			// servicePort.Name is optional if there is only one port
			if servicePort.Name != "" && servicePort.Name != epPort.Name {
				continue
			}
			for _, epAddr := range subset.Addresses {
				result = append(result, &elbv2.TargetDescription{
					Id:   aws.String(epAddr.IP),
					Port: aws.Int64(int64(epPort.Port)),
				})
			}
		}
	}
	return result.Sorted(), nil
}

// getIngressBackendServiceAndPort returns the associated service and servicePort for specific ingressBackend
func getIngressBackendServiceAndPort(storer store.Storer, namespace string, backend *extensions.IngressBackend) (*corev1.Service, *corev1.ServicePort, error) {
	// Verify the service (namespace/serviceName) exists in kubernetes.
	serviceKey := namespace + "/" + backend.ServiceName
	service, err := storer.GetService(serviceKey)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to find the %v service: %s", serviceKey, err.Error())
	}

	if backend.ServicePort.Type == intstr.String {
		for _, svcPort := range service.Spec.Ports {
			if svcPort.Name == backend.ServicePort.StrVal {
				return service, &svcPort, nil
			}
		}
	} else {
		for _, svcPort := range service.Spec.Ports {
			if svcPort.Port == backend.ServicePort.IntVal {
				return service, &svcPort, nil
			}
		}
	}

	return service, nil, fmt.Errorf("no port is mapped for service %s and port name %s", service.Name, backend.ServicePort.String())
}
