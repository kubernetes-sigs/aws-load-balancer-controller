package k8s

import (
	"context"
	"errors"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"time"
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
// 		* if watchNamespace is "", this repo monitors pods in all namespaces
// 		* if watchNamespace is not "", this repo monitors pods in specific namespace
func NewDefaultPodInfoRepo(getter cache.Getter, watchNamespace string, logger logr.Logger) *defaultPodInfoRepo {
	store := NewConversionStore(podInfoConversionFunc, podInfoKeyFunc)
	lw := cache.NewListWatchFromClient(getter, resourceTypePods, watchNamespace, fields.Everything())
	rt := cache.NewReflector(lw, &corev1.Pod{}, store, 0)

	repo := &defaultPodInfoRepo{
		store:  store,
		rt:     rt,
		logger: logger,
	}
	return repo
}

var _ PodInfoRepo = &defaultPodInfoRepo{}
var _ manager.Runnable = &defaultPodInfoRepo{}

// default implementation for PodInfoRepo
type defaultPodInfoRepo struct {
	store  *ConversionStore
	rt     *cache.Reflector
	logger logr.Logger
}

// Get returns PodInfo specified with specific podKey, and whether it exists.
func (r *defaultPodInfoRepo) Get(_ context.Context, key types.NamespacedName) (PodInfo, bool, error) {
	pInfo := PodInfo{Key: key}
	raw, exists, err := r.store.Get(&pInfo)
	if err != nil {
		return PodInfo{}, false, err
	}
	if !exists {
		return PodInfo{}, false, nil
	}
	pInfo = *raw.(*PodInfo)
	return pInfo, true, nil
}

// ListKeys will list the pod keys in this repo.
func (r *defaultPodInfoRepo) ListKeys(_ context.Context) []types.NamespacedName {
	storeKeys := r.store.ListKeys()
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

// Start will start the repo.
// It leverages ListWatch to keep pod info stored locally to be in-sync with Kubernetes.
func (r *defaultPodInfoRepo) Start(ctx context.Context) error {
	r.rt.Run(ctx.Done())
	return nil
}

// WaitForCacheSync waits for the initial sync of pod information repository.
func (r *defaultPodInfoRepo) WaitForCacheSync(ctx context.Context) error {
	return wait.PollImmediateUntil(waitCacheSyncPollPeriod, func() (bool, error) {
		lastSyncResourceVersion := r.rt.LastSyncResourceVersion()
		return lastSyncResourceVersion != "", nil
	}, ctx.Done())
}

// podInfoKeyFunc computes the store key per PodInfo object.
func podInfoKeyFunc(obj interface{}) (string, error) {
	info, ok := obj.(*PodInfo)
	if !ok {
		return "", errors.New("expect PodInfo object")
	}
	return info.Key.String(), nil
}

// podInfoConversionFunc computes the converted PodInfo per pod object.
func podInfoConversionFunc(obj interface{}) (interface{}, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, errors.New("expect pod object")
	}
	podInfo := buildPodInfo(pod)
	return &podInfo, nil
}
