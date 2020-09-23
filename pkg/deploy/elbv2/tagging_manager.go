package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tagging"
)

const (
	// ELBV2 API supports up to 20 resource per DescribeTags API call.
	defaultDescribeTagsChunkSize = 20
)

// LoadBalancer with it's tags.
type LoadBalancerWithTags struct {
	LoadBalancer *elbv2sdk.LoadBalancer
	Tags         map[string]string
}

// TargetGroup with it's tags.
type TargetGroupWithTags struct {
	TargetGroup *elbv2sdk.TargetGroup
	Tags        map[string]string
}

// options for ReconcileTags API.
type ReconcileTagsOptions struct {
	// CurrentTags on resources.
	// when it's nil, the TaggingManager will try to get the CurrentTags from AWS
	CurrentTags map[string]string
}

func (opts *ReconcileTagsOptions) ApplyOptions(options []ReconcileTagsOption) {
	for _, option := range options {
		option(opts)
	}
}

type ReconcileTagsOption func(opts *ReconcileTagsOptions)

func WithCurrentTags(tags map[string]string) ReconcileTagsOption {
	return func(opts *ReconcileTagsOptions) {
		opts.CurrentTags = tags
	}
}

// abstraction around tagging operations for ELBV2.
type TaggingManager interface {
	// ReconcileTags will reconcile tags on resources.
	ReconcileTags(ctx context.Context, arn string, desiredTags map[string]string, opts ...ReconcileTagsOption) error

	// ListLoadBalancers returns LoadBalancers matching tagging requirements.
	ListLoadBalancers(ctx context.Context, tagFilter tagging.TagFilter) ([]LoadBalancerWithTags, error)

	// ListTargetGroups returns TargetGroups matching tagging requirements.
	ListTargetGroups(ctx context.Context, tagFilter tagging.TagFilter) ([]TargetGroupWithTags, error)
}

// NewDefaultTaggingManager constructs default TaggingManager.
func NewDefaultTaggingManager(elbv2Client services.ELBV2, logger logr.Logger) *defaultTaggingManager {
	return &defaultTaggingManager{
		elbv2Client: elbv2Client,
		logger:      logger,

		describeTagsChunkSize: defaultDescribeTagsChunkSize,
	}
}

var _ TaggingManager = &defaultTaggingManager{}

// default implementation for TaggingManager
// @TODO: use AWS Resource Groups Tagging API to optimize this implementation once it have PrivateLink support.
type defaultTaggingManager struct {
	elbv2Client services.ELBV2
	logger      logr.Logger

	describeTagsChunkSize int
}

func (m *defaultTaggingManager) ReconcileTags(ctx context.Context, arn string, desiredTags map[string]string, opts ...ReconcileTagsOption) error {
	reconcileOpts := ReconcileTagsOptions{}
	reconcileOpts.ApplyOptions(opts)
	currentTags := reconcileOpts.CurrentTags
	if currentTags == nil {
		tagsByARN, err := m.describeResourceTags(ctx, []string{arn})
		if err != nil {
			return err
		}
		currentTags = tagsByARN[arn]
	}

	tagsToUpdate, tagsToRemove := algorithm.DiffStringMap(desiredTags, currentTags)

	if len(tagsToUpdate) > 0 {
		req := &elbv2sdk.AddTagsInput{
			ResourceArns: []*string{awssdk.String(arn)},
			Tags:         convertTagsToSDKTags(tagsToUpdate),
		}

		m.logger.Info("adding resource tags",
			"arn", arn,
			"change", tagsToUpdate)
		if _, err := m.elbv2Client.AddTagsWithContext(ctx, req); err != nil {
			return err
		}
		m.logger.Info("added resource tags",
			"arn", arn)
	}

	if len(tagsToRemove) > 0 {
		tagKeys := sets.StringKeySet(tagsToRemove).List()
		req := &elbv2sdk.RemoveTagsInput{
			ResourceArns: []*string{awssdk.String(arn)},
			TagKeys:      awssdk.StringSlice(tagKeys),
		}

		m.logger.Info("removing resource tags",
			"arn", arn,
			"change", tagKeys)
		if _, err := m.elbv2Client.RemoveTagsWithContext(ctx, req); err != nil {
			return err
		}
		m.logger.Info("removed resource tags",
			"arn", arn)
	}
	return nil
}

func (m *defaultTaggingManager) ListLoadBalancers(ctx context.Context, tagFilter tagging.TagFilter) ([]LoadBalancerWithTags, error) {
	req := &elbv2sdk.DescribeLoadBalancersInput{}
	lbs, err := m.elbv2Client.DescribeLoadBalancersAsList(ctx, req)
	if err != nil {
		return nil, err
	}

	lbARNs := make([]string, 0, len(lbs))
	lbByARN := make(map[string]*elbv2sdk.LoadBalancer, len(lbs))
	for _, lb := range lbs {
		lbARN := awssdk.StringValue(lb.LoadBalancerArn)
		lbARNs = append(lbARNs, lbARN)
		lbByARN[lbARN] = lb
	}
	tagsByARN, err := m.describeResourceTags(ctx, lbARNs)
	if err != nil {
		return nil, err
	}

	var matchedLBs []LoadBalancerWithTags
	for _, arn := range lbARNs {
		if tagFilter.Matches(tagsByARN[arn]) {
			matchedLBs = append(matchedLBs, LoadBalancerWithTags{
				LoadBalancer: lbByARN[arn],
				Tags:         tagsByARN[arn],
			})
		}
	}
	return matchedLBs, nil
}

func (m *defaultTaggingManager) ListTargetGroups(ctx context.Context, tagFilter tagging.TagFilter) ([]TargetGroupWithTags, error) {
	req := &elbv2sdk.DescribeTargetGroupsInput{}
	tgs, err := m.elbv2Client.DescribeTargetGroupsAsList(ctx, req)
	if err != nil {
		return nil, err
	}

	tgARNs := make([]string, 0, len(tgs))
	tgByARN := make(map[string]*elbv2sdk.TargetGroup, len(tgs))
	for _, tg := range tgs {
		tgARN := awssdk.StringValue(tg.TargetGroupArn)
		tgARNs = append(tgARNs, tgARN)
		tgByARN[tgARN] = tg
	}
	tagsByARN, err := m.describeResourceTags(ctx, tgARNs)
	if err != nil {
		return nil, err
	}

	var matchedTGs []TargetGroupWithTags

	for _, arn := range tgARNs {
		if tagFilter.Matches(tagsByARN[arn]) {
			matchedTGs = append(matchedTGs, TargetGroupWithTags{
				TargetGroup: tgByARN[arn],
				Tags:        tagsByARN[arn],
			})
		}
	}
	return matchedTGs, nil
}

// describeResourceTags describes tags for elbv2 resources.
// returns tags indexed by resource ARN.
func (m *defaultTaggingManager) describeResourceTags(ctx context.Context, arns []string) (map[string]map[string]string, error) {
	tagsByARN := make(map[string]map[string]string, len(arns))
	arnsChunks := algorithm.ChunkStrings(arns, m.describeTagsChunkSize)
	for _, arnsChunk := range arnsChunks {
		req := &elbv2sdk.DescribeTagsInput{
			ResourceArns: awssdk.StringSlice(arnsChunk),
		}
		resp, err := m.elbv2Client.DescribeTagsWithContext(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, tagDescription := range resp.TagDescriptions {
			tagsByARN[awssdk.StringValue(tagDescription.ResourceArn)] = convertSDKTagsToTags(tagDescription.Tags)
		}
	}
	return tagsByARN, nil
}

// convert tags into AWS SDK tag presentation.
func convertTagsToSDKTags(tags map[string]string) []*elbv2sdk.Tag {
	if len(tags) == 0 {
		return nil
	}
	sdkTags := make([]*elbv2sdk.Tag, 0, len(tags))

	for _, key := range sets.StringKeySet(tags).List() {
		sdkTags = append(sdkTags, &elbv2sdk.Tag{
			Key:   awssdk.String(key),
			Value: awssdk.String(tags[key]),
		})
	}
	return sdkTags
}

// convert AWS SDK tag presentation into tags.
func convertSDKTagsToTags(sdkTags []*elbv2sdk.Tag) map[string]string {
	tags := make(map[string]string, len(sdkTags))
	for _, sdkTag := range sdkTags {
		tags[awssdk.StringValue(sdkTag.Key)] = awssdk.StringValue(sdkTag.Value)
	}
	return tags
}
