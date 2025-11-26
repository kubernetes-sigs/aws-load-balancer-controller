package aga

import (
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
)

// ResourceKey uniquely identifies a resource by its type and name
type ResourceKey struct {
	Type ResourceType
	Name types.NamespacedName
}

// ReferenceTracker tracks which resources are referenced by which GlobalAccelerators
type ReferenceTracker struct {
	mutex       sync.RWMutex
	resourceMap map[ResourceKey]sets.String                    // Resource -> Set of GA names
	gaRefMap    map[types.NamespacedName]sets.Set[ResourceKey] // GA -> Set of resources
	logger      logr.Logger
}

// NewReferenceTracker creates a new ReferenceTracker
func NewReferenceTracker(logger logr.Logger) *ReferenceTracker {
	return &ReferenceTracker{
		resourceMap: make(map[ResourceKey]sets.String),
		gaRefMap:    make(map[types.NamespacedName]sets.Set[ResourceKey]),
		logger:      logger,
	}
}

// UpdateDesiredEndpointReferencesForGA updates the tracking information for a GlobalAccelerator
func (t *ReferenceTracker) UpdateDesiredEndpointReferencesForGA(ga *agaapi.GlobalAccelerator, desiredEndpoints []EndpointReference) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	gaKey := k8s.NamespacedName(ga)

	// Track current resources referenced by this GA
	currentResources := sets.New[ResourceKey]()

	// Process each endpoint
	for _, endpoint := range desiredEndpoints {
		resourceKey := endpoint.ToResourceKey()

		currentResources.Insert(resourceKey)

		// Update resource -> GA mapping
		if _, exists := t.resourceMap[resourceKey]; !exists {
			t.resourceMap[resourceKey] = sets.NewString()
		}
		t.resourceMap[resourceKey].Insert(gaKey.String())

		t.logger.V(1).Info("Resource referenced by GA",
			"ga", gaKey.String(),
			"resourceType", resourceKey.Type,
			"resourceName", resourceKey.Name)
	}

	// Remove old references
	if oldResources, exists := t.gaRefMap[gaKey]; exists {
		for resourceKey := range oldResources {
			if !currentResources.Has(resourceKey) {
				// Resource no longer referenced by this GA
				if gaSet, exists := t.resourceMap[resourceKey]; exists {
					gaSet.Delete(gaKey.String())
					if gaSet.Len() == 0 {
						delete(t.resourceMap, resourceKey)
						t.logger.V(1).Info("Resource no longer referenced by any GA",
							"resourceType", resourceKey.Type,
							"resourceName", resourceKey.Name)
					}
				}
			}
		}
	}

	// Update GA -> resources mapping
	t.gaRefMap[gaKey] = currentResources
}

// RemoveGA removes all tracking information for a GlobalAccelerator
func (t *ReferenceTracker) RemoveGA(gaKey types.NamespacedName) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if resources, exists := t.gaRefMap[gaKey]; exists {
		for resourceKey := range resources {
			if gaSet, exists := t.resourceMap[resourceKey]; exists {
				gaSet.Delete(gaKey.String())
				if gaSet.Len() == 0 {
					delete(t.resourceMap, resourceKey)
					t.logger.V(1).Info("Resource no longer referenced by any GA",
						"resourceType", resourceKey.Type,
						"resourceName", resourceKey.Name)
				}
			}
		}

		delete(t.gaRefMap, gaKey)
	}
}

// IsResourceReferenced checks if a resource is referenced by any GlobalAccelerator
func (t *ReferenceTracker) IsResourceReferenced(resourceKey ResourceKey) bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	gaSet, exists := t.resourceMap[resourceKey]
	return exists && gaSet.Len() > 0
}

// GetGAsForResource returns all GlobalAccelerators that reference a resource
func (t *ReferenceTracker) GetGAsForResource(resourceKey ResourceKey) []types.NamespacedName {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	var result []types.NamespacedName

	if gaSet, exists := t.resourceMap[resourceKey]; exists {
		for gaStr := range gaSet {
			parts := strings.Split(gaStr, "/")
			if len(parts) == 2 {
				result = append(result, types.NamespacedName{
					Namespace: parts[0],
					Name:      parts[1],
				})
			}
		}
	}

	return result
}
