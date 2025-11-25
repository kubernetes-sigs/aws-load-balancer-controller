package aga

import (
	"context"
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceWatcher is a generic implementation for watching Kubernetes resources
type ResourceWatcher struct {
	store     cache.Store
	reflector *cache.Reflector
	consumers sets.String // Set of GA names that reference this resource
	stopCh    chan struct{}
	mutex     sync.RWMutex // Protects consumers from concurrent access
}

// ResourceClient is an interface for common operations on a resource
type ResourceClient interface {
	List(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

// NewResourceWatcher creates a new ResourceWatcher for a specific resource
func NewResourceWatcher(
	namespace, name string,
	resourceClient ResourceClient,
	store cache.Store,
	exampleObject client.Object,
) *ResourceWatcher {
	fieldSelector := fields.Set{"metadata.name": name}.AsSelector().String()

	listFunc := func(options metav1.ListOptions) (runtime.Object, error) {
		options.FieldSelector = fieldSelector
		return resourceClient.List(context.Background(), options)
	}

	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		options.FieldSelector = fieldSelector
		return resourceClient.Watch(context.Background(), options)
	}

	rt := cache.NewNamedReflector(
		fmt.Sprintf("%T-%s/%s", exampleObject, namespace, name),
		&cache.ListWatch{ListFunc: listFunc, WatchFunc: watchFunc},
		exampleObject,
		store,
		0,
	)

	watcher := &ResourceWatcher{
		store:     store,
		reflector: rt,
		consumers: sets.NewString(),
		stopCh:    make(chan struct{}),
	}

	go watcher.Start()
	return watcher
}

// Start runs the reflector
func (w *ResourceWatcher) Start() {
	w.reflector.Run(w.stopCh)
}

// Stop stops the reflector
func (w *ResourceWatcher) Stop() {
	close(w.stopCh)
}

// AddConsumer adds a consumer (GlobalAccelerator) to the watcher
func (w *ResourceWatcher) AddConsumer(consumerID string) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.consumers.Insert(consumerID)
}

// RemoveConsumer removes a consumer from the watcher
func (w *ResourceWatcher) RemoveConsumer(consumerID string) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.consumers.Delete(consumerID)
}

// HasConsumers checks if the watcher has any consumers
func (w *ResourceWatcher) HasConsumers() bool {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	return w.consumers.Len() > 0
}

// HasConsumer checks if the watcher has a specific consumer
func (w *ResourceWatcher) HasConsumer(consumerID string) bool {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	return w.consumers.Has(consumerID)
}
