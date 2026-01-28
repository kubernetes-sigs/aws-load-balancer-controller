package backend

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
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

	Cleanup(ctx context.Context, svcKey *elbv2api.TargetGroupBinding)
}

// NewDefaultEndpointResolver constructs new defaultEndpointResolver
func NewDefaultEndpointResolver(k8sClient client.Client, podInfoRepo k8s.PodInfoRepo, failOpenEnabled bool, endpointSliceEnabled bool, logger logr.Logger) *defaultEndpointResolver {
	return &defaultEndpointResolver{
		k8sClient:            k8sClient,
		podInfoRepo:          podInfoRepo,
		failOpenEnabled:      failOpenEnabled,
		endpointSliceEnabled: endpointSliceEnabled,
		logger:               logger,

		externalNameServices:    make(map[types.NamespacedName]string),
		externalNameReconcilers: make(map[string]externalNameReconciler),
		externalNameMutex:       sync.Mutex{},
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

	externalNameServices    map[types.NamespacedName]string
	externalNameReconcilers map[string]externalNameReconciler
	externalNameMutex       sync.Mutex
}

type externalNameReconciler struct {
	cancel       context.CancelFunc
	externalName string
	services     map[types.NamespacedName]int32
}

func (r *defaultEndpointResolver) Cleanup(ctx context.Context, tgb *elbv2api.TargetGroupBinding) {
	qualifiedSvcName := types.NamespacedName{
		Namespace: tgb.Namespace,
		Name:      tgb.Spec.ServiceRef.Name,
	}
	r.externalNameMutex.Lock()
	defer r.externalNameMutex.Unlock()
	externalName, ok := r.externalNameServices[qualifiedSvcName]
	if !ok {
		return
	}
	delete(r.externalNameServices, qualifiedSvcName)
	r.removeReconciler(externalName, qualifiedSvcName)
	go func() {
		if err := r.k8sClient.DeleteAllOf(ctx, &discovery.EndpointSlice{},
			client.InNamespace(tgb.Namespace),
			client.MatchingLabels{
				discovery.LabelServiceName: tgb.Spec.ServiceRef.Name,
				discovery.LabelManagedBy:   "aws-load-balancer-controller",
			}); err != nil {
			r.logger.Error(err, "failed to delete EndpointSlices", "service", tgb.Spec.ServiceRef.Name)
		}
	}()
}

func (r *defaultEndpointResolver) removeReconciler(externalName string, qualifiedSvcName types.NamespacedName) {
	reconciler, ok := r.externalNameReconcilers[externalName]
	if !ok {
		return
	}
	delete(reconciler.services, qualifiedSvcName)
	if len(reconciler.services) > 0 {
		return
	}
	reconciler.cancel()
	delete(r.externalNameReconcilers, externalName)
	// Delete endpoint slice generated for ExternalName service
	return
}

func (r *defaultEndpointResolver) ResolvePodEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString, opts ...EndpointResolveOption) ([]PodEndpoint, bool, error) {
	resolveOpts := defaultEndpointResolveOptions()
	resolveOpts.ApplyOptions(opts)

	svc, svcPort, err := r.findServiceAndServicePort(ctx, svcKey, port)
	if err != nil {
		return nil, false, err
	}
	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		err = r.startReconcileExternalNameEndpointSlice(ctx, svc, port)
		if err != nil {
			return nil, false, fmt.Errorf("failed to reconcile external name service: %w", err)
		}
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
			epsPortName := awssdk.ToString(port.Name)
			if len(svcPort.Name) != 0 && len(epsPortName) != 0 && svcPort.Name != epsPortName {
				continue
			}
			epPort := awssdk.ToInt32(port.Port)
			for _, ep := range epsData.Endpoints {
				if len(ep.Addresses) == 0 {
					continue // this should never happen per specification.
				}

				if ep.TargetRef == nil || ep.TargetRef.Kind != "Pod" {
					for _, epAddr := range ep.Addresses {
						readyPodEndpoints = append(readyPodEndpoints,
							PodEndpoint{
								IP:   epAddr,
								Port: epPort,
							})
					}
				} else {
					epAddr := ep.Addresses[0]
					podNamespace := svcKey.Namespace
					if ep.TargetRef.Namespace != "" {
						podNamespace = ep.TargetRef.Namespace
					}
					podKey := types.NamespacedName{Namespace: podNamespace, Name: ep.TargetRef.Name}
					pod, exists, err := r.podInfoRepo.Get(ctx, podKey)
					if err != nil {
						return nil, false, err
					}
					if !exists {
						r.logger.Info("the pod in endpoint is not found in pod cache yet, will keep retrying", "podKey", podKey.String())
						containsPotentialReadyEndpoints = true
						continue
					}

					podEndpoint := buildPodEndpoint(pod, epAddr, epPort)
					// Recommendation from Kubernetes is to consider unknown ready status as ready (ready == nil)
					if ep.Conditions.Ready == nil || *ep.Conditions.Ready {
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
		IP:           epAddr,
		Port:         port,
		Pod:          &pod,
		QuicServerID: pod.GetQUICServerID(port),
	}
}

func buildNodePortEndpoint(node *corev1.Node, instanceID string, nodePort int32) NodePortEndpoint {
	return NodePortEndpoint{
		InstanceID: instanceID,
		Port:       nodePort,
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

func (r *defaultEndpointResolver) watchExternalName(ctx context.Context, externalName string, reconciler externalNameReconciler) {
	ttl, err := r.reconcileExternalNameEndpointSlice(ctx, externalName, reconciler)
	if ttl == math.MaxUint32 || ttl < 10 {
		// Try again in 10 seconds if no valid TTL was found. Don't query in less than 10 seconds to avoid churn
		ttl = 10
	}
	ticker := time.NewTicker(time.Second * time.Duration(ttl))
	defer ticker.Stop()

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case _ = <-ticker.C:
		go r.watchExternalName(ctx, externalName, reconciler)
	}

	if err != nil && err != context.Canceled {
		r.logger.Error(err, "failure in watching externalname",
			"externalname", externalName)
	}
}

func (r *defaultEndpointResolver) startReconcileExternalNameEndpointSlice(ctx context.Context, svc *corev1.Service, port intstr.IntOrString) error {
	if !r.endpointSliceEnabled {
		return fmt.Errorf("using external name service is not supported when endpoint slice is disabled")
	}
	qualifiedSvcName := k8s.NamespacedName(svc)
	r.externalNameMutex.Lock()
	defer r.externalNameMutex.Unlock()

	externalName, ok := r.externalNameServices[qualifiedSvcName]
	if externalName == svc.Spec.ExternalName {
		return nil
	}
	if ok {
		r.removeReconciler(externalName, qualifiedSvcName)
	}

	currentExternalName := svc.Spec.ExternalName
	r.externalNameServices[qualifiedSvcName] = currentExternalName
	reconciler, ok := r.externalNameReconcilers[currentExternalName]
	if ok {
		reconciler.cancel()
		reconciler.services[qualifiedSvcName] = port.IntVal
	} else {
		reconciler = externalNameReconciler{
			services: map[types.NamespacedName]int32{qualifiedSvcName: port.IntVal},
		}
		r.externalNameReconcilers[currentExternalName] = reconciler
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	reconciler.cancel = cancel
	go r.watchExternalName(cancelCtx, currentExternalName, reconciler)
	return nil
}

func (r *defaultEndpointResolver) reconcileExternalNameEndpointSlice(ctx context.Context, externalName string, reconciler externalNameReconciler) (uint32, error) {
	dnsConf, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return 0, fmt.Errorf("failing to read /etc/resolv.conf: %w", err)
	}

	addresses, minTTL := r.resolveExternalName(dnsConf, externalName)
	for svcKey, portNumber := range reconciler.services {
		labels := map[string]string{
			discovery.LabelManagedBy:   "aws-load-balancer-controller",
			discovery.LabelServiceName: svcKey.Name,
		}
		epSliceList := &discovery.EndpointSliceList{}
		if err := r.k8sClient.List(ctx, epSliceList,
			client.InNamespace(svcKey.Namespace),
			client.MatchingLabels(labels)); err != nil {
			return minTTL, fmt.Errorf("failing to list EndpointSlices: %w", err)
		}
		sort.Strings(addresses)
		if len(epSliceList.Items) == 0 {
			if len(addresses) == 0 {
				return minTTL, nil
			}
			// Create EndPointSlice
			epSlice := &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    svcKey.Namespace,
					GenerateName: svcKey.Name + "-",
					Labels:       labels,
				},
				AddressType: discovery.AddressTypeIPv4,
				Endpoints: []discovery.Endpoint{
					{
						Addresses: addresses,
					},
				},
				Ports: []discovery.EndpointPort{
					{
						Port: &portNumber,
					},
				},
			}
			r.logger.V(1).Info("creating EndpointSlice",
				"addresses", addresses,
				"service", svcKey.Name,
				"namespace", svcKey.Namespace,
			)
			if err := r.k8sClient.Create(ctx, epSlice); err != nil {
				return minTTL, fmt.Errorf("failing to create EndpointSlice: %w", err)
			}
		} else if len(epSliceList.Items) == 1 {
			// Synchronize EndPointSlice
			persistedES := epSliceList.Items[0]
			if len(addresses) == 0 {
				if err = r.k8sClient.Delete(ctx, &persistedES); err != nil {
					return minTTL, fmt.Errorf("failing to delete EndpointSlice: %w", err)
				}
			} else {
				persistedAddresses := persistedES.Endpoints[0].Addresses
				if !reflect.DeepEqual(persistedAddresses, addresses) {
					persistedES.Endpoints[0].Addresses = addresses
					r.logger.V(1).Info("updating EndpointSlice",
						"name", persistedES.GetName(),
						"addresses", addresses,
						"service", svcKey.Name,
						"namespace", svcKey.Namespace,
					)
					if err = r.k8sClient.Update(ctx, &persistedES); err != nil {
						return minTTL, fmt.Errorf("failing to update EndpointSlice: %w", err)
					}
				}
			}
		} else {
			return minTTL, errors.New("multiple endpoint slice resources found")
		}
	}
	return minTTL, nil
}

func (r *defaultEndpointResolver) resolveExternalName(dnsConf *dns.ClientConfig, externalName string) ([]string, uint32) {
	var addresses []string
	var minTTL uint32
	minTTL = math.MaxUint32
	c := new(dns.Client)
	c.Net = "udp"

	for _, server := range dnsConf.Servers {
		for _, domain := range dnsConf.NameList(externalName) {
			// Since svc.Spec.IPFamilies is nil for services with svc.Spec.Type = corev1.ServiceTypeExternalName
			// targetGroup.Spec.IPAddressType = "ipv4". To support AAAA the logic for setting IPAddressType would need to be changed.
			qtype := dns.TypeA
			m := new(dns.Msg)
			m.SetQuestion(dns.Fqdn(domain), qtype)
			m.RecursionDesired = true
			resp, _, err := c.Exchange(m, server+":53")
			if err != nil {
				r.logger.V(1).Info("DNS query for external name failed",
					"server", server,
					"domain", domain,
					"error", err)
				continue
			}
			if resp.Rcode != dns.RcodeSuccess {
				r.logger.V(1).Info("DNS query for external name failed",
					"server", server,
					"domain", domain,
					"error", dns.RcodeToString[resp.Rcode])
				continue
			}

			for _, ans := range resp.Answer {
				minTTL = min(ans.Header().Ttl, minTTL)
				if a, ok := ans.(*dns.A); ok {
					addresses = append(addresses, a.A.String())
				}
			}
			if addresses != nil {
				r.logger.V(1).Info("got DNS response for external name",
					"server", server,
					"addresses", addresses,
					"domain", domain)
				return addresses, minTTL
			}
		}
	}
	return nil, minTTL
}
