package aga

import (
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	fakegwclientset "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

// MockEventChannel represents an event channel for testing
type MockEventChannel struct {
	Events []event.GenericEvent
	mu     sync.Mutex
}

func NewMockEventChannel() *MockEventChannel {
	return &MockEventChannel{
		Events: make([]event.GenericEvent, 0),
	}
}

func (m *MockEventChannel) Send(e event.GenericEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Events = append(m.Events, e)
}

func (m *MockEventChannel) Channel() chan<- event.GenericEvent {
	ch := make(chan event.GenericEvent, 10)
	go func() {
		for e := range ch {
			m.Send(e)
		}
	}()
	return ch
}

func TestMonitorEndpointResourcesAndRemoveGA(t *testing.T) {
	// Create test dependencies
	clientSet := fake.NewSimpleClientset()
	gwClient := fakegwclientset.NewSimpleClientset()

	// Use our mock event channels to capture events
	serviceEventChannel := NewMockEventChannel()
	ingressEventChannel := NewMockEventChannel()
	gatewayEventChannel := NewMockEventChannel()

	logger := logr.Discard()

	// Create the manager
	manager := NewEndpointResourcesManager(
		clientSet,
		gwClient,
		serviceEventChannel.Channel(),
		ingressEventChannel.Channel(),
		gatewayEventChannel.Channel(),
		logger,
	)

	// Create a GlobalAccelerator with endpoints
	ga := &agaapi.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ga",
			Namespace: "default",
		},
	}

	// Create loaded endpoints
	svcName := "test-service"
	svcNamespace := "default"
	endpoints := []*LoadedEndpoint{
		{
			Type:      agaapi.GlobalAcceleratorEndpointTypeService,
			Name:      svcName,
			Namespace: svcNamespace,
			EndpointRef: &agaapi.GlobalAcceleratorEndpoint{
				Type: agaapi.GlobalAcceleratorEndpointTypeService,
				Name: &svcName,
			},
			Status: EndpointStatusLoaded,
			ARN:    "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/test",
		},
	}

	// Call MonitorEndpointResources
	manager.MonitorEndpointResources(ga, endpoints)

	// Get the internal service watches map to verify
	defaultManager, ok := manager.(*defaultEndpointResourcesManager)
	assert.True(t, ok, "Manager should be a defaultEndpointResourcesManager")

	// Verify watch was created
	resourceKey := ktypes.NamespacedName{Namespace: svcNamespace, Name: svcName}
	assert.Contains(t, defaultManager.serviceWatches, resourceKey, "Service watch should be created")

	// Call RemoveGA to remove the GA
	gaKey := ktypes.NamespacedName{Namespace: "default", Name: "test-ga"}
	manager.RemoveGA(gaKey)

	// Verify watch was removed
	assert.NotContains(t, defaultManager.serviceWatches, resourceKey, "Service watch should be removed")
}

// We create a separate test for multiple consumers since we need to verify the watch isn't removed until all consumers are gone
func TestMultipleConsumers(t *testing.T) {
	// Create test dependencies
	clientSet := fake.NewSimpleClientset()
	gwClient := fakegwclientset.NewSimpleClientset()

	// Use our mock event channels
	serviceEventChannel := NewMockEventChannel()
	ingressEventChannel := NewMockEventChannel()
	gatewayEventChannel := NewMockEventChannel()

	logger := logr.Discard()

	// Create the manager
	manager := NewEndpointResourcesManager(
		clientSet,
		gwClient,
		serviceEventChannel.Channel(),
		ingressEventChannel.Channel(),
		gatewayEventChannel.Channel(),
		logger,
	)

	// Create two GlobalAccelerators with endpoints to the same Service
	ga1 := &agaapi.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ga-1",
			Namespace: "default",
		},
	}

	ga2 := &agaapi.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ga-2",
			Namespace: "default",
		},
	}

	// Create loaded endpoints to the same service
	svcName := "test-service"
	svcNamespace := "default"
	endpoints := []*LoadedEndpoint{
		{
			Type:      agaapi.GlobalAcceleratorEndpointTypeService,
			Name:      svcName,
			Namespace: svcNamespace,
			EndpointRef: &agaapi.GlobalAcceleratorEndpoint{
				Type: agaapi.GlobalAcceleratorEndpointTypeService,
				Name: &svcName,
			},
			Status: EndpointStatusLoaded,
			ARN:    "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/test",
		},
	}

	// Add both GAs to monitor the same service
	manager.MonitorEndpointResources(ga1, endpoints)
	manager.MonitorEndpointResources(ga2, endpoints)

	defaultManager, _ := manager.(*defaultEndpointResourcesManager)
	resourceKey := ktypes.NamespacedName{Namespace: svcNamespace, Name: svcName}

	// Get the watcher to verify it has both consumers
	watcher := defaultManager.serviceWatches[resourceKey]
	assert.True(t, watcher.HasConsumer("default/test-ga-1"), "Watcher should have GA1 as consumer")
	assert.True(t, watcher.HasConsumer("default/test-ga-2"), "Watcher should have GA2 as consumer")

	// Remove first GA
	gaKey1 := ktypes.NamespacedName{Namespace: "default", Name: "test-ga-1"}
	manager.RemoveGA(gaKey1)

	// Verify watcher still exists after removing first GA
	assert.Contains(t, defaultManager.serviceWatches, resourceKey, "Service watch should still exist")
	assert.False(t, watcher.HasConsumer("default/test-ga-1"), "Watcher should not have GA1 as consumer anymore")
	assert.True(t, watcher.HasConsumer("default/test-ga-2"), "Watcher should still have GA2 as consumer")

	// Remove second GA
	gaKey2 := ktypes.NamespacedName{Namespace: "default", Name: "test-ga-2"}
	manager.RemoveGA(gaKey2)

	// Verify watcher is removed after removing all consumers
	assert.NotContains(t, defaultManager.serviceWatches, resourceKey, "Service watch should be removed")
}

func TestCrossNamespaceReferences(t *testing.T) {
	t.Run("cross-namespace reference not allowed", func(t *testing.T) {
		// Create test dependencies
		clientSet := fake.NewSimpleClientset()
		gwClient := fakegwclientset.NewSimpleClientset()

		// Use our mock event channels
		serviceEventChannel := NewMockEventChannel()
		ingressEventChannel := NewMockEventChannel()
		gatewayEventChannel := NewMockEventChannel()

		logger := logr.Discard()

		// Create the manager
		manager := NewEndpointResourcesManager(
			clientSet,
			gwClient,
			serviceEventChannel.Channel(),
			ingressEventChannel.Channel(),
			gatewayEventChannel.Channel(),
			logger,
		)

		// Create a GlobalAccelerator with cross-namespace endpoint
		ga := &agaapi.GlobalAccelerator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ga",
				Namespace: "default",
			},
		}

		// Create loaded endpoint to a service in another namespace
		svcName := "cross-ns-service"
		svcNamespace := "other-namespace" // Different from GA's namespace
		endpoints := []*LoadedEndpoint{
			{
				Type:      agaapi.GlobalAcceleratorEndpointTypeService,
				Name:      svcName,
				Namespace: svcNamespace,
				EndpointRef: &agaapi.GlobalAcceleratorEndpoint{
					Type: agaapi.GlobalAcceleratorEndpointTypeService,
					Name: &svcName,
				},
				Status: EndpointStatusLoaded,
				ARN:    "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/test",
				// Important: Set CrossNamespaceAllowed to false for this test
				CrossNamespaceAllowed: false,
			},
		}

		// Monitor the cross-namespace endpoint
		manager.MonitorEndpointResources(ga, endpoints)

	// Verify no watches were created since cross-namespace references should be skipped
	defaultManager, _ := manager.(*defaultEndpointResourcesManager)
	resourceKey := ktypes.NamespacedName{Namespace: svcNamespace, Name: svcName}
	assert.NotContains(t, defaultManager.serviceWatches, resourceKey, "Cross-namespace service watch should be skipped")
}
