package aga

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
)

func TestNewReferenceTracker(t *testing.T) {
	// Test creating a new reference tracker
	logger := logr.Discard()
	tracker := NewReferenceTracker(logger)

	// Verify that the tracker is initialized properly
	assert.NotNil(t, tracker)
	assert.NotNil(t, tracker.resourceMap)
	assert.NotNil(t, tracker.gaRefMap)
	assert.Equal(t, 0, len(tracker.resourceMap))
	assert.Equal(t, 0, len(tracker.gaRefMap))
}

func TestReferenceTracker_UpdateReferencesForGA(t *testing.T) {
	// Helper function to create a string pointer
	strPtr := func(s string) *string {
		return &s
	}

	// Test cases
	tests := []struct {
		name               string
		ga                 *agaapi.GlobalAccelerator
		expectedResources  int
		expectedReferences map[ResourceKey][]string
	}{
		{
			name: "GA with no endpoints",
			ga: &agaapi.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ga-no-endpoints",
					Namespace: "test-ns",
				},
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{},
				},
			},
			expectedResources:  0,
			expectedReferences: map[ResourceKey][]string{},
		},
		{
			name: "GA with service endpoints",
			ga: &agaapi.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ga-service-endpoints",
					Namespace: "test-ns",
				},
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{
											Type: agaapi.GlobalAcceleratorEndpointTypeService,
											Name: strPtr("service1"),
										},
										{
											Type: agaapi.GlobalAcceleratorEndpointTypeService,
											Name: strPtr("service2"),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResources: 2,
			expectedReferences: map[ResourceKey][]string{
				{
					Type: ServiceResourceType,
					Name: types.NamespacedName{Namespace: "test-ns", Name: "service1"},
				}: {"test-ns/ga-service-endpoints"},
				{
					Type: ServiceResourceType,
					Name: types.NamespacedName{Namespace: "test-ns", Name: "service2"},
				}: {"test-ns/ga-service-endpoints"},
			},
		},
		{
			name: "GA with mixed endpoints",
			ga: &agaapi.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ga-mixed-endpoints",
					Namespace: "test-ns",
				},
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{
											Type: agaapi.GlobalAcceleratorEndpointTypeService,
											Name: strPtr("service1"),
										},
										{
											Type:      agaapi.GlobalAcceleratorEndpointTypeIngress,
											Name:      strPtr("ingress1"),
											Namespace: strPtr("other-ns"),
										},
										{
											Type:       agaapi.GlobalAcceleratorEndpointTypeEndpointID,
											EndpointID: strPtr("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test/1234567890"),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResources: 3,
			expectedReferences: map[ResourceKey][]string{
				{
					Type: ServiceResourceType,
					Name: types.NamespacedName{Namespace: "test-ns", Name: "service1"},
				}: {"test-ns/ga-mixed-endpoints"},
				{
					Type: IngressResourceType,
					Name: types.NamespacedName{Namespace: "other-ns", Name: "ingress1"},
				}: {"test-ns/ga-mixed-endpoints"},
				{
					Type: ResourceType(agaapi.GlobalAcceleratorEndpointTypeEndpointID),
					Name: types.NamespacedName{Namespace: "", Name: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test/1234567890"},
				}: {"test-ns/ga-mixed-endpoints"},
			},
		},
	}

	// Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create tracker
			tracker := NewReferenceTracker(logr.Discard())

			endpoints := GetAllEndpointsFromGA(tt.ga)
			// Update references
			tracker.UpdateReferencesForGA(tt.ga, endpoints)

			// Check number of tracked resources
			gaKey := types.NamespacedName{Namespace: tt.ga.Namespace, Name: tt.ga.Name}
			resources, exists := tracker.gaRefMap[gaKey]
			if tt.expectedResources == 0 {
				assert.Equal(t, tt.expectedResources, len(resources))
			} else {
				assert.True(t, exists)
				assert.Equal(t, tt.expectedResources, len(resources))
			}

			// Check resource references
			for resourceKey, expectedGAs := range tt.expectedReferences {
				gaSet, exists := tracker.resourceMap[resourceKey]
				assert.True(t, exists)
				assert.Equal(t, len(expectedGAs), gaSet.Len())

				for _, expectedGA := range expectedGAs {
					assert.True(t, gaSet.Has(expectedGA))
				}
			}
		})
	}
}

func TestReferenceTracker_UpdateReferencesForGA_RemoveStaleReferences(t *testing.T) {
	// Helper function to create a string pointer
	strPtr := func(s string) *string {
		return &s
	}

	// Create GA with initial endpoints
	ga := &agaapi.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ga-test",
			Namespace: "test-ns",
		},
		Spec: agaapi.GlobalAcceleratorSpec{
			Listeners: &[]agaapi.GlobalAcceleratorListener{
				{
					EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
						{
							Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
								{
									Type: agaapi.GlobalAcceleratorEndpointTypeService,
									Name: strPtr("service1"),
								},
								{
									Type: agaapi.GlobalAcceleratorEndpointTypeService,
									Name: strPtr("service2"),
								},
							},
						},
					},
				},
			},
		},
	}

	// Create tracker and add initial references
	tracker := NewReferenceTracker(logr.Discard())

	endpoints := GetAllEndpointsFromGA(ga)
	tracker.UpdateReferencesForGA(ga, endpoints)

	// Verify initial state
	service1Key := ResourceKey{
		Type: ServiceResourceType,
		Name: types.NamespacedName{Namespace: "test-ns", Name: "service1"},
	}
	service2Key := ResourceKey{
		Type: ServiceResourceType,
		Name: types.NamespacedName{Namespace: "test-ns", Name: "service2"},
	}
	service3Key := ResourceKey{
		Type: ServiceResourceType,
		Name: types.NamespacedName{Namespace: "test-ns", Name: "service3"},
	}

	// Both services should be referenced
	assert.True(t, tracker.IsResourceReferenced(service1Key))
	assert.True(t, tracker.IsResourceReferenced(service2Key))

	// Now modify the GA to remove service2 and add service3
	ga.Spec = agaapi.GlobalAcceleratorSpec{
		Listeners: &[]agaapi.GlobalAcceleratorListener{
			{
				EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
					{
						Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
							{
								Type: agaapi.GlobalAcceleratorEndpointTypeService,
								Name: strPtr("service1"),
							},
							{
								Type: agaapi.GlobalAcceleratorEndpointTypeService,
								Name: strPtr("service3"),
							},
						},
					},
				},
			},
		},
	}

	// Update references with modified GA
	endpoints = GetAllEndpointsFromGA(ga)
	tracker.UpdateReferencesForGA(ga, endpoints)

	// Verify that service1 is still referenced, service2 is no longer referenced, and service3 is now referenced
	assert.True(t, tracker.IsResourceReferenced(service1Key))
	assert.False(t, tracker.IsResourceReferenced(service2Key))
	assert.True(t, tracker.IsResourceReferenced(service3Key))
}

func TestReferenceTracker_RemoveGA(t *testing.T) {
	// Helper function to create a string pointer
	strPtr := func(s string) *string {
		return &s
	}

	// Create two GAs with overlapping references
	ga1 := &agaapi.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ga1",
			Namespace: "test-ns",
		},
		Spec: agaapi.GlobalAcceleratorSpec{
			Listeners: &[]agaapi.GlobalAcceleratorListener{
				{
					EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
						{
							Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
								{
									Type: agaapi.GlobalAcceleratorEndpointTypeService,
									Name: strPtr("service1"),
								},
								{
									Type: agaapi.GlobalAcceleratorEndpointTypeService,
									Name: strPtr("service2"),
								},
							},
						},
					},
				},
			},
		},
	}

	ga2 := &agaapi.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ga2",
			Namespace: "test-ns",
		},
		Spec: agaapi.GlobalAcceleratorSpec{
			Listeners: &[]agaapi.GlobalAcceleratorListener{
				{
					EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
						{
							Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
								{
									Type: agaapi.GlobalAcceleratorEndpointTypeService,
									Name: strPtr("service2"),
								},
								{
									Type: agaapi.GlobalAcceleratorEndpointTypeService,
									Name: strPtr("service3"),
								},
							},
						},
					},
				},
			},
		},
	}

	// Create tracker and add references from both GAs
	tracker := NewReferenceTracker(logr.Discard())
	endpoints1 := GetAllEndpointsFromGA(ga1)
	endpoints2 := GetAllEndpointsFromGA(ga2)
	tracker.UpdateReferencesForGA(ga1, endpoints1)
	tracker.UpdateReferencesForGA(ga2, endpoints2)

	// Resource keys
	service1Key := ResourceKey{
		Type: ServiceResourceType,
		Name: types.NamespacedName{Namespace: "test-ns", Name: "service1"},
	}
	service2Key := ResourceKey{
		Type: ServiceResourceType,
		Name: types.NamespacedName{Namespace: "test-ns", Name: "service2"},
	}
	service3Key := ResourceKey{
		Type: ServiceResourceType,
		Name: types.NamespacedName{Namespace: "test-ns", Name: "service3"},
	}

	// Verify initial state - all services should be referenced
	assert.True(t, tracker.IsResourceReferenced(service1Key))
	assert.True(t, tracker.IsResourceReferenced(service2Key))
	assert.True(t, tracker.IsResourceReferenced(service3Key))

	// Remove ga1
	ga1Key := types.NamespacedName{Namespace: "test-ns", Name: "ga1"}
	tracker.RemoveGA(ga1Key)

	// Verify that service1 is no longer referenced, service2 is still referenced by ga2, and service3 is still referenced
	assert.False(t, tracker.IsResourceReferenced(service1Key))
	assert.True(t, tracker.IsResourceReferenced(service2Key))
	assert.True(t, tracker.IsResourceReferenced(service3Key))

	// Remove ga2
	ga2Key := types.NamespacedName{Namespace: "test-ns", Name: "ga2"}
	tracker.RemoveGA(ga2Key)

	// Verify that no services are referenced anymore
	assert.False(t, tracker.IsResourceReferenced(service1Key))
	assert.False(t, tracker.IsResourceReferenced(service2Key))
	assert.False(t, tracker.IsResourceReferenced(service3Key))

	// Verify that gaRefMap is empty
	assert.Equal(t, 0, len(tracker.gaRefMap))
}

func TestReferenceTracker_IsResourceReferenced(t *testing.T) {
	// Helper function to create a string pointer
	strPtr := func(s string) *string {
		return &s
	}

	// Create GA
	ga := &agaapi.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ga-test",
			Namespace: "test-ns",
		},
		Spec: agaapi.GlobalAcceleratorSpec{
			Listeners: &[]agaapi.GlobalAcceleratorListener{
				{
					EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
						{
							Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
								{
									Type: agaapi.GlobalAcceleratorEndpointTypeService,
									Name: strPtr("service1"),
								},
							},
						},
					},
				},
			},
		},
	}

	// Create tracker and add references
	tracker := NewReferenceTracker(logr.Discard())
	endpoints := GetAllEndpointsFromGA(ga)
	tracker.UpdateReferencesForGA(ga, endpoints)

	// Resource keys - one that exists and one that doesn't
	existingResourceKey := ResourceKey{
		Type: ServiceResourceType,
		Name: types.NamespacedName{Namespace: "test-ns", Name: "service1"},
	}
	nonExistingResourceKey := ResourceKey{
		Type: ServiceResourceType,
		Name: types.NamespacedName{Namespace: "test-ns", Name: "non-existing-service"},
	}

	// Test IsResourceReferenced
	assert.True(t, tracker.IsResourceReferenced(existingResourceKey))
	assert.False(t, tracker.IsResourceReferenced(nonExistingResourceKey))
}

func TestReferenceTracker_GetGAsForResource(t *testing.T) {
	// Helper function to create a string pointer
	strPtr := func(s string) *string {
		return &s
	}

	// Create GAs
	ga1 := &agaapi.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ga1",
			Namespace: "test-ns",
		},
		Spec: agaapi.GlobalAcceleratorSpec{
			Listeners: &[]agaapi.GlobalAcceleratorListener{
				{
					EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
						{
							Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
								{
									Type: agaapi.GlobalAcceleratorEndpointTypeService,
									Name: strPtr("shared-service"),
								},
							},
						},
					},
				},
			},
		},
	}

	ga2 := &agaapi.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ga2",
			Namespace: "test-ns",
		},
		Spec: agaapi.GlobalAcceleratorSpec{
			Listeners: &[]agaapi.GlobalAcceleratorListener{
				{
					EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
						{
							Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
								{
									Type: agaapi.GlobalAcceleratorEndpointTypeService,
									Name: strPtr("shared-service"),
								},
							},
						},
					},
				},
			},
		},
	}

	// Create tracker and add references
	tracker := NewReferenceTracker(logr.Discard())
	endpoints1 := GetAllEndpointsFromGA(ga1)
	endpoints2 := GetAllEndpointsFromGA(ga2)
	tracker.UpdateReferencesForGA(ga1, endpoints1)
	tracker.UpdateReferencesForGA(ga2, endpoints2)

	// Resource key for the shared service
	sharedServiceKey := ResourceKey{
		Type: ServiceResourceType,
		Name: types.NamespacedName{Namespace: "test-ns", Name: "shared-service"},
	}

	// Resource key for a non-existing service
	nonExistingServiceKey := ResourceKey{
		Type: ServiceResourceType,
		Name: types.NamespacedName{Namespace: "test-ns", Name: "non-existing-service"},
	}

	// Test GetGAsForResource for shared service
	gasForSharedService := tracker.GetGAsForResource(sharedServiceKey)
	assert.Equal(t, 2, len(gasForSharedService))

	// Verify that both GAs are returned
	ga1Key := types.NamespacedName{Namespace: "test-ns", Name: "ga1"}
	ga2Key := types.NamespacedName{Namespace: "test-ns", Name: "ga2"}

	foundGA1 := false
	foundGA2 := false
	for _, gaKey := range gasForSharedService {
		if gaKey == ga1Key {
			foundGA1 = true
		}
		if gaKey == ga2Key {
			foundGA2 = true
		}
	}
	assert.True(t, foundGA1)
	assert.True(t, foundGA2)

	// Test GetGAsForResource for non-existing service
	gasForNonExistingService := tracker.GetGAsForResource(nonExistingServiceKey)
	assert.Equal(t, 0, len(gasForNonExistingService))
}
