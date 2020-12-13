package backend

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO: for pod endpoints, we currently rely on endpoints events, we might change to use pod events directly in the future.
// under current implementation with pod readinessGate enabled, an unready endpoint but not match our inclusionCriteria won't be registered,
// and it won't turn ready due to blocked by readinessGate, and no future endpoint events will trigger.
// We solve this by requeue the TGB if unready endpoints have the potential to be ready if reconcile in later time.

//  EndpointResolver resolves the endpoints for specific service & service Port.
type EndpointResolver interface {
	// ResolvePodEndpoints will resolve endpoints backed by pods directly.
	// returns resolved podEndpoints and whether there are unready endpoints that can potentially turn ready in future reconciles.
	ResolvePodEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString,
		opts ...EndpointResolveOption) ([]PodEndpoint, bool, error)

	// ResolveNodePortEndpoints will resolve endpoints backed by nodePort.
	ResolveNodePortEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString,
		opts ...EndpointResolveOption) ([]NodePortEndpoint, error)
}

// NewDefaultEndpointResolver constructs new defaultEndpointResolver
func NewDefaultEndpointResolver(k8sClient client.Client, podInfoRepo k8s.PodInfoRepo, logger logr.Logger) *defaultEndpointResolver {
	return &defaultEndpointResolver{
		k8sClient:   k8sClient,
		podInfoRepo: podInfoRepo,
		logger:      logger,
	}
}

var _ EndpointResolver = &defaultEndpointResolver{}

// default implementation for EndpointResolver
type defaultEndpointResolver struct {
	k8sClient   client.Client
	podInfoRepo k8s.PodInfoRepo
	logger      logr.Logger
}

func (r *defaultEndpointResolver) ResolvePodEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString,
	opts ...EndpointResolveOption) ([]PodEndpoint, bool, error) {
	resolveOpts := defaultEndpointResolveOptions()
	resolveOpts.ApplyOptions(opts)

	svc, svcPort, err := r.findServiceAndServicePort(ctx, svcKey, port)
	if err != nil {
		return nil, false, err
	}
	epsKey := k8s.NamespacedName(svc) // k8s Endpoints have same name as k8s Service
	eps := &corev1.Endpoints{}
	if err := r.k8sClient.Get(ctx, epsKey, eps); err != nil {
		return nil, false, err
	}

	containsPotentialReadyEndpoints := false
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
				pod, exists, err := r.findPodByReference(ctx, svc.Namespace, *epAddr.TargetRef)
				if err != nil {
					return nil, false, err
				}
				if !exists {
					return nil, false, errors.New("couldn't find podInfo for ready endpoint")
				}
				endpoints = append(endpoints, buildPodEndpoint(pod, epAddr, epPort))
			}

			if len(resolveOpts.PodReadinessGates) != 0 {
				for _, epAddr := range epSubset.NotReadyAddresses {
					if epAddr.TargetRef == nil || epAddr.TargetRef.Kind != "Pod" {
						continue
					}
					pod, exists, err := r.findPodByReference(ctx, svc.Namespace, *epAddr.TargetRef)
					if err != nil {
						return nil, false, err
					}
					if !exists {
						containsPotentialReadyEndpoints = true
						continue
					}
					if !pod.HasAnyOfReadinessGates(resolveOpts.PodReadinessGates) {
						continue
					}
					if !pod.IsContainersReady() {
						containsPotentialReadyEndpoints = true
						continue
					}
					endpoints = append(endpoints, buildPodEndpoint(pod, epAddr, epPort))
				}
			}
		}
	}

	return endpoints, containsPotentialReadyEndpoints, nil
}

func (r *defaultEndpointResolver) ResolveNodePortEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString, opts ...EndpointResolveOption) ([]NodePortEndpoint, error) {
	resolveOpts := defaultEndpointResolveOptions()
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
		if !k8s.IsNodeSuitableAsTrafficProxy(node) {
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

func (r *defaultEndpointResolver) findPodByReference(ctx context.Context, namespace string, podRef corev1.ObjectReference) (k8s.PodInfo, bool, error) {
	podKey := types.NamespacedName{Namespace: namespace, Name: podRef.Name}
	return r.podInfoRepo.Get(ctx, podKey)
}

func buildPodEndpoint(pod k8s.PodInfo, epAddr corev1.EndpointAddress, epPort corev1.EndpointPort) PodEndpoint {
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
