package targetgroupbinding

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/backend"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
)

const (
	trackedTargetsPrefix = "aws-lbc-targets-"
	targetsKey           = "targets"
)

// MultiClusterManager implements logic to support multiple LBCs managing the same Target Group.
type MultiClusterManager interface {
	// FilterTargetsForDeregistration Given a list of targets, filter the list down to only targets the cluster should operate on.
	FilterTargetsForDeregistration(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targetInfo []TargetInfo) ([]TargetInfo, bool, error)

	// UpdateTrackedIPTargets Update the tracked target set in persistent storage
	UpdateTrackedIPTargets(ctx context.Context, updateRequested bool, endpoints []backend.PodEndpoint, tgb *elbv2api.TargetGroupBinding) error

	// UpdateTrackedInstanceTargets Update the tracked target set in persistent storage
	UpdateTrackedInstanceTargets(ctx context.Context, updateRequested bool, endpoints []backend.NodePortEndpoint, tgb *elbv2api.TargetGroupBinding) error

	// CleanUp Removes any resources used to implement multicluster support.
	CleanUp(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error
}

type multiClusterManagerImpl struct {
	kubeClient client.Client
	apiReader  client.Reader
	logger     logr.Logger

	configMapCache      map[string]sets.Set[string]
	configMapCacheMutex sync.RWMutex
}

// NewMultiClusterManager constructs a multicluster manager that is immediately ready to use.
func NewMultiClusterManager(kubeClient client.Client, apiReader client.Reader, logger logr.Logger) MultiClusterManager {
	return &multiClusterManagerImpl{
		apiReader:           apiReader,
		kubeClient:          kubeClient,
		logger:              logger,
		configMapCacheMutex: sync.RWMutex{},
		configMapCache:      make(map[string]sets.Set[string]),
	}
}

func (m *multiClusterManagerImpl) UpdateTrackedIPTargets(ctx context.Context, updateRequested bool, endpoints []backend.PodEndpoint, tgb *elbv2api.TargetGroupBinding) error {
	endpointStringFn := func() []string {
		endpointStrings := make([]string, 0, len(endpoints))

		for _, ep := range endpoints {
			endpointStrings = append(endpointStrings, ep.GetIdentifier(false))
		}
		return endpointStrings
	}

	return m.updateTrackedTargets(ctx, updateRequested, endpointStringFn, tgb)
}

func (m *multiClusterManagerImpl) UpdateTrackedInstanceTargets(ctx context.Context, updateRequested bool, endpoints []backend.NodePortEndpoint, tgb *elbv2api.TargetGroupBinding) error {
	endpointStringFn := func() []string {
		endpointStrings := make([]string, 0, len(endpoints))

		for _, ep := range endpoints {
			endpointStrings = append(endpointStrings, ep.GetIdentifier(false))
		}
		return endpointStrings
	}

	return m.updateTrackedTargets(ctx, updateRequested, endpointStringFn, tgb)
}

func (m *multiClusterManagerImpl) updateTrackedTargets(ctx context.Context, updateRequested bool, endpointStringFn func() []string, tgb *elbv2api.TargetGroupBinding) error {
	if !tgb.Spec.MultiClusterTargetGroup {
		return nil
	}

	// Initial case, we want to create the config map when it doesn't exist.
	if !updateRequested {
		cachedData := m.retrieveConfigMapFromCache(tgb)
		if cachedData != nil {
			return nil
		}
	}

	endpoints := endpointStringFn()
	persistedEndpoints := make(sets.Set[string])

	for _, ep := range endpoints {
		persistedEndpoints.Insert(ep)
	}

	return m.persistConfigMap(ctx, persistedEndpoints, tgb)
}

func (m *multiClusterManagerImpl) FilterTargetsForDeregistration(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targets []TargetInfo) ([]TargetInfo, bool, error) {
	if !tgb.Spec.MultiClusterTargetGroup {
		return targets, false, nil
	}

	persistedEndpoints, err := m.getConfigMapContents(ctx, tgb)

	if err != nil {
		return nil, false, err
	}

	if persistedEndpoints == nil {
		// Initial state after enabling MC or new TGB, we don't have enough data to accurately deregister targets here.
		m.logger.Info(fmt.Sprintf("Initial data population for multicluster target group. No deregister will occur on this reconcile run for tg: %s", tgb.Spec.TargetGroupARN))
		return []TargetInfo{}, true, nil
	}

	filteredTargets := make([]TargetInfo, 0)

	// Loop through the purposed target lists, removing any targets that we have not stored in the config map.
	for _, target := range targets {
		if _, ok := persistedEndpoints[target.GetIdentifier()]; ok {
			filteredTargets = append(filteredTargets, target)
		}
	}

	return filteredTargets, false, nil
}

func (m *multiClusterManagerImpl) CleanUp(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	// Technically we should try this clean up anyway to clean up configmaps that would exist from
	// flipping between shared / not shared. However, it's a pretty big change for users not using multicluster support
	// to start having these delete cm calls. The concern is around bricking clusters where users do not use MC and forget
	// to include the new controller permissions for configmaps.
	// TL;DR We'll document not to flip between shared / not shared.

	// Always delete from in memory cache, as it's basically "free" to do so.
	m.configMapCacheMutex.Lock()
	delete(m.configMapCache, getCacheKey(tgb))
	m.configMapCacheMutex.Unlock()

	// If not using multicluster support currently, just bail here.
	if !tgb.Spec.MultiClusterTargetGroup {
		return nil
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: tgb.Namespace,
			Name:      getConfigMapName(tgb),
		},
	}
	err := m.kubeClient.Delete(ctx, cm)
	if err == nil {
		return nil
	}
	return client.IgnoreNotFound(err)
}

func (m *multiClusterManagerImpl) getConfigMapContents(ctx context.Context, tgb *elbv2api.TargetGroupBinding) (sets.Set[string], error) {

	// First load from cache.
	cachedData := m.retrieveConfigMapFromCache(tgb)
	if cachedData != nil {
		return cachedData, nil
	}

	// If not available from in-memory cache, acquire write lock, look up data from kube api, store into cache.
	cm := &corev1.ConfigMap{}

	err := m.apiReader.Get(ctx, client.ObjectKey{
		Namespace: tgb.Namespace,
		Name:      getConfigMapName(tgb),
	}, cm)

	if err == nil {
		targetSet := algorithm.CSVToStringSet(cm.Data[targetsKey])
		m.updateCache(tgb, targetSet)
		return targetSet, nil
	}

	// Detect not found error, if so first time running so need to populate the config map contents.
	err = client.IgnoreNotFound(err)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (m *multiClusterManagerImpl) retrieveConfigMapFromCache(tgb *elbv2api.TargetGroupBinding) sets.Set[string] {
	m.configMapCacheMutex.RLock()
	defer m.configMapCacheMutex.RUnlock()

	cacheKey := getCacheKey(tgb)

	if v, ok := m.configMapCache[cacheKey]; ok {
		return v
	}
	return nil
}

func (m *multiClusterManagerImpl) persistConfigMap(ctx context.Context, endpointMap sets.Set[string], tgb *elbv2api.TargetGroupBinding) error {

	targetData := algorithm.StringSetToCSV(endpointMap)

	// Update the cm in kube api, to ensure things work across controller restarts.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: tgb.Namespace,
			Name:      getConfigMapName(tgb),
		},
		Data: map[string]string{
			targetsKey: targetData,
		},
	}

	err := m.kubeClient.Update(ctx, cm)

	if err == nil {
		m.updateCache(tgb, endpointMap)
		return nil
	}

	// Check for initial case and create config map.
	err = client.IgnoreNotFound(err)
	if err == nil {
		err = m.kubeClient.Create(ctx, cm)
		if err == nil {
			m.updateCache(tgb, endpointMap)
		}
		return err
	}

	return err
}

func (m *multiClusterManagerImpl) updateCache(tgb *elbv2api.TargetGroupBinding, endpointMap sets.Set[string]) {
	m.configMapCacheMutex.Lock()
	defer m.configMapCacheMutex.Unlock()
	cacheKey := getCacheKey(tgb)
	m.configMapCache[cacheKey] = endpointMap
}

// getCacheKey generates a key to use with the k8s api
func getCacheKey(tgb *elbv2api.TargetGroupBinding) string {
	return fmt.Sprintf("%s-%s", tgb.Namespace, tgb.Name)
}

// getConfigMapName generates a config map name to use with the k8s api.
func getConfigMapName(tgb *elbv2api.TargetGroupBinding) string {
	return fmt.Sprintf("%s%s", trackedTargetsPrefix, tgb.Name)
}
