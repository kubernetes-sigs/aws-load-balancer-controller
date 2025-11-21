package aga

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwclientset "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

// ServiceClient adapts Kubernetes Service client to ResourceClient
type ServiceClient struct {
	client    kubernetes.Interface
	namespace string
}

func NewServiceClient(client kubernetes.Interface, namespace string) *ServiceClient {
	return &ServiceClient{
		client:    client,
		namespace: namespace,
	}
}

func (c *ServiceClient) List(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
	return c.client.CoreV1().Services(c.namespace).List(ctx, opts)
}

func (c *ServiceClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return c.client.CoreV1().Services(c.namespace).Watch(ctx, opts)
}

// IngressClient adapts Kubernetes Ingress client to ResourceClient
type IngressClient struct {
	client    kubernetes.Interface
	namespace string
}

func NewIngressClient(client kubernetes.Interface, namespace string) *IngressClient {
	return &IngressClient{
		client:    client,
		namespace: namespace,
	}
}

func (c *IngressClient) List(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
	return c.client.NetworkingV1().Ingresses(c.namespace).List(ctx, opts)
}

func (c *IngressClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return c.client.NetworkingV1().Ingresses(c.namespace).Watch(ctx, opts)
}

// GatewayClient adapts Gateway API client to ResourceClient
type GatewayClient struct {
	client    gwclientset.Interface
	namespace string
}

func NewGatewayClient(client gwclientset.Interface, namespace string) *GatewayClient {
	return &GatewayClient{
		client:    client,
		namespace: namespace,
	}
}

func (c *GatewayClient) List(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
	return c.client.GatewayV1().Gateways(c.namespace).List(ctx, opts)
}

func (c *GatewayClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return c.client.GatewayV1().Gateways(c.namespace).Watch(ctx, opts)
}

// Create example objects for type info
var (
	ExampleService = &corev1.Service{}
	ExampleIngress = &networkingv1.Ingress{}
	ExampleGateway = &gwv1.Gateway{}
)
