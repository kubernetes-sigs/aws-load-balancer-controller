package aga

import (
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// ResourceStore is a generic implementation of cache.Store for Kubernetes resources
type ResourceStore[T client.Object] struct {
	store     cache.Store
	eventChan chan<- event.GenericEvent
	logger    logr.Logger
}

// NewResourceStore creates a new ResourceStore for a specific resource type
func NewResourceStore[T client.Object](eventChan chan<- event.GenericEvent, keyFunc cache.KeyFunc, logger logr.Logger) *ResourceStore[T] {
	return &ResourceStore[T]{
		store:     cache.NewStore(keyFunc),
		eventChan: eventChan,
		logger:    logger,
	}
}

var _ cache.Store = &ResourceStore[client.Object]{}

// Add adds the given object to the store
func (s *ResourceStore[T]) Add(obj interface{}) error {
	if err := s.store.Add(obj); err != nil {
		return err
	}
	s.logger.V(1).Info("Resource created or updated", "resource", obj)
	s.eventChan <- event.GenericEvent{
		Object: obj.(T),
	}
	return nil
}

// Update updates the given object in the store
func (s *ResourceStore[T]) Update(obj interface{}) error {
	if err := s.store.Update(obj); err != nil {
		return err
	}
	s.logger.V(1).Info("Resource updated", "resource", obj)
	s.eventChan <- event.GenericEvent{
		Object: obj.(T),
	}
	return nil
}

// Delete deletes the given object from the store
func (s *ResourceStore[T]) Delete(obj interface{}) error {
	if err := s.store.Delete(obj); err != nil {
		return err
	}
	s.logger.V(1).Info("Resource deleted", "resource", obj)
	s.eventChan <- event.GenericEvent{
		Object: obj.(T),
	}
	return nil
}

// Replace will delete the contents of the store, using instead the given list
func (s *ResourceStore[T]) Replace(list []interface{}, resourceVersion string) error {
	return s.store.Replace(list, resourceVersion)
}

// Resync is meaningless in the terms appearing here
func (s *ResourceStore[T]) Resync() error {
	return s.store.Resync()
}

// List returns a list of all the currently non-empty accumulators
func (s *ResourceStore[T]) List() []interface{} {
	return s.store.List()
}

// ListKeys returns a list of all the keys currently associated with non-empty accumulators
func (s *ResourceStore[T]) ListKeys() []string {
	return s.store.ListKeys()
}

// Get returns the accumulator associated with the given object's key
func (s *ResourceStore[T]) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return s.store.Get(obj)
}

// GetByKey returns the accumulator associated with the given key
func (s *ResourceStore[T]) GetByKey(key string) (item interface{}, exists bool, err error) {
	return s.store.GetByKey(key)
}
