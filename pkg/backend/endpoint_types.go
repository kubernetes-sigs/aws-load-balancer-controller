package backend

import (
	corev1 "k8s.io/api/core/v1"
	discv1 "k8s.io/api/discovery/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
)

// An endpoint provided by pod directly.
type PodEndpoint struct {
	// Pod's IP.
	IP string
	// Pod's container port.
	Port int64
	// Pod that provides this endpoint.
	Pod k8s.PodInfo
}

// An endpoint provided by nodePort as traffic proxy.
type NodePortEndpoint struct {
	// Node's instanceID.
	InstanceID string
	// Node's NodePort.
	Port int64
	// Node that provides this endpoint.
	Node *corev1.Node
}

type EndpointsData struct {
	Ports     []discv1.EndpointPort
	Endpoints []discv1.Endpoint
}

// options for Endpoints resolve APIs
type EndpointResolveOptions struct {
	// [NodePort Endpoint] only nodes that are matched by nodeSelector will be included.
	// By default, no node will be selected.
	NodeSelector labels.Selector

	// [Pod Endpoint] if pod readinessGates is defined, then pods from unready addresses with any of these readinessGates and containersReady condition will be included as well.
	// By default, no readinessGate is specified.
	PodReadinessGates []corev1.PodConditionType
}

func (opts *EndpointResolveOptions) ApplyOptions(options []EndpointResolveOption) {
	for _, option := range options {
		option(opts)
	}
}

type EndpointResolveOption func(opts *EndpointResolveOptions)

// WithNodeSelector is a option that sets nodeSelector.
func WithNodeSelector(nodeSelector labels.Selector) EndpointResolveOption {
	return func(opts *EndpointResolveOptions) {
		opts.NodeSelector = nodeSelector
	}
}

// WithPodReadinessGate is a option that appends podReadinessGate into EndpointResolveOptions.
func WithPodReadinessGate(cond corev1.PodConditionType) EndpointResolveOption {
	return func(opts *EndpointResolveOptions) {
		opts.PodReadinessGates = append(opts.PodReadinessGates, cond)
	}
}

// defaultEndpointResolveOptions returns the default value for EndpointResolveOptions.
func defaultEndpointResolveOptions() EndpointResolveOptions {
	return EndpointResolveOptions{
		NodeSelector:      labels.Nothing(),
		PodReadinessGates: nil,
	}
}
