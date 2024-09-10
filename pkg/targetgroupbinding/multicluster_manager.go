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
)

// mergingStrategy The strategy used when updating tracked targets
type mergingStrategy int

const (
	merge mergingStrategy = iota
	replace
)

const (
	namespace            = "kube-system"
	trackedTargetsPrefix = "aws-lbc-targets-"
	targetsKey           = "targets"
)

// MultiClusterManager implements logic to support multiple LBCs managing the same Target Group.
type MultiClusterManager interface {
	// FilterTargets Given a purposed list of targets from a source (probably ELB API), filter the list down to only targets
	// the cluster should operate on.
	FilterTargets(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targetInfo []TargetInfo) ([]TargetInfo, error)

	// UpdateTrackedIPTargets Update the tracked target set in persistent storage
	UpdateTrackedIPTargets(ctx context.Context, ms mergingStrategy, endpoints []backend.PodEndpoint, tgb *elbv2api.TargetGroupBinding) error

	// UpdateTrackedInstanceTargets Update the tracked target set in persistent storage
	UpdateTrackedInstanceTargets(ctx context.Context, ms mergingStrategy, endpoints []backend.NodePortEndpoint, tgb *elbv2api.TargetGroupBinding) error

	// CleanUp Removes any resources used to implement multicluster support.
	CleanUp(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error
}

type multiClusterManagerImpl struct {
	kubeClient client.Client
	apiReader  client.Reader
	logger     logr.Logger
}

func (m *multiClusterManagerImpl) UpdateTrackedIPTargets(ctx context.Context, ms mergingStrategy, endpoints []backend.PodEndpoint, tgb *elbv2api.TargetGroupBinding) error {
	if !tgb.Spec.SharedTargetGroup {
		return nil
	}

	endpointStrings := make([]string, 0, len(endpoints))

	for _, ep := range endpoints {
		endpointStrings = append(endpointStrings, ep.GetIdentifier())
	}

	return m.updateTrackedTargets(ctx, ms, endpointStrings, tgb)
}

func (m *multiClusterManagerImpl) UpdateTrackedInstanceTargets(ctx context.Context, ms mergingStrategy, endpoints []backend.NodePortEndpoint, tgb *elbv2api.TargetGroupBinding) error {
	if !tgb.Spec.SharedTargetGroup {
		return nil
	}

	endpointStrings := make([]string, 0, len(endpoints))

	for _, ep := range endpoints {
		endpointStrings = append(endpointStrings, ep.GetIdentifier())
	}

	return m.updateTrackedTargets(ctx, ms, endpointStrings, tgb)
}

func (m *multiClusterManagerImpl) updateTrackedTargets(ctx context.Context, ms mergingStrategy, endpoints []string, tgb *elbv2api.TargetGroupBinding) error {

	persistedEndpoints, err := m.getConfigMapContents(ctx, tgb)
	if err != nil {
		return err
	}

	createNeeded := false
	if persistedEndpoints == nil {
		createNeeded = true
	}

	if createNeeded || ms == replace {
		persistedEndpoints = make(map[string]bool)
	}

	for _, ep := range endpoints {
		persistedEndpoints[ep] = true
	}

	return m.persistConfigMap(ctx, persistedEndpoints, createNeeded, tgb)
}

// NewMultiClusterManager constructs a multicluster manager that is immediately ready to use.
func NewMultiClusterManager(kubeClient client.Client, apiReader client.Reader, logger logr.Logger) MultiClusterManager {
	return &multiClusterManagerImpl{
		apiReader:  apiReader,
		kubeClient: kubeClient,
		logger:     logger,
	}
}

func (m *multiClusterManagerImpl) FilterTargets(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targets []TargetInfo) ([]TargetInfo, error) {
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

	// Loop through the purposed target lists, removing any targets that we have no stored within the config map.
	for _, target := range targets {
		if _, ok := persistedEndpoints[target.GetIdentifier()]; ok {
			filteredTargets = append(filteredTargets, target)
		}
	}

	return filteredTargets, nil
}

func (m *multiClusterManagerImpl) CleanUp(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	// Technically we should try this clean up anyway to clean up configmaps that would pop up from
	// flipping between shared / not shared. However, it's a pretty big change for users not using multicluster support
	// to start having these delete cm calls. The concern is around bricking clusters where users do not use MC and forget
	// to include the new controller permissions.
	// TL;DR We'll document not to flip between shared / not shared.
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
	cm := &corev1.ConfigMap{}

	err := m.apiReader.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      getConfigMapName(tgb),
	}, cm)

	if err == nil {
		return algorithm.CSVToMap(cm.Data[targetsKey]), nil
	}

	// Detect not found error, if so first time running so need to pre-populate the config map contents.
	err = client.IgnoreNotFound(err)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (m *multiClusterManagerImpl) persistConfigMap(ctx context.Context, endpointMap map[string]bool, createNeeded bool, tgb *elbv2api.TargetGroupBinding) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      getConfigMapName(tgb),
		},
		Data: map[string]string{
			targetsKey: algorithm.MapToCSV(endpointMap),
		},
	}

	if createNeeded {
		return m.kubeClient.Create(ctx, cm)
	}
	return m.kubeClient.Update(ctx, cm)
}

func getConfigMapName(tgb *elbv2api.TargetGroupBinding) string {
	return fmt.Sprintf("%s%s-%s", trackedTargetsPrefix, tgb.Namespace, tgb.Name)
}
