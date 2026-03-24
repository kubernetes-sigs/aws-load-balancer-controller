package aga

import (
	"context"
	"fmt"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	rgtsdk "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

const (
	// cache ttl for tags on GlobalAccelerator resources.
	defaultResourceTagsCacheTTL = 20 * time.Minute
)

// options for ReconcileTags API.
type ReconcileTagsOptions struct {
	// CurrentTags on resources.
	// when it's nil, the TaggingManager will try to get the CurrentTags from AWS
	CurrentTags map[string]string

	// IgnoredTagKeys defines the tag keys that should be ignored.
	// these tags shouldn't be altered or deleted.
	IgnoredTagKeys []string
}

func (opts *ReconcileTagsOptions) ApplyOptions(options []ReconcileTagsOption) {
	for _, option := range options {
		option(opts)
	}
}

type ReconcileTagsOption func(opts *ReconcileTagsOptions)

// WithCurrentTags is a reconcile option that supplies current tags.
func WithCurrentTags(tags map[string]string) ReconcileTagsOption {
	return func(opts *ReconcileTagsOptions) {
		opts.CurrentTags = tags
	}
}

// WithIgnoredTagKeys is a reconcile option that configures IgnoredTagKeys.
func WithIgnoredTagKeys(ignoredTagKeys []string) ReconcileTagsOption {
	return func(opts *ReconcileTagsOptions) {
		opts.IgnoredTagKeys = append(opts.IgnoredTagKeys, ignoredTagKeys...)
	}
}

// TaggingManager is responsible for tagging AGA resources.
type TaggingManager interface {
	// ReconcileTags will reconcile tags on resources.
	ReconcileTags(ctx context.Context, arn string, desiredTags map[string]string, opts ...ReconcileTagsOption) error

	// ConvertTagsToSDKTags Convert tags into AWS SDK tag presentation.
	ConvertTagsToSDKTags(tags map[string]string) []agatypes.Tag
}

// NewDefaultTaggingManager constructs new defaultTaggingManager.
func NewDefaultTaggingManager(gaService services.GlobalAccelerator, rgt services.RGT, logger logr.Logger) *defaultTaggingManager {
	return &defaultTaggingManager{
		gaService:            gaService,
		logger:               logger,
		resourceTagsCache:    cache.NewExpiring(),
		resourceTagsCacheTTL: defaultResourceTagsCacheTTL,
		rgt:                  rgt,
	}
}

var _ TaggingManager = &defaultTaggingManager{}

// defaultTaggingManager is the default implementation for TaggingManager.
type defaultTaggingManager struct {
	gaService services.GlobalAccelerator
	logger    logr.Logger
	// cache for tags on GlobalAccelerator resources.
	resourceTagsCache      *cache.Expiring
	resourceTagsCacheTTL   time.Duration
	resourceTagsCacheMutex sync.RWMutex
	rgt                    services.RGT
}

func (m *defaultTaggingManager) ReconcileTags(ctx context.Context, arn string, desiredTags map[string]string, opts ...ReconcileTagsOption) error {
	reconcileOpts := ReconcileTagsOptions{
		CurrentTags:    nil,
		IgnoredTagKeys: nil,
	}
	reconcileOpts.ApplyOptions(opts)
	currentTags := reconcileOpts.CurrentTags
	if currentTags == nil {
		var err error
		currentTags, err = m.describeResourceTags(ctx, arn)
		if err != nil {
			return err
		}
	}

	tagsToUpdate, tagsToRemove := algorithm.DiffStringMap(desiredTags, currentTags)
	for _, ignoredTagKey := range reconcileOpts.IgnoredTagKeys {
		delete(tagsToUpdate, ignoredTagKey)
		delete(tagsToRemove, ignoredTagKey)
	}

	if len(tagsToUpdate) > 0 {
		req := &globalaccelerator.TagResourceInput{
			ResourceArn: awssdk.String(arn),
			Tags:        m.ConvertTagsToSDKTags(tagsToUpdate),
		}

		m.logger.Info("adding resource tags",
			"arn", arn,
			"change", tagsToUpdate)
		if _, err := m.gaService.TagResourceWithContext(ctx, req); err != nil {
			return err
		}
		m.invalidateResourceTagsCache(arn)
		m.logger.Info("added resource tags",
			"arn", arn)
	}

	if len(tagsToRemove) > 0 {
		tagKeys := sets.StringKeySet(tagsToRemove).List()
		req := &globalaccelerator.UntagResourceInput{
			ResourceArn: awssdk.String(arn),
			TagKeys:     tagKeys,
		}

		m.logger.Info("removing resource tags",
			"arn", arn,
			"change", tagKeys)
		if _, err := m.gaService.UntagResourceWithContext(ctx, req); err != nil {
			return err
		}
		m.invalidateResourceTagsCache(arn)
		m.logger.Info("removed resource tags",
			"arn", arn)
	}
	return nil
}

func (m *defaultTaggingManager) describeResourceTags(ctx context.Context, arn string) (map[string]string, error) {
	m.resourceTagsCacheMutex.Lock()
	defer m.resourceTagsCacheMutex.Unlock()

	// Check if the ARN is in cache
	if rawTagsCacheItem, exists := m.resourceTagsCache.Get(arn); exists {
		tagsCacheItem := rawTagsCacheItem.(map[string]string)
		return tagsCacheItem, nil
	}

	// ARN not in cache, need to fetch from RGT API
	tags, err := m.describeResourceTagsFromRGT(ctx, arn)
	if err != nil {
		return nil, err
	}

	// Store in cache
	m.resourceTagsCache.Set(arn, tags, m.resourceTagsCacheTTL)

	return tags, nil
}

func (m *defaultTaggingManager) invalidateResourceTagsCache(arn string) {
	m.resourceTagsCacheMutex.Lock()
	defer m.resourceTagsCacheMutex.Unlock()

	m.resourceTagsCache.Delete(arn)
}

// Convert tags into AWS SDK tag presentation.
func (m *defaultTaggingManager) ConvertTagsToSDKTags(tags map[string]string) []agatypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	sdkTags := make([]agatypes.Tag, 0, len(tags))

	for _, key := range sets.StringKeySet(tags).List() {
		sdkTags = append(sdkTags, agatypes.Tag{
			Key:   awssdk.String(key),
			Value: awssdk.String(tags[key]),
		})
	}
	return sdkTags
}

// describeResourceTagsFromRGT describes tags for a GlobalAccelerator resource using the Resource Groups Tagging API.
// returns tags for the resource.
func (m *defaultTaggingManager) describeResourceTagsFromRGT(ctx context.Context, arn string) (map[string]string, error) {
	req := &rgtsdk.GetResourcesInput{
		ResourceARNList:     []string{arn},
		ResourceTypeFilters: []string{services.ResourceTypeGlobalAccelerator},
	}

	resources, err := m.rgt.GetResourcesAsList(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource from RGT API: %w", err)
	}

	// Check if the resource was found
	for _, resource := range resources {
		resourceArn := awssdk.ToString(resource.ResourceARN)
		if resourceArn == arn {
			return services.ParseRGTTags(resource.Tags), nil
		}
	}

	// Resource not found in RGT API - return error
	return nil, fmt.Errorf("resource not found in RGT API: %s", arn)
}
