package k8s

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// NewSecretsStore constructs new conversionStore
func NewSecretsStore(eventHandler handler.EventHandler, keyFunc cache.KeyFunc, queue workqueue.RateLimitingInterface) *SecretsStore {
	return &SecretsStore{
		eventHandler: eventHandler,
		queue:        queue,
		store:        cache.NewStore(keyFunc),
	}
}

var _ cache.Store = &SecretsStore{}

// SecretsStore implements cache.Store.
// It invokes the eventhandler for Add, Update, Delete events
type SecretsStore struct {
	store        cache.Store
	queue        workqueue.RateLimitingInterface
	eventHandler handler.EventHandler
}

// Add adds the given object to the accumulator associated with the given object's key
func (s *SecretsStore) Add(obj interface{}) error {
	if err := s.store.Add(obj); err != nil {
		return err
	}
	s.eventHandler.Create(event.CreateEvent{Object: obj.(*corev1.Secret)}, s.queue)
	return nil
}

// Update updates the given object in the accumulator associated with the given object's key
func (s *SecretsStore) Update(obj interface{}) error {
	oldObj, exists, err := s.store.Get(obj)
	if err != nil || !exists {
		return err
	}
	if err := s.store.Update(obj); err != nil {
		return err
	}
	updateEvent := event.UpdateEvent{ObjectOld: oldObj.(*corev1.Secret), ObjectNew: obj.(*corev1.Secret)}
	s.eventHandler.Update(updateEvent, s.queue)
	return nil
}

// Delete deletes the given object from the accumulator associated with the given object's key
func (s *SecretsStore) Delete(obj interface{}) error {
	if err := s.store.Delete(obj); err != nil {
		return err
	}
	s.eventHandler.Delete(event.DeleteEvent{Object: obj.(*corev1.Secret)}, s.queue)
	return nil
}

// Replace will delete the contents of the store, using instead the given list.
func (s *SecretsStore) Replace(list []interface{}, resourceVersion string) error {
	return s.store.Replace(list, resourceVersion)
}

// Resync is meaningless in the terms appearing here
func (s *SecretsStore) Resync() error {
	return s.store.Resync()
}

// List returns a list of all the currently non-empty accumulators
func (s *SecretsStore) List() []interface{} {
	return s.store.List()
}

// ListKeys returns a list of all the keys currently associated with non-empty accumulators
func (s *SecretsStore) ListKeys() []string {
	return s.store.ListKeys()
}

// Get returns the accumulator associated with the given object's key
func (s *SecretsStore) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return s.store.Get(obj)
}

// GetByKey returns the accumulator associated with the given key
func (s *SecretsStore) GetByKey(key string) (item interface{}, exists bool, err error) {
	return s.store.GetByKey(key)
}
