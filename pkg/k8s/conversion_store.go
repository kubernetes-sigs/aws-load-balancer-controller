package k8s

import (
	"k8s.io/client-go/tools/cache"
)

type ConversionFunc func(obj interface{}) (interface{}, error)

// NewConversionStore constructs new conversionStore
func NewConversionStore(conversionFunc ConversionFunc, keyFunc cache.KeyFunc) *ConversionStore {
	return &ConversionStore{
		conversionFunc: conversionFunc,
		store:          cache.NewStore(keyFunc),
	}
}

var _ cache.Store = &ConversionStore{}

// ConversionStore implements cache.Store.
// It converts objects to another type using specified conversionFunc.
// In store's term, the accumulator for this store is a convert that converts object's latest state into another object.
type ConversionStore struct {
	conversionFunc ConversionFunc
	store          cache.Store
}

// Add adds the given object to the accumulator associated with the given object's key
func (s *ConversionStore) Add(obj interface{}) error {
	converted, err := s.conversionFunc(obj)
	if err != nil {
		return err
	}
	return s.store.Add(converted)
}

// Update updates the given object in the accumulator associated with the given object's key
func (s *ConversionStore) Update(obj interface{}) error {
	converted, err := s.conversionFunc(obj)
	if err != nil {
		return err
	}
	return s.store.Update(converted)
}

// Delete deletes the given object from the accumulator associated with the given object's key
func (s *ConversionStore) Delete(obj interface{}) error {
	converted, err := s.conversionFunc(obj)
	if err != nil {
		return err
	}
	return s.store.Delete(converted)
}

// Replace will delete the contents of the store, using instead the given list.
func (s *ConversionStore) Replace(list []interface{}, resourceVersion string) error {
	items := make([]interface{}, 0, len(list))
	for _, item := range list {
		converted, err := s.conversionFunc(item)
		if err != nil {
			return err
		}
		items = append(items, converted)
	}
	return s.store.Replace(items, resourceVersion)
}

// Resync is meaningless in the terms appearing here
func (s *ConversionStore) Resync() error {
	return s.store.Resync()
}

// List returns a list of all the currently non-empty accumulators
func (s *ConversionStore) List() []interface{} {
	return s.store.List()
}

// ListKeys returns a list of all the keys currently associated with non-empty accumulators
func (s *ConversionStore) ListKeys() []string {
	return s.store.ListKeys()
}

// Get returns the accumulator associated with the given object's key
func (s *ConversionStore) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return s.store.Get(obj)
}

// GetByKey returns the accumulator associated with the given key
func (s *ConversionStore) GetByKey(key string) (item interface{}, exists bool, err error) {
	return s.store.GetByKey(key)
}
