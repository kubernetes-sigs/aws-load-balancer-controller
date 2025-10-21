package shared_utils

import (
	"context"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sync"
	"time"
)

const (
	defaultTargetGroupNameToARNCacheTTL = 20 * time.Minute
)

type TargetGroupARNMapper interface {
	GetArnByName(ctx context.Context, targetGroupName string) (string, error)
	GetCache() *cache.Expiring
}

type targetGroupNameToArnMapper struct {
	elbv2Client services.ELBV2
	cache       *cache.Expiring
	cacheTTL    time.Duration
	cacheMutex  sync.RWMutex
}

func (t *targetGroupNameToArnMapper) GetCache() *cache.Expiring {
	return t.cache
}

func NewTargetGroupNameToArnMapper(elbv2Client services.ELBV2) TargetGroupARNMapper {
	return &targetGroupNameToArnMapper{
		elbv2Client: elbv2Client,
		cache:       cache.NewExpiring(),
		cacheTTL:    defaultTargetGroupNameToARNCacheTTL,
	}
}

// GetArnByName returns the ARN of an AWS target group identified by its name
func (t *targetGroupNameToArnMapper) GetArnByName(ctx context.Context, targetGroupName string) (string, error) {
	t.cacheMutex.Lock()
	defer t.cacheMutex.Unlock()

	if rawCacheItem, exists := t.cache.Get(targetGroupName); exists {
		return rawCacheItem.(string), nil
	}
	req := &elbv2sdk.DescribeTargetGroupsInput{
		Names: []string{targetGroupName},
	}

	targetGroups, err := t.elbv2Client.DescribeTargetGroupsAsList(ctx, req)
	if err != nil {
		return "", err
	}
	if len(targetGroups) != 1 {
		return "", errors.Errorf("expecting a single targetGroup with query [%s] but got %v", targetGroupName, len(targetGroups))
	}
	arn := *targetGroups[0].TargetGroupArn
	t.cache.Set(targetGroupName, arn, t.cacheTTL)
	return arn, nil
}
