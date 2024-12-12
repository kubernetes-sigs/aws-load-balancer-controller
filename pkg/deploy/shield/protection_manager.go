package shield

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	shieldsdk "github.com/aws/aws-sdk-go-v2/service/shield"
	shieldtypes "github.com/aws/aws-sdk-go-v2/service/shield/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"time"
)

const (
	defaultProtectionInfoByResourceARNCacheTTL = 10 * time.Minute
	// service subscription rarely changes, cache it with longer period.
	defaultSubscriptionStateCacheTTL = 2 * time.Hour
	subscriptionStateCacheKey        = "subscriptionState"
	tagKeyK8sCluster                 = "elbv2.k8s.aws/cluster"
)

type ProtectionManager interface {
	// CreateProtection creates shield protection for resource. returns protectionID.
	CreateProtection(ctx context.Context, resourceARN string, protectionName string) (string, error)

	// DeleteProtection deletes shield protection for resource.
	DeleteProtection(ctx context.Context, resourceARN string, protectionID string) error

	// GetProtection returns shield protection information for resource.
	// returns nil if no protection exists.
	GetProtection(ctx context.Context, resourceARN string) (*ProtectionInfo, error)

	// IsSubscribed checks whether subscribed to shield service.
	IsSubscribed(ctx context.Context) (bool, error)
}

func NewDefaultProtectionManager(shieldClient services.Shield, clusterName string, logger logr.Logger) *defaultProtectionManager {
	return &defaultProtectionManager{
		shieldClient:                        shieldClient,
		clusterName:                         clusterName,
		logger:                              logger,
		protectionInfoByResourceARNCache:    cache.NewExpiring(),
		protectionInfoByResourceARNCacheTTL: defaultProtectionInfoByResourceARNCacheTTL,
		subscriptionStateCache:              cache.NewExpiring(),
		subscriptionStateCacheTTL:           defaultSubscriptionStateCacheTTL,
	}
}

var _ ProtectionManager = &defaultProtectionManager{}

type defaultProtectionManager struct {
	shieldClient services.Shield
	clusterName  string
	logger       logr.Logger

	protectionInfoByResourceARNCache    *cache.Expiring
	protectionInfoByResourceARNCacheTTL time.Duration
	subscriptionStateCache              *cache.Expiring
	subscriptionStateCacheTTL           time.Duration
}

type ProtectionInfo struct {
	Name string
	ID   string
}

func (m *defaultProtectionManager) CreateProtection(ctx context.Context, resourceARN string, protectionName string) (string, error) {
	req := &shieldsdk.CreateProtectionInput{
		ResourceArn: awssdk.String(resourceARN),
		Name:        awssdk.String(protectionName),
		Tags: []*shieldsdk.Tag{
			&shieldsdk.Tag{
				Key:   awssdk.String(tagKeyK8sCluster),
				Value: awssdk.String(m.clusterName),
			},
		},
	}
	m.logger.Info("enabling shield protection",
		"resourceARN", resourceARN,
		"protectionName", protectionName, "TagKey", tagKeyK8sCluster, "TagValue", m.clusterName)
	resp, err := m.shieldClient.CreateProtectionWithContext(ctx, req)
	if err != nil {
		return "", err
	}
	protectionID := awssdk.ToString(resp.ProtectionId)
	m.logger.Info("enabled shield protection",
		"resourceARN", resourceARN,
		"protectionName", protectionName,
		"protectionID", protectionID)
	protectionInfo := &ProtectionInfo{
		Name: protectionName,
		ID:   protectionID,
	}
	m.protectionInfoByResourceARNCache.Set(resourceARN, protectionInfo, m.protectionInfoByResourceARNCacheTTL)
	return protectionID, nil
}

func (m *defaultProtectionManager) DeleteProtection(ctx context.Context, resourceARN string, protectionID string) error {
	req := &shieldsdk.DeleteProtectionInput{
		ProtectionId: awssdk.String(protectionID),
	}
	m.logger.Info("disabling shield protection",
		"resourceARN", resourceARN,
		"protectionID", protectionID)
	_, err := m.shieldClient.DeleteProtectionWithContext(ctx, req)
	if err != nil {
		return err
	}
	m.logger.Info("disabled shield protection",
		"resourceARN", resourceARN)

	var protectionInfo *ProtectionInfo
	m.protectionInfoByResourceARNCache.Set(resourceARN, protectionInfo, m.protectionInfoByResourceARNCacheTTL)
	return nil
}

func (m *defaultProtectionManager) GetProtection(ctx context.Context, resourceARN string) (*ProtectionInfo, error) {
	rawCacheItem, exists := m.protectionInfoByResourceARNCache.Get(resourceARN)
	if exists {
		return rawCacheItem.(*ProtectionInfo), nil
	}

	req := &shieldsdk.DescribeProtectionInput{
		ResourceArn: awssdk.String(resourceARN),
	}
	resp, err := m.shieldClient.DescribeProtectionWithContext(ctx, req)
	var protectionInfo *ProtectionInfo
	if err != nil {
		var resourceNotFoundException *shieldtypes.ResourceNotFoundException
		if errors.As(err, &resourceNotFoundException) {
			protectionInfo = nil
		} else {
			return nil, err
		}
	}
	if resp != nil && resp.Protection != nil {
		protectionInfo = &ProtectionInfo{
			Name: awssdk.ToString(resp.Protection.Name),
			ID:   awssdk.ToString(resp.Protection.Id),
		}
	}
	m.protectionInfoByResourceARNCache.Set(resourceARN, protectionInfo, m.protectionInfoByResourceARNCacheTTL)
	return protectionInfo, nil
}

func (m *defaultProtectionManager) IsSubscribed(ctx context.Context) (bool, error) {
	rawCacheItem, exists := m.subscriptionStateCache.Get(subscriptionStateCacheKey)
	if exists {
		subscriptionState := rawCacheItem.(shieldtypes.SubscriptionState)
		return shieldtypes.SubscriptionStateActive == subscriptionState, nil
	}

	req := &shieldsdk.GetSubscriptionStateInput{}
	resp, err := m.shieldClient.GetSubscriptionStateWithContext(ctx, req)
	if err != nil {
		return false, err
	}
	subscriptionState := resp.SubscriptionState
	m.subscriptionStateCache.Set(subscriptionStateCacheKey, subscriptionState, m.subscriptionStateCacheTTL)
	return shieldtypes.SubscriptionStateActive == subscriptionState, nil
}
