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
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"

	api_v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestResolveWithModeInstance(t *testing.T) {
	var (
		nodeName1 = "node1"
		nodeName2 = "node2"
		nodeName3 = "node3"
	)
	const nodePort = 8888

	for _, tc := range []struct {
		ingress         *extensions.Ingress
		service         *api_v1.Service
		nodes           []*api_v1.Node
		nodeHealthProbe func(string) (bool, error)
		expectedTargets albelbv2.TargetDescriptions
		expectedError   bool
	}{
		{
			// success scenario by numeric service port
			ingress: &extensions.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "ingress",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					},
				},
			},
			service: &api_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "service",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: api_v1.ServiceSpec{
					Type: api_v1.ServiceTypeNodePort,
					Ports: []api_v1.ServicePort{
						{
							Port:     8080,
							NodePort: nodePort,
						},
					},
				},
			},
			nodes: []*api_v1.Node{
				{
					Spec: api_v1.NodeSpec{
						ProviderID: nodeName1,
					},
				},
				{
					Spec: api_v1.NodeSpec{
						ProviderID: nodeName2,
					},
				},
				{
					Spec: api_v1.NodeSpec{
						ProviderID: nodeName3,
					},
				},
			},
			nodeHealthProbe: func(instanceID string) (bool, error) { return instanceID != nodeName2, nil },
			expectedTargets: albelbv2.TargetDescriptions{
				{
					Id:   &nodeName1,
					Port: aws.Int64(nodePort),
				},
				{
					Id:   &nodeName3,
					Port: aws.Int64(nodePort),
				},
			},
			expectedError: false,
		},
		{
			// success scenario by string service port
			ingress: &extensions.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "ingress",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromString("http"),
					},
				},
			},
			service: &api_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "service",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: api_v1.ServiceSpec{
					Type: api_v1.ServiceTypeNodePort,
					Ports: []api_v1.ServicePort{
						{
							Name:     "http",
							NodePort: nodePort,
						},
					},
				},
			},
			nodes: []*api_v1.Node{
				{
					Spec: api_v1.NodeSpec{
						ProviderID: nodeName1,
					},
				},
				{
					Spec: api_v1.NodeSpec{
						ProviderID: nodeName2,
					},
				},
				{
					Spec: api_v1.NodeSpec{
						ProviderID: nodeName3,
					},
				},
			},
			nodeHealthProbe: func(instanceID string) (bool, error) { return instanceID != nodeName2, nil },
			expectedTargets: albelbv2.TargetDescriptions{
				{
					Id:   &nodeName1,
					Port: aws.Int64(nodePort),
				},
				{
					Id:   &nodeName3,
					Port: aws.Int64(nodePort),
				},
			},
			expectedError: false,
		},
		{
			//failure scenario by service not found
			ingress: &extensions.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "ingress",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromString("http"),
					},
				},
			},
			service:         nil,
			nodes:           []*api_v1.Node{},
			expectedTargets: nil,
			expectedError:   true,
		},
		{
			// failure scenario by service port not found
			ingress: &extensions.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "ingress",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromString("http"),
					},
				},
			},
			service: &api_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "service",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: api_v1.ServiceSpec{
					Type: api_v1.ServiceTypeNodePort,
					Ports: []api_v1.ServicePort{
						{
							Name:     "https",
							NodePort: nodePort,
						},
					},
				},
			},
			nodes:           []*api_v1.Node{},
			expectedTargets: nil,
			expectedError:   true,
		},
		{
			// failure scenario by service type isn't nodePort
			ingress: &extensions.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "ingress",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromString("http"),
					},
				},
			},
			service: &api_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "service",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: api_v1.ServiceSpec{
					Type: api_v1.ServiceTypeClusterIP,
					Ports: []api_v1.ServicePort{
						{
							Name:     "http",
							NodePort: nodePort,
						},
					},
				},
			},
			nodes:           []*api_v1.Node{},
			expectedTargets: nil,
			expectedError:   true,
		},
		{
			// failure scenario by failed nodeHealthCheck
			ingress: &extensions.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "ingress",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					},
				},
			},
			service: &api_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "service",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: api_v1.ServiceSpec{
					Type: api_v1.ServiceTypeNodePort,
					Ports: []api_v1.ServicePort{
						{
							Port:     8080,
							NodePort: nodePort,
						},
					},
				},
			},
			nodes: []*api_v1.Node{
				{
					Spec: api_v1.NodeSpec{
						ProviderID: nodeName1,
					},
				},
			},
			nodeHealthProbe: func(instanceID string) (bool, error) { return false, fmt.Errorf("dummy") },
			expectedTargets: nil,
			expectedError:   true,
		},
	} {
		store := store.NewDummy()
		store.GetServiceFunc = func(string) (*api_v1.Service, error) {
			if tc.service != nil {
				return tc.service, nil
			}
			return nil, fmt.Errorf("No such service")
		}
		store.ListNodesFunc = func() []*api_v1.Node {
			return tc.nodes
		}
		store.GetNodeInstanceIDFunc = func(node *api_v1.Node) (string, error) {
			return node.Spec.ProviderID, nil
		}

		resolver := newEndpointResolverWithNodeHealthProbe(
			store, "instance", tc.nodeHealthProbe)
		targets, err := resolver.Resolve(tc.ingress, tc.ingress.Spec.Backend)
		if !reflect.DeepEqual(tc.expectedTargets, targets) {
			t.Errorf("expected targets: %#v, actual targets:%#v", tc.expectedTargets, targets)
		}
		if (err != nil) != tc.expectedError {
			t.Errorf("expected error:%v, actual err:%v", tc.expectedError, err)
		}
	}
}

func TestResolveWithModeIP(t *testing.T) {
	var (
		ip1 = "192.168.1.1"
		ip2 = "192.168.1.2"
		ip3 = "192.168.1.3"
	)
	const (
		portHTTP  = 8080
		portHTTPS = 8443
	)

	for _, tc := range []struct {
		ingress         *extensions.Ingress
		service         *api_v1.Service
		endpoints       *api_v1.Endpoints
		expectedTargets albelbv2.TargetDescriptions
		expectedError   bool
	}{
		{
			// success scenario by numeric service port and numeric pod port
			ingress: &extensions.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "ingress",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					},
				},
			},
			service: &api_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "service",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: api_v1.ServiceSpec{
					Type: api_v1.ServiceTypeClusterIP,
					Ports: []api_v1.ServicePort{
						{
							Port: portHTTP,
						},
					},
				},
			},
			endpoints: &api_v1.Endpoints{
				Subsets: []api_v1.EndpointSubset{
					{
						Addresses: []api_v1.EndpointAddress{
							{
								IP: ip1,
							},
							{
								IP: ip2,
							},
						},
						Ports: []api_v1.EndpointPort{
							{
								Port: portHTTP,
							},
						},
					},
					{
						Addresses: []api_v1.EndpointAddress{
							{
								IP: ip3,
							},
						},
						Ports: []api_v1.EndpointPort{
							{
								Port: portHTTP,
							},
						},
					},
				},
			},
			expectedTargets: albelbv2.TargetDescriptions{
				{
					Id:   aws.String(ip1),
					Port: aws.Int64(portHTTP),
				},
				{
					Id:   aws.String(ip2),
					Port: aws.Int64(portHTTP),
				},
				{
					Id:   aws.String(ip3),
					Port: aws.Int64(portHTTP),
				},
			},
			expectedError: false,
		},
		{
			// success scenario by string service port and string pod port
			ingress: &extensions.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "ingress",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromString("https"),
					},
				},
			},
			service: &api_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "service",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: api_v1.ServiceSpec{
					Type: api_v1.ServiceTypeClusterIP,
					Ports: []api_v1.ServicePort{
						{
							Name: "http",
						},
						{
							Name: "https",
						},
					},
				},
			},
			endpoints: &api_v1.Endpoints{
				Subsets: []api_v1.EndpointSubset{
					{
						Addresses: []api_v1.EndpointAddress{
							{
								IP: ip1,
							},
							{
								IP: ip2,
							},
						},
						Ports: []api_v1.EndpointPort{
							{
								Name: "http",
								Port: portHTTP,
							},
							{
								Name: "https",
								Port: portHTTPS,
							},
						},
					},
					{
						Addresses: []api_v1.EndpointAddress{
							{
								IP: ip3,
							},
						},
						Ports: []api_v1.EndpointPort{
							{
								Name: "http",
								Port: portHTTP,
							},
							{
								Name: "https",
								Port: portHTTPS,
							},
						},
					},
				},
			},
			expectedTargets: albelbv2.TargetDescriptions{
				{
					Id:   aws.String(ip1),
					Port: aws.Int64(portHTTPS),
				},
				{
					Id:   aws.String(ip2),
					Port: aws.Int64(portHTTPS),
				},
				{
					Id:   aws.String(ip3),
					Port: aws.Int64(portHTTPS),
				},
			},
			expectedError: false,
		},
		{
			// failure scenario by no endpoint found
			ingress: &extensions.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "ingress",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					},
				},
			},
			service: &api_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "service",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: api_v1.ServiceSpec{
					Type: api_v1.ServiceTypeClusterIP,
					Ports: []api_v1.ServicePort{
						{
							Port: portHTTP,
						},
					},
				},
			},
			endpoints:       nil,
			expectedTargets: nil,
			expectedError:   true,
		},
		{
			// failure scenario by no service found
			ingress: &extensions.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "ingress",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					},
				},
			},
			service:         nil,
			endpoints:       nil,
			expectedTargets: nil,
			expectedError:   true,
		},
	} {
		store := store.NewDummy()
		store.GetServiceFunc = func(string) (*api_v1.Service, error) {
			if tc.service != nil {
				return tc.service, nil
			}
			return nil, fmt.Errorf("No such service")
		}
		store.GetServiceEndpointsFunc = func(string) (*api_v1.Endpoints, error) {
			if tc.endpoints != nil {
				return tc.endpoints, nil
			}
			return nil, fmt.Errorf("No such endpoints")
		}

		resolver := NewEndpointResolver(store, "ip")
		targets, err := resolver.Resolve(tc.ingress, tc.ingress.Spec.Backend)
		if !reflect.DeepEqual(tc.expectedTargets, targets) {
			t.Errorf("expected targets: %#v, actual targets:%#v", tc.expectedTargets, targets)
		}
		if (err != nil) != tc.expectedError {
			t.Errorf("expected error:%v, actual err:%v", tc.expectedError, err)
		}
	}
}

// newEndpointResolverWithNodeHealthProbe constructs a new EndpointResolver with specified nodeHealthProbe
// so that test cases can mock the default nodeHealthProbe :D
func newEndpointResolverWithNodeHealthProbe(store store.Storer, targetType string,
	nodeHealthProbe func(string) (bool, error)) EndpointResolver {
	if targetType == elbv2.TargetTypeEnumInstance {
		return &endpointResolverModeInstance{
			store,
			nodeHealthProbe,
		}
	}
	return &endpointResolverModeIP{store}
}
