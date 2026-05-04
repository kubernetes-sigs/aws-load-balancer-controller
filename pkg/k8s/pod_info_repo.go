package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	resourceTypePods        = "pods"
	waitCacheSyncPollPeriod = 2 * time.Second
)

// PodInfoRepo provides access to pod information within cluster.
// We only store necessary pod information related to our controller to reduce memory usage.
type PodInfoRepo interface {
	// Get returns PodInfo specified with specific podKey, and whether it exists.
	Get(ctx context.Context, key types.NamespacedName) (PodInfo, bool, error)

	// ListKeys will list the pod keys in this repo.
	ListKeys(ctx context.Context) []types.NamespacedName
}

// NewDefaultPodInfoRepo constructs new defaultPodInfoRepo.
// * watchNamespace is the namespace to monitor pod spec.
//   - if watchNamespace is "", this repo monitors pods in all namespaces
//   - if watchNamespace is not "", this repo monitors pods in specific namespace
func NewDefaultPodInfoRepo(getter cache.Getter, watchNamespace string, quicServerIDVariableName string, logger logr.Logger) *defaultPodInfoRepo {
	converter := newPodInfoBuilder(quicServerIDVariableName)
	lw := cache.NewListWatchFromClient(getter, resourceTypePods, watchNamespace, fields.Everything())

	informer := cache.NewSharedIndexInformer(lw, &corev1.Pod{}, 0, cache.Indexers{})
	informer.SetTransform(converter.podInfoConverter)
	repo := &defaultPodInfoRepo{
		logger:   logger,
		informer: informer,
	}
	return repo
}

var _ PodInfoRepo = &defaultPodInfoRepo{}
var _ manager.Runnable = &defaultPodInfoRepo{}

// default implementation for PodInfoRepo
type defaultPodInfoRepo struct {
	informer cache.SharedIndexInformer
	logger   logr.Logger
}

func (r *defaultPodInfoRepo) GetInformer() cache.SharedIndexInformer {
	return r.informer
}

// Get returns PodInfo specified with specific podKey, and whether it exists.
func (r *defaultPodInfoRepo) Get(_ context.Context, key types.NamespacedName) (PodInfo, bool, error) {
	raw, exists, err := r.informer.GetStore().Get(cache.ExplicitKey(key.String()))
	if err != nil {
		return PodInfo{}, false, err
	}
	if !exists {
		return PodInfo{}, false, nil
	}
	pInfo, ok := raw.(*PodInfo)
	if !ok {
		return PodInfo{}, false, fmt.Errorf("expect *PodInfo object got %T", raw)
	}
	return *pInfo, true, nil
}

// ListKeys will list the pod keys in this repo.
func (r *defaultPodInfoRepo) ListKeys(_ context.Context) []types.NamespacedName {
	storeKeys := r.informer.GetStore().ListKeys()
	keys := make([]types.NamespacedName, 0, len(storeKeys))
	for _, storeKey := range storeKeys {
		namespace, name, _ := cache.SplitMetaNamespaceKey(storeKey)
		key := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}
		keys = append(keys, key)
	}
	return keys
}

// Start will start the informer.
// It leverages ListWatch to keep pod info stored locally to be in-sync with Kubernetes.
func (r *defaultPodInfoRepo) Start(ctx context.Context) error {
	r.informer.RunWithContext(ctx)
	return nil
}

// WaitForCacheSync waits for the initial sync of pod information repository.
func (r *defaultPodInfoRepo) WaitForCacheSync(ctx context.Context) error {
	return wait.PollImmediateUntil(waitCacheSyncPollPeriod, func() (bool, error) {
		lastSyncResourceVersion := r.informer.LastSyncResourceVersion()
		return lastSyncResourceVersion != "", nil
	}, ctx.Done())
}
