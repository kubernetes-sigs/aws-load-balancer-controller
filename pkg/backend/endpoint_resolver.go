package backend

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EndpointResolver resolves the endpoints for specific service & service Port.
type EndpointResolver interface {
	// ResolvePodEndpoints will resolve endpoints backed by pods directly.
	ResolvePodEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString, opts ...EndpointResolveOption) ([]PodEndpoint, error)

	// ResolveNodePortEndpoints will resolve endpoints backed by nodePort.
	ResolveNodePortEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString, opts ...EndpointResolveOption) ([]NodePortEndpoint, error)
}

// NewDefaultEndpointResolver constructs new defaultEndpointResolver
func NewDefaultEndpointResolver(k8sClient client.Client, logger logr.Logger) *defaultEndpointResolver {
	return &defaultEndpointResolver{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

var _ EndpointResolver = &defaultEndpointResolver{}

// default implementation for EndpointResolver
type defaultEndpointResolver struct {
	k8sClient client.Client
	logger    logr.Logger
}

func (r *defaultEndpointResolver) ResolvePodEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString, opts ...EndpointResolveOption) ([]PodEndpoint, error) {
	resolveOpts := EndpointResolveOptions{
		NodeSelector: labels.Nothing(),
	}
	resolveOpts.ApplyOptions(opts)
	svc, svcPort, err := r.findServiceAndServicePort(ctx, svcKey, port)
	if err != nil {
		return nil, err
	}

	epsKey := k8s.NamespacedName(svc) // k8s Endpoints have same name as k8s Service
	eps := &corev1.Endpoints{}
	if err := r.k8sClient.Get(ctx, epsKey, eps); err != nil {
		return nil, err
	}

	var endpoints []PodEndpoint
	for _, epSubset := range eps.Subsets {
		for _, epPort := range epSubset.Ports {
			// servicePort.Name is optional if there is only one port
			if svcPort.Name != "" && svcPort.Name != epPort.Name {
				continue
			}

			for _, epAddr := range epSubset.Addresses {
				if epAddr.TargetRef == nil || epAddr.TargetRef.Kind != "Pod" {
					continue
				}
				epPod, err := r.findPodByReference(ctx, svc.Namespace, *epAddr.TargetRef)
				if err != nil {
					return nil, err
				}
				endpoints = append(endpoints, buildPodEndpoint(epPod, epAddr, epPort))
			}

			if len(resolveOpts.UnreadyPodInclusionCriteria) != 0 {
				for _, epAddr := range epSubset.NotReadyAddresses {
					if epAddr.TargetRef == nil || epAddr.TargetRef.Kind != "Pod" {
						continue
					}
					epPod, err := r.findPodByReference(ctx, svc.Namespace, *epAddr.TargetRef)
					if err != nil {
						return nil, err
					}
					if isPodMeetCriteria(epPod, resolveOpts.UnreadyPodInclusionCriteria) {
						endpoints = append(endpoints, buildPodEndpoint(epPod, epAddr, epPort))
					}
				}
			}
		}
	}

	return endpoints, nil
}

func (r *defaultEndpointResolver) ResolveNodePortEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString, opts ...EndpointResolveOption) ([]NodePortEndpoint, error) {
	resolveOpts := EndpointResolveOptions{
		NodeSelector: labels.Nothing(),
	}
	resolveOpts.ApplyOptions(opts)
	svc, svcPort, err := r.findServiceAndServicePort(ctx, svcKey, port)
	if err != nil {
		return nil, err
	}

	if svc.Spec.Type != corev1.ServiceTypeNodePort && svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return nil, errors.Errorf("service type must be either 'NodePort' or 'LoadBalancer': %v", svcKey)
	}
	svcNodePort := svcPort.NodePort
	nodeList := &corev1.NodeList{}
	if err := r.k8sClient.List(ctx, nodeList, client.MatchingLabelsSelector{Selector: resolveOpts.NodeSelector}); err != nil {
		return nil, err
	}

	var endpoints []NodePortEndpoint
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		if !k8s.IsNodeReady(node) {
			continue
		}
		instanceID, err := k8s.ExtractNodeInstanceID(node)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, buildNodePortEndpoint(node, instanceID, svcNodePort))
	}

	return endpoints, nil
}

func (r *defaultEndpointResolver) findServiceAndServicePort(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString) (*corev1.Service, corev1.ServicePort, error) {
	svc := &corev1.Service{}
	if err := r.k8sClient.Get(ctx, svcKey, svc); err != nil {
		return nil, corev1.ServicePort{}, err
	}
	svcPort, err := k8s.LookupServicePort(svc, port)
	if err != nil {
		return nil, corev1.ServicePort{}, err
	}

	return svc, svcPort, nil
}

func (r *defaultEndpointResolver) findPodByReference(ctx context.Context, namespace string, podRef corev1.ObjectReference) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	podKey := types.NamespacedName{Namespace: namespace, Name: podRef.Name}
	if err := r.k8sClient.Get(ctx, podKey, pod); err != nil {
		return nil, err
	}
	return pod, nil
}

func isPodMeetCriteria(pod *corev1.Pod, criteria []PodPredicate) bool {
	for _, criterion := range criteria {
		if !criterion(pod) {
			return false
		}
	}
	return true
}

func buildPodEndpoint(pod *corev1.Pod, epAddr corev1.EndpointAddress, epPort corev1.EndpointPort) PodEndpoint {
	return PodEndpoint{
		IP:   epAddr.IP,
		Port: int64(epPort.Port),
		Pod:  pod,
	}
}

func buildNodePortEndpoint(node *corev1.Node, instanceID string, nodePort int32) NodePortEndpoint {
	return NodePortEndpoint{
		InstanceID: instanceID,
		Port:       int64(nodePort),
		Node:       node,
	}
}
