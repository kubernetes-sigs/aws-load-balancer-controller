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
	"net"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EndpointResolver resolves the endpoints for specific ingress backend
type EndpointResolver interface {
	Resolve(ingress *extensions.Ingress, backend *extensions.IngressBackend) ([]*elbv2.TargetDescription, error)
}

// NewEndpointResolver constructs a new EndpointResolver
func NewEndpointResolver(store store.Storer, targetType string) EndpointResolver {
	if targetType == elbv2.TargetTypeEnumInstance {
		return &endpointResolverModeInstance{
			store,
			albec2.EC2svc.IsNodeHealthy,
		}
	}
	return &endpointResolverModeIP{store}
}

type endpointResolverModeInstance struct {
	store           store.Storer
	nodeHealthProbe func(instanceID string) (bool, error)
}

type endpointResolverModeIP struct {
	store store.Storer
}

func (resolver *endpointResolverModeInstance) Resolve(ingress *extensions.Ingress, backend *extensions.IngressBackend) ([]*elbv2.TargetDescription, error) {
	service, servicePort, err := findServiceAndPort(resolver.store, ingress.Namespace, backend.ServiceName, backend.ServicePort)
	if err != nil {
		return nil, err
	}
	if service.Spec.Type != corev1.ServiceTypeNodePort {
		return nil, fmt.Errorf("%v service is not of type NodePort and target-type is instance", service.Name)
	}
	nodePort := servicePort.NodePort

	var result []*elbv2.TargetDescription
	for _, node := range resolver.store.ListNodes() {
		instanceID, err := resolver.store.GetNodeInstanceID(node)
		if err != nil {
			return nil, err
		} else if b, err := resolver.nodeHealthProbe(instanceID); err != nil {
			return nil, err
		} else if b != true {
			continue
		}
		result = append(result, &elbv2.TargetDescription{
			Id:   aws.String(instanceID),
			Port: aws.Int64(int64(nodePort)),
		})
	}
	return result, nil
}

func (resolver *endpointResolverModeIP) Resolve(ingress *extensions.Ingress, backend *extensions.IngressBackend) ([]*elbv2.TargetDescription, error) {
	service, servicePort, err := findServiceAndPort(resolver.store, ingress.Namespace, backend.ServiceName, backend.ServicePort)
	if err != nil {
		return nil, err
	}
	serviceKey := ingress.Namespace + "/" + service.Name
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

	err = populateAZ(result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func populateAZ(a []*elbv2.TargetDescription) error {
	vpcID, err := albec2.EC2svc.GetVPCID()
	if err != nil {
		return err
	}

	vpc, err := albec2.EC2svc.GetVPC(vpcID)
	if err != nil {
		return err
	}

	// Parse all CIDR blocks associated with the VPC
	var ipv4Nets []*net.IPNet
	for _, cblock := range vpc.CidrBlockAssociationSet {
		_, parsed, err := net.ParseCIDR(*cblock.CidrBlock)
		if err != nil {
			return err
		}
		ipv4Nets = append(ipv4Nets, parsed)
	}

	// Check if endpoints are in any of the blocks. If not the IP is outside the VPC
	for i := range a {
		found := false
		aNet := net.ParseIP(*a[i].Id)
		for _, ipv4Net := range ipv4Nets {
			if ipv4Net.Contains(aNet) {
				found = true
				break
			}
		}
		if !found {
			a[i].AvailabilityZone = aws.String("all")
		}
	}
	return nil
}

// findServiceAndPort returns the service & servicePort by name
func findServiceAndPort(store store.Storer, namespace string, serviceName string, servicePort intstr.IntOrString) (*corev1.Service, *corev1.ServicePort, error) {
	serviceKey := namespace + "/" + serviceName
	service, err := store.GetService(serviceKey)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to find the %s service: %s", serviceKey, err.Error())
	}

	if servicePort.Type == intstr.String {
		for _, p := range service.Spec.Ports {
			if p.Name == servicePort.StrVal {
				return service, &p, nil
			}
		}
	} else {
		for _, p := range service.Spec.Ports {
			if p.Port == servicePort.IntVal {
				return service, &p, nil
			}
		}
	}

	return service, nil, fmt.Errorf("Unable to find the %s service with %s port", serviceKey, servicePort.String())
}
