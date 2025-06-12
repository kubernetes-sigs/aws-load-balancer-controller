package referencecounter

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sync"
)

// ServiceReferenceCounter tracks gateways and their relations to service objects.
type ServiceReferenceCounter interface {
	UpdateRelations(svcs []types.NamespacedName, gateway types.NamespacedName, isDelete bool)
	IsEligibleForRemoval(svcName types.NamespacedName, expectedGateways []types.NamespacedName) bool
}

type serviceReferenceCounter struct {
	mutex sync.RWMutex
	// key: gateway, value: set of svc
	relations map[types.NamespacedName]sets.Set[types.NamespacedName]
	refCount  map[types.NamespacedName]int
}

// UpdateRelations updates the Gateway -> Service. mapping.
func (t *serviceReferenceCounter) UpdateRelations(svcs []types.NamespacedName, gateway types.NamespacedName, isDelete bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	existingValues, exists := t.relations[gateway]

	// Remove the old values from the ref count, we may add them back later.
	if exists {
		t.updateRefCount(existingValues, true)
	}

	// If this a delete, we just need to remove the deleted gateway from the relations map.
	if isDelete {
		if exists {
			delete(t.relations, gateway)
		}
		return
	}

	// On additions, we simply create a new set, save the gateway -> relations set, and update the ref count map.
	svcsSet := sets.New(svcs...)

	t.relations[gateway] = svcsSet
	t.updateRefCount(svcsSet, false)
}

// updateRefCount updates the ref count field, so consumers don't have to calculate ref counts on each IsEligibleForRemoval
func (t *serviceReferenceCounter) updateRefCount(svcs sets.Set[types.NamespacedName], isRemove bool) {
	modifier := 1
	if isRemove {
		modifier = -1
	}
	for _, svc := range svcs.UnsortedList() {
		t.refCount[svc] += modifier

		if t.refCount[svc] == 0 {
			delete(t.refCount, svc)
		}
	}
}

// IsEligibleForRemoval determines if it is safe to remove to no longer track resources that care about a particular service.
func (t *serviceReferenceCounter) IsEligibleForRemoval(svcName types.NamespacedName, expectedGateways []types.NamespacedName) bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	_, ok := t.refCount[svcName]
	// If we have a ref count for this service, we can't remove it.
	// updateRefCount should always remove 0 entry services.
	if ok {
		return false
	}

	// Next we check if the Gateway cache is correctly populated. This prevents premature removal of items
	// when the cache is not warm.
	for _, gw := range expectedGateways {
		if _, exists := t.relations[gw]; !exists {
			return false
		}
	}
	return true
}

func NewServiceReferenceCounter() ServiceReferenceCounter {
	return &serviceReferenceCounter{
		relations: make(map[types.NamespacedName]sets.Set[types.NamespacedName]),
		refCount:  make(map[types.NamespacedName]int),
	}
}
