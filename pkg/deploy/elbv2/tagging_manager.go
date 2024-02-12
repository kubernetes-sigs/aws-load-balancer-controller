package elbv2

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/cache"

	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
)

const (
	// ELBV2 API supports up to 20 resource per DescribeTags API call.
	defaultDescribeTagsChunkSize = 20
	// cache ttl for tags on ELB resources.
	defaultResourceTagsCacheTTL = 20 * time.Minute
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

// Listener with it's tags.
type ListenerWithTags struct {
	Listener *elbv2sdk.Listener
	Tags     map[string]string
}

// ListenerRule with tags
type ListenerRuleWithTags struct {
	ListenerRule *elbv2sdk.Rule
	Tags         map[string]string
}

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

// abstraction around tagging operations for ELBV2.
type TaggingManager interface {
	// ReconcileTags will reconcile tags on resources.
	ReconcileTags(ctx context.Context, arn string, desiredTags map[string]string, opts ...ReconcileTagsOption) error

	// ListLoadBalancers returns LoadBalancers that matches any of the tagging requirements.
	ListLoadBalancers(ctx context.Context, tagFilters ...tracking.TagFilter) ([]LoadBalancerWithTags, error)

	// ListTargetGroups returns TargetGroups that matches any of the tagging requirements.
	ListTargetGroups(ctx context.Context, tagFilters ...tracking.TagFilter) ([]TargetGroupWithTags, error)

	// ListListeners returns the LoadBalancer listeners along with tags
	ListListeners(ctx context.Context, lbARN string) ([]ListenerWithTags, error)

	// ListListenerRules returns the Listener Rules along with tags
	ListListenerRules(ctx context.Context, lsARN string) ([]ListenerRuleWithTags, error)
}

// NewDefaultTaggingManager constructs default TaggingManager.
func NewDefaultTaggingManager(elbv2Client services.ELBV2, vpcID string, featureGates config.FeatureGates, rgt services.RGT, logger logr.Logger) *defaultTaggingManager {
	return &defaultTaggingManager{
		elbv2Client:           elbv2Client,
		vpcID:                 vpcID,
		featureGates:          featureGates,
		logger:                logger,
		describeTagsChunkSize: defaultDescribeTagsChunkSize,
		resourceTagsCache:     cache.NewExpiring(),
		resourceTagsCacheTTL:  defaultResourceTagsCacheTTL,
		rgt:                   rgt,
	}
}

var _ TaggingManager = &defaultTaggingManager{}

// default implementation for TaggingManager
// @TODO: deprecate ELB API and only use AWS Resource Groups Tagging API to optimize this implementation once RGT has PrivateLink support.
type defaultTaggingManager struct {
	elbv2Client           services.ELBV2
	vpcID                 string
	featureGates          config.FeatureGates
	logger                logr.Logger
	describeTagsChunkSize int
	// cache for tags on ELB resources.
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
		tagsByARN, err := m.describeResourceTags(ctx, []string{arn})
		if err != nil {
			return err
		}
		currentTags = tagsByARN[arn]
	}

	tagsToUpdate, tagsToRemove := algorithm.DiffStringMap(desiredTags, currentTags)
	for _, ignoredTagKey := range reconcileOpts.IgnoredTagKeys {
		delete(tagsToUpdate, ignoredTagKey)
		delete(tagsToRemove, ignoredTagKey)
	}

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
		m.invalidateResourceTagsCache(arn)
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
		m.invalidateResourceTagsCache(arn)
		m.logger.Info("removed resource tags",
			"arn", arn)
	}
	return nil
}

func (m *defaultTaggingManager) ListListeners(ctx context.Context, lbARN string) ([]ListenerWithTags, error) {
	req := &elbv2sdk.DescribeListenersInput{
		LoadBalancerArn: awssdk.String(lbARN),
	}
	listeners, err := m.elbv2Client.DescribeListenersAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	lsARNs := make([]string, 0, len(listeners))
	lsByARN := make(map[string]*elbv2sdk.Listener, len(listeners))
	for _, listener := range listeners {
		lsARN := awssdk.StringValue(listener.ListenerArn)
		lsARNs = append(lsARNs, lsARN)
		lsByARN[lsARN] = listener
	}
	var tagsByARN map[string]map[string]string
	if m.featureGates.Enabled(config.ListenerRulesTagging) {
		tagsByARN, err = m.describeResourceTags(ctx, lsARNs)
		if err != nil {
			return nil, err
		}
	}
	var sdkLSs []ListenerWithTags
	for _, arn := range lsARNs {
		tags := tagsByARN[arn]
		sdkLSs = append(sdkLSs, ListenerWithTags{
			Listener: lsByARN[arn],
			Tags:     tags,
		})
	}
	return sdkLSs, err
}

func (m *defaultTaggingManager) ListListenerRules(ctx context.Context, lsARN string) ([]ListenerRuleWithTags, error) {
	req := &elbv2sdk.DescribeRulesInput{
		ListenerArn: awssdk.String(lsARN),
	}
	rules, err := m.elbv2Client.DescribeRulesAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	lrARNs := make([]string, 0, len(rules))
	lrByARN := make(map[string]*elbv2sdk.Rule, len(rules))
	for _, rule := range rules {
		lrARN := awssdk.StringValue(rule.RuleArn)
		lrARNs = append(lrARNs, lrARN)
		lrByARN[lrARN] = rule
	}
	var tagsByARN map[string]map[string]string
	if m.featureGates.Enabled(config.ListenerRulesTagging) {
		tagsByARN, err = m.describeResourceTags(ctx, lrARNs)
		if err != nil {
			return nil, err
		}
	}
	var sdkLRs []ListenerRuleWithTags
	for _, arn := range lrARNs {
		tags := tagsByARN[arn]
		sdkLRs = append(sdkLRs, ListenerRuleWithTags{
			ListenerRule: lrByARN[arn],
			Tags:         tags,
		})
	}
	return sdkLRs, err
}

// TODO: we can refactor this by store provisioned LB's ARN as annotations on Ingress/Service, thus avoid this heavy lookup calls when RGT is not available.
func (m *defaultTaggingManager) ListLoadBalancers(ctx context.Context, tagFilters ...tracking.TagFilter) ([]LoadBalancerWithTags, error) {
	if m.featureGates.Enabled(config.EnableRGTAPI) {
		m.logger.V(1).Info("ResourceGroupTagging enabled, list the load balancers via RGT API")
		return m.listLoadBalancersRGT(ctx, tagFilters)
	}
	return m.listLoadBalancersNative(ctx, tagFilters)
}

func (m *defaultTaggingManager) ListTargetGroups(ctx context.Context, tagFilters ...tracking.TagFilter) ([]TargetGroupWithTags, error) {
	if m.featureGates.Enabled(config.EnableRGTAPI) {
		m.logger.V(1).Info("ResourceGroupTagging enabled, list the target groups via RGT API")
		return m.listTargetGroupsRGT(ctx, tagFilters)
	}
	return m.listTargetGroupsNative(ctx, tagFilters)
}

func (m *defaultTaggingManager) listLoadBalancersRGT(ctx context.Context, tagFilters []tracking.TagFilter) ([]LoadBalancerWithTags, error) {
	// use a map to avoid potential duplication in returned resources
	resourceTagsByARN := make(map[string][]*resourcegroupstaggingapi.Tag)
	for _, tagFilter := range tagFilters {
		req := &resourcegroupstaggingapi.GetResourcesInput{
			TagFilters:          convertTagFiltersToRGTTagFilters(tagFilter),
			ResourceTypeFilters: awssdk.StringSlice([]string{services.ResourceTypeELBLoadBalancer}),
		}
		resources, err := m.rgt.GetResourcesAsList(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			if _, exists := resourceTagsByARN[awssdk.StringValue(resource.ResourceARN)]; !exists {
				resourceTagsByARN[awssdk.StringValue(resource.ResourceARN)] = resource.Tags
			}
		}
	}
	var matchedLBs []LoadBalancerWithTags
	for resourceARN, resourceTags := range resourceTagsByARN {
		elbv2Req := &elbv2sdk.DescribeLoadBalancersInput{
			LoadBalancerArns: []*string{&resourceARN},
		}
		elbv2Resp, err := m.elbv2Client.DescribeLoadBalancersAsList(ctx, elbv2Req)
		if err != nil {
			return nil, err
		}
		if len(elbv2Resp) == 0 {
			return nil, errors.Errorf("no load balancer found for the arn: %v", resourceARN)
		}
		matchedLBs = append(matchedLBs, LoadBalancerWithTags{
			LoadBalancer: elbv2Resp[0],
			Tags:         services.ParseRGTTags(resourceTags),
		})
	}
	return matchedLBs, nil
}

func (m *defaultTaggingManager) listLoadBalancersNative(ctx context.Context, tagFilters []tracking.TagFilter) ([]LoadBalancerWithTags, error) {
	req := &elbv2sdk.DescribeLoadBalancersInput{}
	lbs, err := m.elbv2Client.DescribeLoadBalancersAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	lbARNsWithinVPC := make([]string, 0, len(lbs))
	lbByARNWithinVPC := make(map[string]*elbv2sdk.LoadBalancer, len(lbs))
	for _, lb := range lbs {
		if awssdk.StringValue(lb.VpcId) != m.vpcID {
			continue
		}
		lbARN := awssdk.StringValue(lb.LoadBalancerArn)
		lbARNsWithinVPC = append(lbARNsWithinVPC, lbARN)
		lbByARNWithinVPC[lbARN] = lb
	}
	tagsByARN, err := m.describeResourceTags(ctx, lbARNsWithinVPC)
	if err != nil {
		return nil, err
	}

	var matchedLBs []LoadBalancerWithTags
	for _, arn := range lbARNsWithinVPC {
		tags := tagsByARN[arn]
		matchedAnyTagFilter := false
		for _, tagFilter := range tagFilters {
			if tagFilter.Matches(tags) {
				matchedAnyTagFilter = true
				break
			}
		}
		if matchedAnyTagFilter {
			matchedLBs = append(matchedLBs, LoadBalancerWithTags{
				LoadBalancer: lbByARNWithinVPC[arn],
				Tags:         tags,
			})
		}
	}
	return matchedLBs, nil
}

func (m *defaultTaggingManager) listTargetGroupsRGT(ctx context.Context, tagFilters []tracking.TagFilter) ([]TargetGroupWithTags, error) {
	// use a map to avoid potential duplication in returned resources
	resourceTagsByARN := make(map[string][]*resourcegroupstaggingapi.Tag)
	for _, tagFilter := range tagFilters {
		req := &resourcegroupstaggingapi.GetResourcesInput{
			TagFilters:          convertTagFiltersToRGTTagFilters(tagFilter),
			ResourceTypeFilters: awssdk.StringSlice([]string{services.ResourceTypeELBTargetGroup}),
		}
		resources, err := m.rgt.GetResourcesAsList(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			if _, exists := resourceTagsByARN[awssdk.StringValue(resource.ResourceARN)]; !exists {
				resourceTagsByARN[awssdk.StringValue(resource.ResourceARN)] = resource.Tags
			}
		}
	}
	var matchedTGs []TargetGroupWithTags
	for resourceARN, resourceTags := range resourceTagsByARN {
		elbv2Req := &elbv2sdk.DescribeTargetGroupsInput{
			TargetGroupArns: []*string{&resourceARN},
		}
		elbv2Resp, err := m.elbv2Client.DescribeTargetGroupsAsList(ctx, elbv2Req)
		if err != nil {
			return nil, err
		}
		if len(elbv2Resp) == 0 {
			return nil, errors.Errorf("no target group found for the arn: %v", resourceARN)
		}
		matchedTGs = append(matchedTGs, TargetGroupWithTags{
			TargetGroup: elbv2Resp[0],
			Tags:        services.ParseRGTTags(resourceTags),
		})
	}
	return matchedTGs, nil
}

func (m *defaultTaggingManager) listTargetGroupsNative(ctx context.Context, tagFilters []tracking.TagFilter) ([]TargetGroupWithTags, error) {
	req := &elbv2sdk.DescribeTargetGroupsInput{}
	tgs, err := m.elbv2Client.DescribeTargetGroupsAsList(ctx, req)
	if err != nil {
		return nil, err
	}

	tgARNsWithinVPC := make([]string, 0, len(tgs))
	tgByARNWithinVPC := make(map[string]*elbv2sdk.TargetGroup, len(tgs))
	for _, tg := range tgs {
		if awssdk.StringValue(tg.VpcId) != m.vpcID {
			continue
		}
		tgARN := awssdk.StringValue(tg.TargetGroupArn)
		tgARNsWithinVPC = append(tgARNsWithinVPC, tgARN)
		tgByARNWithinVPC[tgARN] = tg
	}
	tagsByARN, err := m.describeResourceTags(ctx, tgARNsWithinVPC)
	if err != nil {
		return nil, err
	}

	var matchedTGs []TargetGroupWithTags
	for _, arn := range tgARNsWithinVPC {
		tags := tagsByARN[arn]
		matchedAnyTagFilter := false
		for _, tagFilter := range tagFilters {
			if tagFilter.Matches(tags) {
				matchedAnyTagFilter = true
				break
			}
		}
		if matchedAnyTagFilter {
			matchedTGs = append(matchedTGs, TargetGroupWithTags{
				TargetGroup: tgByARNWithinVPC[arn],
				Tags:        tags,
			})
		}
	}
	return matchedTGs, nil
}

func (m *defaultTaggingManager) describeResourceTags(ctx context.Context, arns []string) (map[string]map[string]string, error) {
	m.resourceTagsCacheMutex.Lock()
	defer m.resourceTagsCacheMutex.Unlock()

	tagsByARN := make(map[string]map[string]string, len(arns))
	var arnsWithoutTagsCache []string
	for _, arn := range arns {
		if rawTagsCacheItem, exists := m.resourceTagsCache.Get(arn); exists {
			tagsCacheItem := rawTagsCacheItem.(map[string]string)
			tagsByARN[arn] = tagsCacheItem
		} else {
			arnsWithoutTagsCache = append(arnsWithoutTagsCache, arn)
		}
	}
	tagsByARNFromAWS, err := m.describeResourceTagsFromAWS(ctx, arnsWithoutTagsCache)
	if err != nil {
		return nil, err
	}
	for arn, tags := range tagsByARNFromAWS {
		m.resourceTagsCache.Set(arn, tags, m.resourceTagsCacheTTL)
		tagsByARN[arn] = tags
	}
	return tagsByARN, nil
}

// describeResourceTagsFromAWS describes tags for elbv2 resources.
// returns tags indexed by resource ARN.
func (m *defaultTaggingManager) describeResourceTagsFromAWS(ctx context.Context, arns []string) (map[string]map[string]string, error) {
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

func (m *defaultTaggingManager) invalidateResourceTagsCache(arn string) {
	m.resourceTagsCacheMutex.Lock()
	defer m.resourceTagsCacheMutex.Unlock()

	m.resourceTagsCache.Delete(arn)
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

// convert tagFilters to RGTTagFilters
func convertTagFiltersToRGTTagFilters(tagFilter tracking.TagFilter) []*resourcegroupstaggingapi.TagFilter {
	var RGTTagFilters []*resourcegroupstaggingapi.TagFilter
	for k, v := range tagFilter {
		RGTTagFilters = append(RGTTagFilters, &resourcegroupstaggingapi.TagFilter{
			Key:    awssdk.String(k),
			Values: awssdk.StringSlice(v),
		})
	}
	return RGTTagFilters
}
