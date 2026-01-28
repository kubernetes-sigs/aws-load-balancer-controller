package aga

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwclientset "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

// EndpointResourcesManager manages watches for resources referenced by GlobalAccelerator endpoints
type EndpointResourcesManager interface {
	// MonitorEndpointResources updates the watches based on resources referenced by a GA
	MonitorEndpointResources(ga *agaapi.GlobalAccelerator, endpoints []*LoadedEndpoint)

	// RemoveGA removes all watches for resources referenced by a GA being deleted
	RemoveGA(gaKey ktypes.NamespacedName)

	// HasGatewaySupport checks if Gateway API CRDs are supported
	HasGatewaySupport() bool
}

type defaultEndpointResourcesManager struct {
	mutex sync.Mutex
	// TODO: Refactor to encapsulate map access behind accessor methods to prevent accidental direct access without mutex lock
	serviceWatches   map[ktypes.NamespacedName]*ResourceWatcher
	ingressWatches   map[ktypes.NamespacedName]*ResourceWatcher
	gatewayWatches   map[ktypes.NamespacedName]*ResourceWatcher
	serviceEventChan chan<- event.GenericEvent
	ingressEventChan chan<- event.GenericEvent
	gatewayEventChan chan<- event.GenericEvent
	clientSet        kubernetes.Interface
	gatewayClient    gwclientset.Interface
	logger           logr.Logger
}

// NewEndpointResourcesManager creates a new manager
func NewEndpointResourcesManager(
	clientSet kubernetes.Interface,
	gatewayClient gwclientset.Interface,
	serviceEventChan chan<- event.GenericEvent,
	ingressEventChan chan<- event.GenericEvent,
	gatewayEventChan chan<- event.GenericEvent,
	logger logr.Logger) EndpointResourcesManager {

	return &defaultEndpointResourcesManager{
		serviceWatches:   make(map[ktypes.NamespacedName]*ResourceWatcher),
		ingressWatches:   make(map[ktypes.NamespacedName]*ResourceWatcher),
		gatewayWatches:   make(map[ktypes.NamespacedName]*ResourceWatcher),
		serviceEventChan: serviceEventChan,
		ingressEventChan: ingressEventChan,
		gatewayEventChan: gatewayEventChan,
		clientSet:        clientSet,
		gatewayClient:    gatewayClient,
		logger:           logger,
	}
}

var _ EndpointResourcesManager = &defaultEndpointResourcesManager{}

// MonitorEndpointResources updates the watches based on resources referenced by a GA
func (m *defaultEndpointResourcesManager) MonitorEndpointResources(ga *agaapi.GlobalAccelerator, endpoints []*LoadedEndpoint) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	gaID := k8s.NamespacedName(ga).String()

	// Get all references from the GA
	serviceRefs := sets.NewString()
	ingressRefs := sets.NewString()
	gatewayRefs := sets.NewString()
	for _, endpoint := range endpoints {
		// Check if this is a cross-namespace reference that's not allowed
		if endpoint.Namespace != "" && endpoint.Namespace != ga.Namespace && endpoint.Type != agaapi.GlobalAcceleratorEndpointTypeEndpointID && !endpoint.CrossNamespaceAllowed {
			m.logger.Info("Skipping cross-namespace reference monitoring - not allowed",
				"endpointType", endpoint.Type,
				"endpointNamespace", endpoint.Namespace,
				"endpointName", endpoint.Name,
				"gaNamespace", ga.Namespace,
				"status", endpoint.Status)
			continue
		}

		switch endpoint.Type {
		case agaapi.GlobalAcceleratorEndpointTypeService:
			ref := ktypes.NamespacedName{Namespace: endpoint.Namespace, Name: endpoint.Name}
			serviceRefs.Insert(ref.String())

			// Start watching this service if not already watched
			if _, exists := m.serviceWatches[ref]; !exists {
				m.logger.V(1).Info("Starting watch for service", string(ServiceResourceType), ref)
				m.serviceWatches[ref] = m.newResourceWatcher(ref.Namespace, ref.Name, ServiceResourceType)
			}
			m.serviceWatches[ref].AddConsumer(gaID)

		case agaapi.GlobalAcceleratorEndpointTypeIngress:
			ref := ktypes.NamespacedName{Namespace: endpoint.Namespace, Name: endpoint.Name}
			ingressRefs.Insert(ref.String())

			// Start watching this ingress if not already watched
			if _, exists := m.ingressWatches[ref]; !exists {
				m.logger.V(1).Info("Starting watch for ingress", string(IngressResourceType), ref)
				m.ingressWatches[ref] = m.newResourceWatcher(ref.Namespace, ref.Name, IngressResourceType)
			}
			m.ingressWatches[ref].AddConsumer(gaID)

		case agaapi.GlobalAcceleratorEndpointTypeGateway:
			ref := ktypes.NamespacedName{Namespace: endpoint.Namespace, Name: endpoint.Name}
			gatewayRefs.Insert(ref.String())

			// Start watching this gateway if not already watched
			if _, exists := m.gatewayWatches[ref]; !exists {
				m.logger.V(1).Info("Starting watch for gateway", string(GatewayResourceType), ref)
				m.gatewayWatches[ref] = m.newResourceWatcher(ref.Namespace, ref.Name, GatewayResourceType)
			}
			m.gatewayWatches[ref].AddConsumer(gaID)
		}
	}

	// Perform cleanup for resources no longer referenced by this GA
	m.cleanupWatches(m.serviceWatches, serviceRefs, gaID, string(ServiceResourceType))
	m.cleanupWatches(m.ingressWatches, ingressRefs, gaID, string(IngressResourceType))
	m.cleanupWatches(m.gatewayWatches, gatewayRefs, gaID, string(GatewayResourceType))
}

// cleanupWatches removes watches for resources no longer referenced
func (m *defaultEndpointResourcesManager) cleanupWatches(
	watches map[ktypes.NamespacedName]*ResourceWatcher,
	currentRefs sets.String,
	gaID string,
	resourceType string) {

	for ref, watch := range watches {
		if !currentRefs.Has(ref.String()) && watch.HasConsumer(gaID) {
			// This GA no longer references this resource
			watch.RemoveConsumer(gaID)

			// If no GAs reference this resource anymore, stop watching it
			if !watch.HasConsumers() {
				m.logger.V(1).Info("Stopping watch for resource",
					"type", resourceType, "resource", ref)
				watch.Stop()
				delete(watches, ref)
			}
		}
	}
}

// RemoveGA removes all watches for resources referenced by a GA being deleted
func (m *defaultEndpointResourcesManager) RemoveGA(gaKey ktypes.NamespacedName) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	gaID := gaKey.String()

	// Remove from all watch types
	m.removeGAFromWatches(m.serviceWatches, gaID, string(ServiceResourceType))
	m.removeGAFromWatches(m.ingressWatches, gaID, string(IngressResourceType))
	m.removeGAFromWatches(m.gatewayWatches, gaID, string(GatewayResourceType))
}

// removeGAFromWatches removes a GA from the consumers of all watches
func (m *defaultEndpointResourcesManager) removeGAFromWatches(
	watches map[ktypes.NamespacedName]*ResourceWatcher,
	gaID string,
	resourceType string) {

	for ref, watch := range watches {
		if watch.HasConsumer(gaID) {
			watch.RemoveConsumer(gaID)

			// If no GAs reference this resource anymore, stop watching it
			if !watch.HasConsumers() {
				m.logger.V(1).Info("Stopping watch for resource",
					"type", resourceType, "resource", ref)
				watch.Stop()
				delete(watches, ref)
			}
		}
	}
}

// newResourceWatcher creates a new ResourceWatcher for a specific resource type
func (m *defaultEndpointResourcesManager) newResourceWatcher(namespace, name string, resourceType ResourceType) *ResourceWatcher {
	var store cache.Store
	var resourceClient ResourceClient
	var exampleObject client.Object

	switch resourceType {
	case ServiceResourceType:
		store = m.newServiceStore()
		resourceClient = NewServiceClient(m.clientSet, namespace)
		exampleObject = ExampleService
	case IngressResourceType:
		store = m.newIngressStore()
		resourceClient = NewIngressClient(m.clientSet, namespace)
		exampleObject = ExampleIngress
	case GatewayResourceType:
		store = m.newGatewayStore()
		resourceClient = NewGatewayClient(m.gatewayClient, namespace)
		exampleObject = ExampleGateway
	default:
		panic(fmt.Sprintf("Unknown resource type: %s", resourceType))
	}

	return NewResourceWatcher(namespace, name, resourceClient, store, exampleObject)
}

// newServiceStore creates a new store for services
func (m *defaultEndpointResourcesManager) newServiceStore() *ResourceStore[*corev1.Service] {
	return NewResourceStore[*corev1.Service](m.serviceEventChan, cache.MetaNamespaceKeyFunc, m.logger)
}

// newIngressStore creates a new store for ingresses
func (m *defaultEndpointResourcesManager) newIngressStore() *ResourceStore[*networkingv1.Ingress] {
	return NewResourceStore[*networkingv1.Ingress](m.ingressEventChan, cache.MetaNamespaceKeyFunc, m.logger)
}

// newGatewayStore creates a new store for gateways
func (m *defaultEndpointResourcesManager) newGatewayStore() *ResourceStore[*gwv1.Gateway] {
	return NewResourceStore[*gwv1.Gateway](m.gatewayEventChan, cache.MetaNamespaceKeyFunc, m.logger)
}

// HasGatewaySupport checks if Gateway API client is initialized and CRDs are available
func (m *defaultEndpointResourcesManager) HasGatewaySupport() bool {
	// Check if Gateway API client is initialized
	if m.gatewayClient == nil {
		return false
	}

	// Try to access the Gateway API discovery to confirm CRDs are installed
	// We don't need to actually fetch the resources, just check if the API is accessible
	_, err := m.gatewayClient.GatewayV1().Gateways("").List(context.Background(), metav1.ListOptions{Limit: 1})
	if err != nil {
		m.logger.Info("Gateway API CRDs are not available", "error", err)
		return false
	}

	return true
}
