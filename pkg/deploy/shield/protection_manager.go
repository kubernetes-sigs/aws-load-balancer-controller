package shield

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	shieldsdk "github.com/aws/aws-sdk-go/service/shield"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"time"
)

const (
	defaultProtectionInfoByResourceARNCacheTTL = 10 * time.Minute
	// service subscription rarely changes, cache it with longer period.
	defaultSubscriptionStateCacheTTL = 2 * time.Hour
	subscriptionStateCacheKey        = "subscriptionState"
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

func NewDefaultProtectionManager(shieldClient services.Shield, logger logr.Logger) *defaultProtectionManager {
	return &defaultProtectionManager{
		shieldClient:                        shieldClient,
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
	}
	m.logger.Info("enabling shield protection",
		"resourceARN", resourceARN,
		"protectionName", protectionName)
	resp, err := m.shieldClient.CreateProtectionWithContext(ctx, req)
	if err != nil {
		return "", err
	}
	protectionID := awssdk.StringValue(resp.ProtectionId)
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
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == shieldsdk.ErrCodeResourceNotFoundException {
			protectionInfo = nil
		} else {
			return nil, err
		}
	}
	if resp.Protection != nil {
		protectionInfo = &ProtectionInfo{
			Name: awssdk.StringValue(resp.Protection.Name),
			ID:   awssdk.StringValue(resp.Protection.Id),
		}
	}
	m.protectionInfoByResourceARNCache.Set(resourceARN, protectionInfo, m.protectionInfoByResourceARNCacheTTL)
	return protectionInfo, nil
}

func (m *defaultProtectionManager) IsSubscribed(ctx context.Context) (bool, error) {
	rawCacheItem, exists := m.subscriptionStateCache.Get(subscriptionStateCacheKey)
	if exists {
		subscriptionState := rawCacheItem.(string)
		return subscriptionState == shieldsdk.SubscriptionStateActive, nil
	}

	req := &shieldsdk.GetSubscriptionStateInput{}
	resp, err := m.shieldClient.GetSubscriptionStateWithContext(ctx, req)
	if err != nil {
		return false, err
	}
	subscriptionState := awssdk.StringValue(resp.SubscriptionState)
	m.subscriptionStateCache.Set(subscriptionStateCacheKey, subscriptionState, m.subscriptionStateCacheTTL)
	return subscriptionState == shieldsdk.SubscriptionStateActive, nil
}
