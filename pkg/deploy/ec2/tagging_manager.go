package ec2

import (
	"context"
	"errors"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
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

// abstraction around tagging operations for EC2.
type TaggingManager interface {
	// ReconcileTags will reconcile tags on resources.
	ReconcileTags(ctx context.Context, resID string, desiredTags map[string]string, opts ...ReconcileTagsOption) error

	// ListSecurityGroups returns SecurityGroups that matches any of the tagging requirements.
	ListSecurityGroups(ctx context.Context, tagFilters ...tracking.TagFilter) ([]networking.SecurityGroupInfo, error)
}

// NewDefaultTaggingManager constructs new defaultTaggingManager.
func NewDefaultTaggingManager(ec2Client services.EC2, networkingSGManager networking.SecurityGroupManager, vpcID string, logger logr.Logger) *defaultTaggingManager {
	return &defaultTaggingManager{
		ec2Client:           ec2Client,
		networkingSGManager: networkingSGManager,
		vpcID:               vpcID,
		logger:              logger,
	}
}

var _ TaggingManager = &defaultTaggingManager{}

// default implementation for TaggingManager.
type defaultTaggingManager struct {
	ec2Client           services.EC2
	networkingSGManager networking.SecurityGroupManager
	vpcID               string
	logger              logr.Logger
}

func (m *defaultTaggingManager) ReconcileTags(ctx context.Context, resID string, desiredTags map[string]string, opts ...ReconcileTagsOption) error {
	reconcileOpts := ReconcileTagsOptions{
		CurrentTags:    nil,
		IgnoredTagKeys: nil,
	}
	reconcileOpts.ApplyOptions(opts)
	currentTags := reconcileOpts.CurrentTags
	if currentTags == nil {
		// TODO: support read currentTags from AWS when we need to support more resources other than securityGroup.
		return errors.New("currentTags must be specified")
	}

	tagsToUpdate, tagsToRemove := algorithm.DiffStringMap(desiredTags, currentTags)
	for _, ignoredTagKey := range reconcileOpts.IgnoredTagKeys {
		delete(tagsToUpdate, ignoredTagKey)
		delete(tagsToRemove, ignoredTagKey)
	}

	if len(tagsToUpdate) > 0 {
		req := &ec2sdk.CreateTagsInput{
			Resources: []*string{awssdk.String(resID)},
			Tags:      convertTagsToSDKTags(tagsToUpdate),
		}

		m.logger.Info("adding resource tags",
			"resourceID", resID,
			"change", tagsToUpdate)
		if _, err := m.ec2Client.CreateTagsWithContext(ctx, req); err != nil {
			return err
		}
		m.logger.Info("added resource tags",
			"resourceID", resID)
	}

	if len(tagsToRemove) > 0 {
		req := &ec2sdk.DeleteTagsInput{
			Resources: []*string{awssdk.String(resID)},
			Tags:      convertTagsToSDKTags(tagsToRemove),
		}

		m.logger.Info("removing resource tags",
			"resourceID", resID,
			"change", tagsToRemove)
		if _, err := m.ec2Client.DeleteTagsWithContext(ctx, req); err != nil {
			return err
		}
		m.logger.Info("removed resource tags",
			"resourceID", resID)
	}
	return nil
}

func (m *defaultTaggingManager) ListSecurityGroups(ctx context.Context, tagFilters ...tracking.TagFilter) ([]networking.SecurityGroupInfo, error) {
	sgInfoByID := make(map[string]networking.SecurityGroupInfo)
	for _, tagFilter := range tagFilters {
		sgInfoByIDForTagFilter, err := m.listSecurityGroupsWithTagFilter(ctx, tagFilter)
		if err != nil {
			return nil, err
		}
		for sgID, sgInfo := range sgInfoByIDForTagFilter {
			sgInfoByID[sgID] = sgInfo
		}
	}

	sgInfos := make([]networking.SecurityGroupInfo, 0, len(sgInfoByID))
	for _, sgInfo := range sgInfoByID {
		sgInfos = append(sgInfos, sgInfo)
	}
	return sgInfos, nil
}

func (m *defaultTaggingManager) listSecurityGroupsWithTagFilter(ctx context.Context, tagFilter tracking.TagFilter) (map[string]networking.SecurityGroupInfo, error) {
	req := &ec2sdk.DescribeSecurityGroupsInput{
		Filters: []*ec2sdk.Filter{
			{
				Name:   awssdk.String("vpc-id"),
				Values: awssdk.StringSlice([]string{m.vpcID}),
			},
		},
	}

	for _, tagKey := range sets.StringKeySet(tagFilter).List() {
		tagValues := tagFilter[tagKey]
		var filter ec2sdk.Filter
		if len(tagValues) == 0 {
			tagFilterName := "tag-key"
			filter.Name = awssdk.String(tagFilterName)
			filter.Values = awssdk.StringSlice([]string{tagKey})
		} else {
			tagFilterName := fmt.Sprintf("tag:%v", tagKey)
			filter.Name = awssdk.String(tagFilterName)
			filter.Values = awssdk.StringSlice(tagValues)
		}
		req.Filters = append(req.Filters, &filter)
	}

	return m.networkingSGManager.FetchSGInfosByRequest(ctx, req)
}

// convert tags into AWS SDK tag presentation.
func convertTagsToSDKTags(tags map[string]string) []*ec2sdk.Tag {
	if len(tags) == 0 {
		return nil
	}
	sdkTags := make([]*ec2sdk.Tag, 0, len(tags))

	for _, key := range sets.StringKeySet(tags).List() {
		sdkTags = append(sdkTags, &ec2sdk.Tag{
			Key:   awssdk.String(key),
			Value: awssdk.String(tags[key]),
		})
	}
	return sdkTags
}
