package backend

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// An endpoint provided by pod directly.
type PodEndpoint struct {
	// Pod's IP.
	IP string
	// Pod's container port.
	Port int64
	// Pod that provides this endpoint.
	Pod *corev1.Pod
}

// An endpoint provided by nodePort as trafficProxy.
type NodePortEndpoint struct {
	// Node's instanceID.
	InstanceID string
	// Node's NodePort.
	Port int64
	// Node that provides this endpoint.
	Node *corev1.Node
}

type PodPredicate func(pod *corev1.Pod) bool

// options for Endpoints resolve APIs
type EndpointResolveOptions struct {
	// [Pod Endpoint] unready pods that have passed all these inclusion criteria will be included as well. (ready pods will always be included)
	// If it's empty, unready pods won't be included.
	// If it's non-empty, unready pods passed *all* criteria will be included.
	// By default, it's empty.
	UnreadyPodInclusionCriteria []PodPredicate

	// [NodePort Endpoint] only nodes that are ready and matched by nodeSelector will be included.
	// By default, no node will be selected.
	NodeSelector labels.Selector
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

// WithUnreadyPodInclusionCriterion is a option that appends additional criterion into UnreadyPodInclusionCriteria.
func WithUnreadyPodInclusionCriterion(predicate PodPredicate) EndpointResolveOption {
	return func(opts *EndpointResolveOptions) {
		opts.UnreadyPodInclusionCriteria = append(opts.UnreadyPodInclusionCriteria, predicate)
	}
}
