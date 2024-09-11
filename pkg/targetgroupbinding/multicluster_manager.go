package targetgroupbinding

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/backend"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
)

const (
	namespace            = "kube-system"
	trackedTargetsPrefix = "aws-lbc-targets-"
	targetsKey           = "targets"
)

// MultiClusterManager implements logic to support multiple LBCs managing the same Target Group.
type MultiClusterManager interface {
	// FilterTargetsForDeregistration Given a purposed list of targets from a source (probably ELB API), filter the list down to only targets
	// the cluster should operate on.
	FilterTargetsForDeregistration(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targetInfo []TargetInfo) ([]TargetInfo, error)

	// UpdateTrackedIPTargets Update the tracked target set in persistent storage
	UpdateTrackedIPTargets(ctx context.Context, endpoints []backend.PodEndpoint, tgb *elbv2api.TargetGroupBinding) error

	// UpdateTrackedInstanceTargets Update the tracked target set in persistent storage
	UpdateTrackedInstanceTargets(ctx context.Context, endpoints []backend.NodePortEndpoint, tgb *elbv2api.TargetGroupBinding) error

	// CleanUp Removes any resources used to implement multicluster support.
	CleanUp(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error
}

type multiClusterManagerImpl struct {
	kubeClient client.Client
	apiReader  client.Reader
	logger     logr.Logger

	configMapCache      map[string]map[string]bool
	configMapCacheMutex sync.RWMutex
}

func (m *multiClusterManagerImpl) UpdateTrackedIPTargets(ctx context.Context, endpoints []backend.PodEndpoint, tgb *elbv2api.TargetGroupBinding) error {
	if !tgb.Spec.SharedTargetGroup {
		return nil
	}

	endpointStrings := make([]string, 0, len(endpoints))

	for _, ep := range endpoints {
		endpointStrings = append(endpointStrings, ep.GetIdentifier())
	}

	return m.updateTrackedTargets(ctx, endpointStrings, tgb)
}

func (m *multiClusterManagerImpl) UpdateTrackedInstanceTargets(ctx context.Context, endpoints []backend.NodePortEndpoint, tgb *elbv2api.TargetGroupBinding) error {
	if !tgb.Spec.SharedTargetGroup {
		return nil
	}

	endpointStrings := make([]string, 0, len(endpoints))

	for _, ep := range endpoints {
		endpointStrings = append(endpointStrings, ep.GetIdentifier())
	}

	return m.updateTrackedTargets(ctx, endpointStrings, tgb)
}

func (m *multiClusterManagerImpl) updateTrackedTargets(ctx context.Context, endpoints []string, tgb *elbv2api.TargetGroupBinding) error {
	persistedEndpoints := make(map[string]bool)

	for _, ep := range endpoints {
		persistedEndpoints[ep] = true
	}

	return m.persistConfigMap(ctx, persistedEndpoints, tgb)
}

// NewMultiClusterManager constructs a multicluster manager that is immediately ready to use.
func NewMultiClusterManager(kubeClient client.Client, apiReader client.Reader, logger logr.Logger) MultiClusterManager {
	return &multiClusterManagerImpl{
		apiReader:           apiReader,
		kubeClient:          kubeClient,
		logger:              logger,
		configMapCacheMutex: sync.RWMutex{},
		configMapCache:      make(map[string]map[string]bool),
	}
}

func (m *multiClusterManagerImpl) FilterTargetsForDeregistration(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targets []TargetInfo) ([]TargetInfo, error) {
	if !tgb.Spec.SharedTargetGroup {
		return targets, nil
	}

	persistedEndpoints, err := m.getConfigMapContents(ctx, tgb)

	if err != nil {
		return nil, err
	}

	if persistedEndpoints == nil {
		// Initial state after enabling MC or new TGB, we don't have enough data to accurately deregister targets here.
		m.logger.Info(fmt.Sprintf("Initial data population for multicluster target group. No deregister will occur on this reconcile run for tg: %s", tgb.Spec.TargetGroupARN))
		return []TargetInfo{}, nil
	}

	filteredTargets := make([]TargetInfo, 0)

	// Loop through the purposed target lists, removing any targets that we have not stored in the config map.
	for _, target := range targets {
		if _, ok := persistedEndpoints[target.GetIdentifier()]; ok {
			filteredTargets = append(filteredTargets, target)
		}
	}

	return filteredTargets, nil
}

func (m *multiClusterManagerImpl) CleanUp(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	// Technically we should try this clean up anyway to clean up configmaps that would exist from
	// flipping between shared / not shared. However, it's a pretty big change for users not using multicluster support
	// to start having these delete cm calls. The concern is around bricking clusters where users do not use MC and forget
	// to include the new controller permissions for configmaps.
	// TL;DR We'll document not to flip between shared / not shared.

	configMapName := getConfigMapName(tgb)

	// Always delete from in memory cache, as it's basically "free" to do so.
	m.configMapCacheMutex.Lock()
	delete(m.configMapCache, configMapName)
	m.configMapCacheMutex.Unlock()

	// If not using multicluster support currently, just bail here.
	if !tgb.Spec.SharedTargetGroup {
		return nil
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      getConfigMapName(tgb),
		},
	}
	err := m.kubeClient.Delete(ctx, cm)
	if err == nil {
		return nil
	}
	return client.IgnoreNotFound(err)
}

func (m *multiClusterManagerImpl) getConfigMapContents(ctx context.Context, tgb *elbv2api.TargetGroupBinding) (map[string]bool, error) {

	// First load from cache.
	configMapName := getConfigMapName(tgb)
	cachedData := m.retrieveConfigMapFromCache(configMapName)
	if cachedData != nil {
		return cachedData, nil
	}

	// If not available from in-memory cache, acquire write lock, look up data from kube api, store into cache.

	m.configMapCacheMutex.Lock()
	defer m.configMapCacheMutex.Unlock()

	cm := &corev1.ConfigMap{}

	err := m.apiReader.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      configMapName,
	}, cm)

	if err == nil {
		targetSet := algorithm.CSVToMap(cm.Data[targetsKey])
		m.configMapCache[configMapName] = targetSet
		return targetSet, nil
	}

	// Detect not found error, if so first time running so need to populate the config map contents.
	err = client.IgnoreNotFound(err)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (m *multiClusterManagerImpl) retrieveConfigMapFromCache(configMapName string) map[string]bool {
	m.configMapCacheMutex.RLock()
	defer m.configMapCacheMutex.RUnlock()

	if v, ok := m.configMapCache[configMapName]; ok {
		return v
	}
	return nil
}

func (m *multiClusterManagerImpl) persistConfigMap(ctx context.Context, endpointMap map[string]bool, tgb *elbv2api.TargetGroupBinding) error {

	configMapName := getConfigMapName(tgb)
	targetData := algorithm.MapToCSV(endpointMap)

	// Update the cm in kube api, to ensure things work across controller restarts.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      configMapName,
		},
		Data: map[string]string{
			targetsKey: targetData,
		},
	}

	err := m.kubeClient.Update(ctx, cm)

	if err == nil {
		m.updateCache(configMapName, endpointMap)
		return nil
	}

	// Check for initial case and create config map.
	err = client.IgnoreNotFound(err)
	if err == nil {
		err = m.kubeClient.Create(ctx, cm)
		if err == nil {
			m.updateCache(configMapName, endpointMap)
		}
		return err
	}

	return err
}

func (m *multiClusterManagerImpl) updateCache(configMapName string, endpointMap map[string]bool) {
	m.configMapCacheMutex.Lock()
	defer m.configMapCacheMutex.Unlock()
	m.configMapCache[configMapName] = endpointMap
}

func getConfigMapName(tgb *elbv2api.TargetGroupBinding) string {
	return fmt.Sprintf("%s%s-%s", trackedTargetsPrefix, tgb.Namespace, tgb.Name)
}
