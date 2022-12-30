package backend

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var ErrNotFound = errors.New("backend not found")

// TODO: for pod endpoints, we currently rely on endpoints events, we might change to use pod events directly in the future.
// under current implementation with pod readinessGate enabled, an unready endpoint but not match our inclusionCriteria won't be registered,
// and it won't turn ready due to blocked by readinessGate, and no future endpoint events will trigger.
// We solve this by requeue the TGB if unready endpoints have the potential to be ready if reconcile in later time.

// EndpointResolver resolves the endpoints for specific service & service Port.
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
func NewDefaultEndpointResolver(k8sClient client.Client, podInfoRepo k8s.PodInfoRepo, failOpenEnabled bool, endpointSliceEnabled bool, logger logr.Logger) *defaultEndpointResolver {
	return &defaultEndpointResolver{
		k8sClient:            k8sClient,
		podInfoRepo:          podInfoRepo,
		failOpenEnabled:      failOpenEnabled,
		endpointSliceEnabled: endpointSliceEnabled,
		logger:               logger,
	}
}

var _ EndpointResolver = &defaultEndpointResolver{}

// default implementation for EndpointResolver
type defaultEndpointResolver struct {
	k8sClient   client.Client
	podInfoRepo k8s.PodInfoRepo
	// [NodePort Endpoint] if fail-open enabled, then nodes that have `Unknown` ready condition will be included if there is no other node with `True` ready condition.
	// [Pod Endpoint] if fail-open enabled, then containerRead pods on nodes that have `Unknown` ready condition will be included if there is no other pods that are ready.
	failOpenEnabled bool
	// [Pod Endpoint] whether to use endpointSlice instead of endpoints
	endpointSliceEnabled bool
	logger               logr.Logger
}

func (r *defaultEndpointResolver) ResolvePodEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString, opts ...EndpointResolveOption) ([]PodEndpoint, bool, error) {
	resolveOpts := defaultEndpointResolveOptions()
	resolveOpts.ApplyOptions(opts)

	_, svcPort, err := r.findServiceAndServicePort(ctx, svcKey, port)
	if err != nil {
		return nil, false, err
	}
	endpointsDataList, err := r.computeServiceEndpointsData(ctx, svcKey)
	if err != nil {
		return nil, false, err
	}
	return r.resolvePodEndpointsWithEndpointsData(ctx, svcKey, svcPort, endpointsDataList, resolveOpts.PodReadinessGates)
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

	var candidateNodes []*corev1.Node
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		if IsNodeSuitableAsTrafficProxy(node) {
			candidateNodes = append(candidateNodes, node)
		}
	}

	targetNodes := filterNodesByReadyConditionStatus(candidateNodes, corev1.ConditionTrue)
	if r.failOpenEnabled && len(targetNodes) == 0 {
		targetNodes = filterNodesByReadyConditionStatus(candidateNodes, corev1.ConditionUnknown)
	}

	var endpoints []NodePortEndpoint
	for _, node := range targetNodes {
		instanceID, err := k8s.ExtractNodeInstanceID(node)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, buildNodePortEndpoint(node, instanceID, svcNodePort))
	}
	return endpoints, nil
}

func (r *defaultEndpointResolver) computeServiceEndpointsData(ctx context.Context, svcKey types.NamespacedName) ([]EndpointsData, error) {
	var endpointsDataList []EndpointsData
	if r.endpointSliceEnabled {
		epSliceList := &discovery.EndpointSliceList{}
		if err := r.k8sClient.List(ctx, epSliceList,
			client.InNamespace(svcKey.Namespace),
			client.MatchingLabels{discovery.LabelServiceName: svcKey.Name}); err != nil {
			return nil, err
		}
		endpointsDataList = buildEndpointsDataFromEndpointSliceList(epSliceList)
	} else {
		eps := &corev1.Endpoints{}
		if err := r.k8sClient.Get(ctx, svcKey, eps); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("%w: %v", ErrNotFound, err.Error())
			}
			return nil, err
		}
		endpointsDataList = buildEndpointsDataFromEndpoints(eps)
	}

	return endpointsDataList, nil
}

func (r *defaultEndpointResolver) resolvePodEndpointsWithEndpointsData(ctx context.Context, svcKey types.NamespacedName, svcPort corev1.ServicePort, endpointsDataList []EndpointsData, podReadinessGates []corev1.PodConditionType) ([]PodEndpoint, bool, error) {
	var readyPodEndpoints []PodEndpoint
	var unknownPodEndpoints []PodEndpoint
	containsPotentialReadyEndpoints := false

	for _, epsData := range endpointsDataList {
		for _, port := range epsData.Ports {
			if len(svcPort.Name) != 0 && svcPort.Name != awssdk.StringValue(port.Name) {
				continue
			}
			epPort := awssdk.Int32Value(port.Port)
			for _, ep := range epsData.Endpoints {
				if ep.TargetRef == nil || ep.TargetRef.Kind != "Pod" {
					continue
				}
				if len(ep.Addresses) == 0 {
					continue // this should never happen per specification.
				}
				epAddr := ep.Addresses[0]

				podKey := types.NamespacedName{Namespace: svcKey.Namespace, Name: ep.TargetRef.Name}
				pod, exists, err := r.podInfoRepo.Get(ctx, podKey)
				if err != nil {
					return nil, false, err
				}
				if !exists {
					r.logger.Info("ignore pod Endpoint with non-existent podInfo", "podKey", podKey.String())
					continue
				}
				podEndpoint := buildPodEndpoint(pod, epAddr, epPort)
				if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
					readyPodEndpoints = append(readyPodEndpoints, podEndpoint)
					continue
				}

				if !pod.IsContainersReady() {
					if pod.HasAnyOfReadinessGates(podReadinessGates) {
						containsPotentialReadyEndpoints = true
					}
					continue
				}

				node := &corev1.Node{}
				if err := r.k8sClient.Get(ctx, types.NamespacedName{Name: pod.NodeName}, node); err != nil {
					r.logger.Error(err, "ignore pod Endpoint without non-exist nodeInfo", "podKey", podKey.String())
					continue
				}

				nodeReadyCondStatus := corev1.ConditionFalse
				if readyCond := k8s.GetNodeCondition(node, corev1.NodeReady); readyCond != nil {
					nodeReadyCondStatus = readyCond.Status
				}
				switch nodeReadyCondStatus {
				case corev1.ConditionTrue:
					// start from 1.22+, terminating pods are included in endpointSlices,
					// and we don't want to include these pods if the node is known to be healthy.
					if ep.Conditions.Terminating == nil || !*ep.Conditions.Terminating {
						readyPodEndpoints = append(readyPodEndpoints, podEndpoint)
					}
				case corev1.ConditionUnknown:
					unknownPodEndpoints = append(unknownPodEndpoints, podEndpoint)
				}
			}
		}
	}
	podEndpoints := readyPodEndpoints
	if r.failOpenEnabled && len(podEndpoints) == 0 {
		podEndpoints = unknownPodEndpoints
	}
	return podEndpoints, containsPotentialReadyEndpoints, nil
}

func (r *defaultEndpointResolver) findServiceAndServicePort(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString) (*corev1.Service, corev1.ServicePort, error) {
	svc := &corev1.Service{}
	if err := r.k8sClient.Get(ctx, svcKey, svc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, corev1.ServicePort{}, fmt.Errorf("%w: %v", ErrNotFound, err.Error())
		}
		return nil, corev1.ServicePort{}, err
	}
	svcPort, err := k8s.LookupServicePort(svc, port)
	if err != nil {
		return nil, corev1.ServicePort{}, fmt.Errorf("%w: %v", ErrNotFound, err.Error())
	}

	return svc, svcPort, nil
}

// filterNodesByReadyConditionStatus will filter out nodes that matches specified ready condition status
func filterNodesByReadyConditionStatus(nodes []*corev1.Node, readyCondStatus corev1.ConditionStatus) []*corev1.Node {
	var nodesWithMatchingReadyStatus []*corev1.Node
	for _, node := range nodes {
		if readyCond := k8s.GetNodeCondition(node, corev1.NodeReady); readyCond != nil && readyCond.Status == readyCondStatus {
			nodesWithMatchingReadyStatus = append(nodesWithMatchingReadyStatus, node)
		}
	}
	return nodesWithMatchingReadyStatus
}

func buildEndpointsDataFromEndpoints(eps *corev1.Endpoints) []EndpointsData {
	var endpointsDataList []EndpointsData
	for _, epSubset := range eps.Subsets {
		var endpointPorts []discovery.EndpointPort
		for _, port := range epSubset.Ports {
			endpointPort := convertCoreEndpointPortToDiscoveryEndpointPort(port)
			endpointPorts = append(endpointPorts, endpointPort)
		}

		var endpoints []discovery.Endpoint
		for _, set := range []struct {
			ready     bool
			addresses []corev1.EndpointAddress
		}{
			{true, epSubset.Addresses},
			{false, epSubset.NotReadyAddresses},
		} {
			for _, addr := range set.addresses {
				endpoint := convertCoreEndpointAddressToDiscoveryEndpoint(addr, set.ready)
				endpoints = append(endpoints, endpoint)
			}
		}
		endpointsDataList = append(endpointsDataList, EndpointsData{
			Ports:     endpointPorts,
			Endpoints: endpoints,
		})
	}
	return endpointsDataList
}

func buildEndpointsDataFromEndpointSliceList(epsList *discovery.EndpointSliceList) []EndpointsData {
	var endpointsDataList []EndpointsData
	for _, epSlice := range epsList.Items {
		endpointsDataList = append(endpointsDataList, EndpointsData{
			Ports:     epSlice.Ports,
			Endpoints: epSlice.Endpoints,
		})
	}
	return endpointsDataList
}

func buildPodEndpoint(pod k8s.PodInfo, epAddr string, port int32) PodEndpoint {
	return PodEndpoint{
		IP:   epAddr,
		Port: int64(port),
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

// convertCoreEndpointPortToDiscoveryEndpointPort converts a EndpointPort in core APIGroup into EndpointPort in discovery APIGroup
func convertCoreEndpointPortToDiscoveryEndpointPort(port corev1.EndpointPort) discovery.EndpointPort {
	epPort := discovery.EndpointPort{
		Port:        awssdk.Int32(port.Port),
		AppProtocol: port.AppProtocol,
	}
	if len(port.Protocol) != 0 {
		epPort.Protocol = &port.Protocol
	}
	if len(port.Name) != 0 {
		epPort.Name = awssdk.String(port.Name)
	}
	return epPort
}

// convertCoreEndpointAddressToDiscoveryEndpoint converts a EndpointAddress in core APIGroup into an Endpoint in discovery APIGroup along with its ready status.
func convertCoreEndpointAddressToDiscoveryEndpoint(endpoint corev1.EndpointAddress, ready bool) discovery.Endpoint {
	ep := discovery.Endpoint{
		Addresses: []string{endpoint.IP},
		Conditions: discovery.EndpointConditions{
			Ready:       awssdk.Bool(ready),
			Serving:     awssdk.Bool(ready),
			Terminating: awssdk.Bool(false),
		},
		TargetRef: endpoint.TargetRef,
		NodeName:  endpoint.NodeName,
	}
	if len(endpoint.Hostname) != 0 {
		ep.Hostname = awssdk.String(endpoint.Hostname)
	}
	return ep
}
