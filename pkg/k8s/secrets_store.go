package k8s

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// NewSecretsStore constructs new conversionStore
func NewSecretsStore(secretsEventChan chan<- event.GenericEvent, keyFunc cache.KeyFunc, logger logr.Logger) *SecretsStore {
	return &SecretsStore{
		secretsEventChan: secretsEventChan,
		logger:           logger,
		store:            cache.NewStore(keyFunc),
	}
}

var _ cache.Store = &SecretsStore{}

// SecretsStore implements cache.Store.
// It invokes the eventhandler for Add, Update, Delete events
type SecretsStore struct {
	store            cache.Store
	secretsEventChan chan<- event.GenericEvent
	logger           logr.Logger
}

// Add adds the given object to the accumulator associated with the given object's key
func (s *SecretsStore) Add(obj interface{}) error {
	if err := s.store.Add(obj); err != nil {
		return err
	}
	s.logger.V(1).Info("secret created, notifying event handler", "resource", obj)
	s.secretsEventChan <- event.GenericEvent{
		Object: obj.(*corev1.Secret),
	}
	return nil
}

// Update updates the given object in the accumulator associated with the given object's key
func (s *SecretsStore) Update(obj interface{}) error {
	if err := s.store.Update(obj); err != nil {
		return err
	}
	s.logger.V(1).Info("secret updated, notifying event handler", "resource", obj)
	s.secretsEventChan <- event.GenericEvent{
		Object: obj.(*corev1.Secret),
	}
	return nil
}

// Delete deletes the given object from the accumulator associated with the given object's key
func (s *SecretsStore) Delete(obj interface{}) error {
	if err := s.store.Delete(obj); err != nil {
		return err
	}
	s.logger.V(1).Info("secret deleted, notifying event handler", "resource", obj)
	s.secretsEventChan <- event.GenericEvent{
		Object: obj.(*corev1.Secret),
	}
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
