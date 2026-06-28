package targetgroupbinding

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/cache"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

const (
	defaultTargetsCacheTTL            = 5 * time.Minute
	defaultRegisterTargetsChunkSize   = 200
	defaultDeregisterTargetsChunkSize = 200
	defaultNeedsPodAZCacheTTL         = 60 * time.Minute
	defaultNodeAZCacheTTL             = 60 * time.Minute
)

// TargetGroupTargets represents a target group binding and its targets to register.
type TargetGroupTargets struct {
	TGB     *elbv2api.TargetGroupBinding
	Targets []elbv2types.TargetDescription
}

// TargetsManager is an abstraction around ELBV2's targets API.
type TargetsManager interface {
	// Register Targets into TargetGroup.
	RegisterTargets(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targets []elbv2types.TargetDescription) error

	// RegisterTargetsInterleaved registers targets to multiple target groups in an interleaved manner.
	// This ensures fair distribution of targets across target groups when quota limits are reached.
	// Instead of registering all targets to TG1, then TG2, etc., it registers one target to each TG
	// before moving to the next target, resulting in even distribution.
	RegisterTargetsInterleaved(ctx context.Context, tgbTargets []TargetGroupTargets) error

	// Deregister Targets from TargetGroup.
	DeregisterTargets(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targets []elbv2types.TargetDescription) error

	// List Targets from TargetGroup.
	ListTargets(ctx context.Context, tgb *elbv2api.TargetGroupBinding) ([]TargetInfo, error)
}

// NewCachedTargetsManager constructs new cachedTargetsManager
func NewCachedTargetsManager(elbv2Client services.ELBV2, logger logr.Logger) *cachedTargetsManager {
	return &cachedTargetsManager{
		elbv2Client:                elbv2Client,
		targetsCache:               cache.NewExpiring(),
		targetsCacheTTL:            defaultTargetsCacheTTL,
		registerTargetsChunkSize:   defaultRegisterTargetsChunkSize,
		deregisterTargetsChunkSize: defaultDeregisterTargetsChunkSize,
		logger:                     logger,
	}
}

var _ TargetsManager = &cachedTargetsManager{}

// an cached implementation for TargetsManager.
// Targets for each TargetGroup will be refreshed per targetsCacheTTL.
// When list Targets with RefreshTargets list Option set,
// only targets with ongoing TargetHealth(unknown/initial/draining) TargetHealth will be refreshed.
type cachedTargetsManager struct {
	elbv2Client services.ELBV2

	// cache of targets by targetGroupARN.
	// NOTE: since this cache implementation will automatically GC expired entries, we don't need to GC entries.
	// Otherwise, we'll need to GC entries according to all TargetGroupBindings in cluster to avoid cache grow indefinitely.
	targetsCache *cache.Expiring
	// TTL for each targetGroup's targets.
	targetsCacheTTL time.Duration
	// targetsCacheMutex protects targetsCache
	targetsCacheMutex sync.RWMutex

	// chunk size for registerTargets API call.
	registerTargetsChunkSize int
	// chunk size for deregisterTargets API call.
	deregisterTargetsChunkSize int

	logger logr.Logger
}

// cache entry for targetsCache
type targetsCacheItem struct {
	// mutex protects below fields
	mutex sync.RWMutex
	// targets is the targets for TargetGroup
	targets []TargetInfo
}

func (m *cachedTargetsManager) RegisterTargets(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targets []elbv2types.TargetDescription) error {
	tgARN := tgb.Spec.TargetGroupARN
	targetsChunks := chunkTargetDescriptions(targets, m.registerTargetsChunkSize)
	for _, targetsChunk := range targetsChunks {
		req := &elbv2sdk.RegisterTargetsInput{
			TargetGroupArn: aws.String(tgARN),
			Targets:        cloneTargetDescriptionSlice(targetsChunk),
		}
		m.logger.Info("registering targets",
			"arn", tgARN,
			"targets", targetsChunk)

		clientToUse, err := m.elbv2Client.AssumeRole(ctx, tgb.Spec.IamRoleArnToAssume, tgb.Spec.AssumeRoleExternalId)
		if err != nil {
			return err
		}

		_, err = clientToUse.RegisterTargetsWithContext(ctx, req)
		if err != nil {
			return err
		}
		m.logger.Info("registered targets",
			"arn", tgARN,
			"targets", targetsChunk)
		m.recordSuccessfulRegisterTargetsOperation(tgARN, targetsChunk)
	}
	return nil
}

// RegisterTargetsInterleaved registers targets to multiple target groups in an interleaved manner.
// This ensures fair distribution when AWS quota limits are reached.
// For example, with 3 target groups and targets [A, B, C], instead of:
//   - TG1: A, B, C (then TG2, TG3 get nothing if quota exceeded)
//
// It registers:
//   - TG1: A, TG2: A, TG3: A, TG1: B, TG2: B, TG3: B, ...
//
// This way, if quota is exceeded, all target groups have roughly equal targets.
func (m *cachedTargetsManager) RegisterTargetsInterleaved(ctx context.Context, tgbTargets []TargetGroupTargets) error {
	if len(tgbTargets) == 0 {
		return nil
	}

	// If only one target group, use the standard method
	if len(tgbTargets) == 1 {
		return m.RegisterTargets(ctx, tgbTargets[0].TGB, tgbTargets[0].Targets)
	}

	// Find the maximum number of targets across all target groups
	maxTargets := 0
	for _, tgt := range tgbTargets {
		if len(tgt.Targets) > maxTargets {
			maxTargets = len(tgt.Targets)
		}
	}

	// Register targets in interleaved manner, one target per TG at a time
	// We batch up to chunkSize targets per TG before making the API call
	// to balance between fairness and API efficiency
	for startIdx := 0; startIdx < maxTargets; startIdx += m.registerTargetsChunkSize {
		endIdx := startIdx + m.registerTargetsChunkSize
		if endIdx > maxTargets {
			endIdx = maxTargets
		}

		// For each target group, register the current chunk of targets
		for _, tgt := range tgbTargets {
			// Skip if this TG has fewer targets than the current index
			if startIdx >= len(tgt.Targets) {
				continue
			}

			// Get the chunk for this TG
			chunkEnd := endIdx
			if chunkEnd > len(tgt.Targets) {
				chunkEnd = len(tgt.Targets)
			}
			targetsChunk := tgt.Targets[startIdx:chunkEnd]

			if len(targetsChunk) == 0 {
				continue
			}

			tgARN := tgt.TGB.Spec.TargetGroupARN
			req := &elbv2sdk.RegisterTargetsInput{
				TargetGroupArn: aws.String(tgARN),
				Targets:        cloneTargetDescriptionSlice(targetsChunk),
			}

			m.logger.Info("registering targets (interleaved)",
				"arn", tgARN,
				"targets", targetsChunk,
				"chunkIndex", startIdx/m.registerTargetsChunkSize)

			clientToUse, err := m.elbv2Client.AssumeRole(ctx, tgt.TGB.Spec.IamRoleArnToAssume, tgt.TGB.Spec.AssumeRoleExternalId)
			if err != nil {
				return err
			}

			_, err = clientToUse.RegisterTargetsWithContext(ctx, req)
			if err != nil {
				// Log the error but continue with other target groups to ensure fair distribution
				m.logger.Error(err, "failed to register targets (interleaved), continuing with other target groups",
					"arn", tgARN,
					"targets", targetsChunk)
				// We continue instead of returning to ensure other TGs get their fair share
				continue
			}

			m.logger.Info("registered targets (interleaved)",
				"arn", tgARN,
				"targets", targetsChunk)
			m.recordSuccessfulRegisterTargetsOperation(tgARN, targetsChunk)
		}
	}

	return nil
}

func (m *cachedTargetsManager) DeregisterTargets(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targets []elbv2types.TargetDescription) error {
	tgARN := tgb.Spec.TargetGroupARN
	targetsChunks := chunkTargetDescriptions(targets, m.deregisterTargetsChunkSize)
	for _, targetsChunk := range targetsChunks {
		req := &elbv2sdk.DeregisterTargetsInput{
			TargetGroupArn: aws.String(tgARN),
			Targets:        cloneTargetDescriptionSlice(targetsChunk),
		}
		m.logger.Info("deRegistering targets",
			"arn", tgARN,
			"targets", targetsChunk)
		clientToUse, err := m.elbv2Client.AssumeRole(ctx, tgb.Spec.IamRoleArnToAssume, tgb.Spec.AssumeRoleExternalId)
		if err != nil {
			return err
		}
		_, err = clientToUse.DeregisterTargetsWithContext(ctx, req)
		if err != nil {
			return err
		}
		m.logger.Info("deRegistered targets",
			"arn", tgARN,
			"targets", targetsChunk)
		m.recordSuccessfulDeregisterTargetsOperation(tgARN, targetsChunk)
	}
	return nil
}

func (m *cachedTargetsManager) ListTargets(ctx context.Context, tgb *elbv2api.TargetGroupBinding) ([]TargetInfo, error) {
	tgARN := tgb.Spec.TargetGroupARN
	m.targetsCacheMutex.Lock()
	defer m.targetsCacheMutex.Unlock()

	if rawTargetsCacheItem, exists := m.targetsCache.Get(tgARN); exists {
		targetsCacheItem := rawTargetsCacheItem.(*targetsCacheItem)
		targetsCacheItem.mutex.Lock()
		defer targetsCacheItem.mutex.Unlock()
		refreshedTargets, err := m.refreshUnhealthyTargets(ctx, tgb, targetsCacheItem.targets)
		if err != nil {
			return nil, err
		}
		targetsCacheItem.targets = refreshedTargets
		return cloneTargetInfoSlice(refreshedTargets), nil
	}

	refreshedTargets, err := m.refreshAllTargets(ctx, tgb)
	if err != nil {
		return nil, err
	}
	targetsCacheItem := &targetsCacheItem{
		mutex:   sync.RWMutex{},
		targets: refreshedTargets,
	}
	m.targetsCache.Set(tgARN, targetsCacheItem, m.targetsCacheTTL)
	return cloneTargetInfoSlice(refreshedTargets), nil
}

// refreshAllTargets will refresh all targets for targetGroup.
func (m *cachedTargetsManager) refreshAllTargets(ctx context.Context, tgb *elbv2api.TargetGroupBinding) ([]TargetInfo, error) {
	targets, err := m.listTargetsFromAWS(ctx, tgb, nil)
	if err != nil {
		return nil, err
	}
	return targets, nil
}

// refreshUnhealthyTargets will refresh targets that are not in healthy status for targetGroup.
// To save API calls, we don't refresh targets that are already healthy since once a target turns healthy, we'll unblock it's readinessProbe.
// we can do nothing from controller perspective when a healthy target becomes unhealthy.
func (m *cachedTargetsManager) refreshUnhealthyTargets(ctx context.Context, tgb *elbv2api.TargetGroupBinding, cachedTargets []TargetInfo) ([]TargetInfo, error) {
	var refreshedTargets []TargetInfo
	var unhealthyTargets []elbv2types.TargetDescription
	for _, cachedTarget := range cachedTargets {
		if cachedTarget.IsHealthy() {
			refreshedTargets = append(refreshedTargets, cachedTarget)
		} else {
			unhealthyTargets = append(unhealthyTargets, cachedTarget.Target)
		}
	}
	if len(unhealthyTargets) == 0 {
		return refreshedTargets, nil
	}

	refreshedUnhealthyTargets, err := m.listTargetsFromAWS(ctx, tgb, unhealthyTargets)
	if err != nil {
		return nil, err
	}
	for _, target := range refreshedUnhealthyTargets {
		if target.IsNotRegistered() {
			continue
		}
		refreshedTargets = append(refreshedTargets, target)
	}
	return refreshedTargets, nil
}

// listTargetsFromAWS will list targets for TargetGroup using ELBV2API.
// if specified targets is non-empty, only these targets will be listed.
// otherwise, all targets for targetGroup will be listed.
func (m *cachedTargetsManager) listTargetsFromAWS(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targets []elbv2types.TargetDescription) ([]TargetInfo, error) {
	tgARN := tgb.Spec.TargetGroupARN
	req := &elbv2sdk.DescribeTargetHealthInput{
		TargetGroupArn: aws.String(tgARN),
		Targets:        targetByIdPort(targets),
	}
	clientToUse, err := m.elbv2Client.AssumeRole(ctx, tgb.Spec.IamRoleArnToAssume, tgb.Spec.AssumeRoleExternalId)
	if err != nil {
		return nil, err
	}
	resp, err := clientToUse.DescribeTargetHealthWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	listedTargets := make([]TargetInfo, 0, len(resp.TargetHealthDescriptions))
	for _, elem := range resp.TargetHealthDescriptions {
		listedTargets = append(listedTargets, TargetInfo{
			Target:       *elem.Target,
			TargetHealth: elem.TargetHealth,
		})
	}
	return listedTargets, nil
}

// recordSuccessfulRegisterTargetsOperation will record a successful deregisterTarget operation
func (m *cachedTargetsManager) recordSuccessfulRegisterTargetsOperation(tgARN string, targets []elbv2types.TargetDescription) {
	m.targetsCacheMutex.RLock()
	rawTargetsCacheItem, exists := m.targetsCache.Get(tgARN)
	m.targetsCacheMutex.RUnlock()

	if !exists {
		return
	}
	targetsByUniqueID := make(map[string]elbv2types.TargetDescription, len(targets))
	for _, target := range targets {
		targetsByUniqueID[UniqueIDForTargetDescription(target)] = target
	}

	targetsCacheItem := rawTargetsCacheItem.(*targetsCacheItem)
	targetsCacheItem.mutex.Lock()
	defer targetsCacheItem.mutex.Unlock()
	for i := range targetsCacheItem.targets {
		cachedTargetUniqueID := UniqueIDForTargetDescription(targetsCacheItem.targets[i].Target)
		if _, ok := targetsByUniqueID[cachedTargetUniqueID]; ok {
			targetsCacheItem.targets[i].TargetHealth = nil
			delete(targetsByUniqueID, cachedTargetUniqueID)
		}
	}

	for _, target := range targetsByUniqueID {
		targetsCacheItem.targets = append(targetsCacheItem.targets, TargetInfo{
			Target:       target,
			TargetHealth: nil,
		})
	}
}

// recordSuccessfulDeregisterTargetsOperation will record a successful deregisterTarget operation
func (m *cachedTargetsManager) recordSuccessfulDeregisterTargetsOperation(tgARN string, targets []elbv2types.TargetDescription) {
	m.targetsCacheMutex.RLock()
	rawTargetsCacheItem, exists := m.targetsCache.Get(tgARN)
	m.targetsCacheMutex.RUnlock()

	if !exists {
		return
	}
	targetsByUniqueID := make(map[string]elbv2types.TargetDescription, len(targets))
	for _, target := range targets {
		targetsByUniqueID[UniqueIDForTargetDescription(target)] = target
	}

	targetsCacheItem := rawTargetsCacheItem.(*targetsCacheItem)
	targetsCacheItem.mutex.Lock()
	defer targetsCacheItem.mutex.Unlock()
	for i := range targetsCacheItem.targets {
		cachedTargetUniqueID := UniqueIDForTargetDescription(targetsCacheItem.targets[i].Target)
		if _, ok := targetsByUniqueID[cachedTargetUniqueID]; ok {
			targetsCacheItem.targets[i].TargetHealth = nil
			delete(targetsByUniqueID, cachedTargetUniqueID)
		}
	}
}

// chunkTargetDescriptions will split slice of TargetDescription into chunks
func chunkTargetDescriptions(targets []elbv2types.TargetDescription, chunkSize int) [][]elbv2types.TargetDescription {
	var chunks [][]elbv2types.TargetDescription
	for i := 0; i < len(targets); i += chunkSize {
		end := i + chunkSize
		if end > len(targets) {
			end = len(targets)
		}
		chunks = append(chunks, targets[i:end])
	}
	return chunks
}

// targetByIdPort returns targets with only Id and Port fields.
// Omitting AZ ensures DescribeTargetHealth finds targets regardless of cached AZ state.
func targetByIdPort(targets []elbv2types.TargetDescription) []elbv2types.TargetDescription {
	if len(targets) == 0 {
		return nil
	}
	result := make([]elbv2types.TargetDescription, 0, len(targets))
	for _, t := range targets {
		result = append(result, elbv2types.TargetDescription{
			Id:   t.Id,
			Port: t.Port,
		})
	}
	return result
}

// cloneTargetDescriptionSlice returns a shallow copy of the TargetDescription slice.
func cloneTargetDescriptionSlice(targets []elbv2types.TargetDescription) []elbv2types.TargetDescription {
	if len(targets) == 0 {
		return nil
	}
	result := make([]elbv2types.TargetDescription, 0, len(targets))
	for i := range targets {
		result = append(result, targets[i])
	}
	return result
}

// cloneTargetInfoSlice returns a clone of TargetInfoSlice.
func cloneTargetInfoSlice(targets []TargetInfo) []TargetInfo {
	return append(targets[:0:0], targets...)
}
