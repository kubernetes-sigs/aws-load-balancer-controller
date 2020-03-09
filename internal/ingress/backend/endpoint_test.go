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
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"

	api_v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		name            string
		ingress         *extensions.Ingress
		service         *api_v1.Service
		nodes           []*api_v1.Node
		nodeHealthProbe func(string) (bool, error)
		expectedTargets []*elbv2.TargetDescription
		expectedError   bool
	}{
		{
			name: "success scenario by numeric service port",
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
			expectedTargets: []*elbv2.TargetDescription{
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
			name: "success scenario by string service port",
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
			expectedTargets: []*elbv2.TargetDescription{
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
			name: "failure scenario by service not found",
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
			name: "failure scenario by service port not found",
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
			name: "failure scenario by service type isn't nodePort",
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
			name: "failure scenario by failed nodeHealthCheck",
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
		t.Run(tc.name, func(t *testing.T) {
			cloud := &mocks.CloudAPI{}
			for i := range tc.nodes {
				cloud.On("IsNodeHealthy", tc.nodes[i].Spec.ProviderID).Return(tc.nodeHealthProbe(tc.nodes[i].Spec.ProviderID))
			}

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

			//  tc.nodeHealthProbe

			resolver := NewEndpointResolver(store, cloud)
			targets, err := resolver.Resolve(tc.ingress, tc.ingress.Spec.Backend, elbv2.TargetTypeEnumInstance)
			if !reflect.DeepEqual(tc.expectedTargets, targets) {
				t.Errorf("expected targets: %#v, actual targets:%#v", tc.expectedTargets, targets)
			}
			if (err != nil) != tc.expectedError {
				t.Errorf("expected error:%v, actual err:%v", tc.expectedError, err)
			}
		})
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

	pods := []*api_v1.Pod{
		// pod not all containers ready
		{
			ObjectMeta: v1.ObjectMeta{
				Name:      "pod1",
				Namespace: api_v1.NamespaceDefault,
				Labels: map[string]string{
					"app": "my-app",
				},
			},
			Spec: api_v1.PodSpec{
				ReadinessGates: []api_v1.PodReadinessGate{
					{
						ConditionType: api_v1.PodConditionType("target-health.alb.ingress.k8s.aws/ingress_service_https"),
					},
				},
			},
			Status: api_v1.PodStatus{
				Conditions: []api_v1.PodCondition{
					{
						Type:   api_v1.ContainersReady,
						Status: api_v1.ConditionFalse,
					},
				},
			},
		},

		// pod with all containers ready
		{
			ObjectMeta: v1.ObjectMeta{
				Name:      "pod2",
				Namespace: api_v1.NamespaceDefault,
				Labels: map[string]string{
					"app": "my-app",
				},
			},
			Spec: api_v1.PodSpec{
				ReadinessGates: []api_v1.PodReadinessGate{
					{
						ConditionType: api_v1.PodConditionType("target-health.alb.ingress.k8s.aws/ingress_service_https"),
					},
				},
			},
			Status: api_v1.PodStatus{
				Conditions: []api_v1.PodCondition{
					{
						Type:   api_v1.ContainersReady,
						Status: api_v1.ConditionTrue,
					},
				},
			},
		},
	}

	for _, tc := range []struct {
		name            string
		ingress         *extensions.Ingress
		service         *api_v1.Service
		endpoints       *api_v1.Endpoints
		expectedTargets []*elbv2.TargetDescription
		expectedError   bool
	}{
		{
			name: "success scenario by numeric service port and numeric pod port",
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
			expectedTargets: []*elbv2.TargetDescription{
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
			name: "success scenario by string service port and string pod port",
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
			expectedTargets: []*elbv2.TargetDescription{
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
			name: "not ready addresses are ignored",
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
						},
						NotReadyAddresses: []api_v1.EndpointAddress{
							{
								IP: ip2,
								TargetRef: &api_v1.ObjectReference{
									Kind: "Pod",
									Name: "pod1",
								},
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
			expectedTargets: []*elbv2.TargetDescription{
				{
					Id:   aws.String(ip1),
					Port: aws.Int64(portHTTPS),
				},
			},
			expectedError: false,
		},
		{
			name: "not ready addresses are used if the ContainersReady condition is true",
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
						},
						NotReadyAddresses: []api_v1.EndpointAddress{
							{
								IP: ip2,
								TargetRef: &api_v1.ObjectReference{
									Kind: "Pod",
									Name: "pod2",
								},
							},
						},
						Ports: []api_v1.EndpointPort{
							{
								Name: "https",
								Port: portHTTPS,
							},
						},
					},
				},
			},
			expectedTargets: []*elbv2.TargetDescription{
				{
					Id:   aws.String(ip1),
					Port: aws.Int64(portHTTPS),
				},
				{
					Id:   aws.String(ip2),
					Port: aws.Int64(portHTTPS),
				},
			},
			expectedError: false,
		},
		{
			name: "failure scenario by no endpoint found",
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
			name: "failure scenario by no service found",
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
		t.Run(tc.name, func(t *testing.T) {
			cloud := &mocks.CloudAPI{}

			s := store.NewDummy()
			s.GetServiceFunc = func(string) (*api_v1.Service, error) {
				if tc.service != nil {
					return tc.service, nil
				}
				return nil, fmt.Errorf("No such service")
			}
			s.GetServiceEndpointsFunc = func(string) (*api_v1.Endpoints, error) {
				if tc.endpoints != nil {
					return tc.endpoints, nil
				}
				return nil, fmt.Errorf("No such endpoints")
			}
			s.GetPodFunc = func(key string) (*api_v1.Pod, error) {
				for _, pod := range pods {
					if fmt.Sprintf("%s/%s", pod.Namespace, pod.Name) == key {
						return pod, nil
					}
				}
				return nil, store.NotExistsError(key)
			}

			resolver := NewEndpointResolver(s, cloud)
			targets, err := resolver.Resolve(tc.ingress, tc.ingress.Spec.Backend, elbv2.TargetTypeEnumIp)
			if !reflect.DeepEqual(tc.expectedTargets, targets) {
				t.Errorf("expected targets: %#v, actual targets:%#v", tc.expectedTargets, targets)
			}
			if (err != nil) != tc.expectedError {
				t.Errorf("expected error:%v, actual err:%v", tc.expectedError, err)
			}
		})
	}
}

func TestReverseResolve(t *testing.T) {
	var (
		ip1 = "192.168.1.1"
		ip2 = "192.168.1.2"
		ip3 = "192.168.1.3"
	)
	const (
		portHTTP  = 8080
		portHTTPS = 8443
	)

	pods := []*api_v1.Pod{
		{
			ObjectMeta: v1.ObjectMeta{
				Name:      "pod1",
				Namespace: api_v1.NamespaceDefault,
				Labels: map[string]string{
					"app": "my-app",
				},
			},
			Status: api_v1.PodStatus{
				PodIP: ip1,
			},
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name:      "pod2",
				Namespace: api_v1.NamespaceDefault,
				Labels: map[string]string{
					"app": "my-app",
				},
			},
			Status: api_v1.PodStatus{
				PodIP: ip2,
			},
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name:      "pod3",
				Namespace: api_v1.NamespaceDefault,
				Labels: map[string]string{
					"app": "my-app",
				},
			},
			Status: api_v1.PodStatus{
				PodIP: ip3,
			},
		},
	}

	for _, tc := range []struct {
		name          string
		ingress       *extensions.Ingress
		service       *api_v1.Service
		endpoints     *api_v1.Endpoints
		targets       []*elbv2.TargetDescription
		expectedPods  []*api_v1.Pod
		expectedError bool
	}{
		{
			name: "success scenario by numeric service port and numeric pod port",
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
					Selector: map[string]string{
						"app": "my-app",
					},
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
								TargetRef: &api_v1.ObjectReference{
									Kind: "Pod",
									Name: "pod1",
								},
							},
						},
						NotReadyAddresses: []api_v1.EndpointAddress{
							{
								IP: ip2,
								TargetRef: &api_v1.ObjectReference{
									Kind: "Pod",
									Name: "pod2",
								},
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
								TargetRef: &api_v1.ObjectReference{
									Kind: "Pod",
									Name: "pod3",
								},
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
			targets: []*elbv2.TargetDescription{
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
			expectedPods:  pods,
			expectedError: false,
		},
		{
			name: "success scenario by string service port and string pod port",
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
					Selector: map[string]string{
						"app": "my-app",
					},
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
								TargetRef: &api_v1.ObjectReference{
									Kind: "Pod",
									Name: "pod1",
								},
							},
							{
								IP: ip2,
								TargetRef: &api_v1.ObjectReference{
									Kind: "Pod",
									Name: "pod2",
								},
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
								TargetRef: &api_v1.ObjectReference{
									Kind: "Pod",
									Name: "pod3",
								},
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
			targets: []*elbv2.TargetDescription{
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
			expectedPods:  pods,
			expectedError: false,
		},
		{
			name: "failure scenario by no endpoint found",
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
			endpoints:     nil,
			targets:       nil,
			expectedError: true,
		},
		{
			name: "failure scenario by no service found",
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
			service:       nil,
			endpoints:     nil,
			targets:       nil,
			expectedError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cloud := &mocks.CloudAPI{}

			s := store.NewDummy()
			s.GetServiceFunc = func(string) (*api_v1.Service, error) {
				if tc.service != nil {
					return tc.service, nil
				}
				return nil, fmt.Errorf("No such service")
			}
			s.GetServiceEndpointsFunc = func(string) (*api_v1.Endpoints, error) {
				if tc.endpoints != nil {
					return tc.endpoints, nil
				}
				return nil, fmt.Errorf("No such endpoints")
			}
			s.GetPodFunc = func(key string) (*api_v1.Pod, error) {
				for _, pod := range pods {
					if fmt.Sprintf("%s/%s", pod.Namespace, pod.Name) == key {
						return pod, nil
					}
				}
				return nil, store.NotExistsError(key)
			}

			resolver := NewEndpointResolver(s, cloud)
			pods, err := resolver.ReverseResolve(tc.ingress, tc.ingress.Spec.Backend, tc.targets)
			if !reflect.DeepEqual(tc.expectedPods, pods) {
				t.Errorf("expected pods:%#v, actual pods: %#v", tc.expectedPods, pods)
			}
			if (err != nil) != tc.expectedError {
				t.Errorf("expected error:%v, actual err:%v", tc.expectedError, err)
			}
		})
	}
}
