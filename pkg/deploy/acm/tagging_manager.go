package acm

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/acm"

	"k8s.io/apimachinery/pkg/util/cache"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	acmsdk "github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	rgttypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
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

type CertificateWithTags struct {
	Certificate *acmtypes.CertificateSummary
	Tags        map[string]string
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

// abstraction around tagging operations for ACM.
type TaggingManager interface {
	// ReconcileTags will reconcile tags on resources.
	ReconcileTags(ctx context.Context, arn string, desiredTags map[string]string, opts ...ReconcileTagsOption) error

	// ListCertificates returns Certificates along with tags
	ListCertificates(ctx context.Context, tagFilters ...tracking.TagFilter) ([]CertificateWithTags, error)
}

// NewDefaultTaggingManager constructs default TaggingManager.
func NewDefaultTaggingManager(acmClient services.ACM, featureGates config.FeatureGates, logger logr.Logger) *defaultTaggingManager {
	return &defaultTaggingManager{
		acmClient:             acmClient,
		featureGates:          featureGates,
		logger:                logger,
		describeTagsChunkSize: defaultDescribeTagsChunkSize,
		resourceTagsCache:     cache.NewExpiring(),
		resourceTagsCacheTTL:  defaultResourceTagsCacheTTL,
	}
}

var _ TaggingManager = &defaultTaggingManager{}

// default implementation for TaggingManager
type defaultTaggingManager struct {
	acmClient             services.ACM
	featureGates          config.FeatureGates
	logger                logr.Logger
	describeTagsChunkSize int
	// cache for tags on ACM resources.
	resourceTagsCache      *cache.Expiring
	resourceTagsCacheTTL   time.Duration
	resourceTagsCacheMutex sync.RWMutex
}

func (m *defaultTaggingManager) ReconcileTags(ctx context.Context, arn string, desiredTags map[string]string, opts ...ReconcileTagsOption) error {
	// reconcileOpts := ReconcileTagsOptions{
	// 	CurrentTags:    nil,
	// 	IgnoredTagKeys: nil,
	// }
	// reconcileOpts.ApplyOptions(opts)
	// currentTags := reconcileOpts.CurrentTags
	// if currentTags == nil {
	// 	tagsByARN, err := m.describeCerificateTags(ctx, []string{arn})
	// 	if err != nil {
	// 		return err
	// 	}
	// 	currentTags = tagsByARN[arn]
	// }
	//
	// tagsToUpdate, tagsToRemove := algorithm.DiffStringMap(desiredTags, currentTags)
	// for _, ignoredTagKey := range reconcileOpts.IgnoredTagKeys {
	// 	delete(tagsToUpdate, ignoredTagKey)
	// 	delete(tagsToRemove, ignoredTagKey)
	// }
	//
	// if len(tagsToUpdate) > 0 {
	// 	req := &elbv2sdk.AddTagsInput{
	// 		ResourceArns: []string{arn},
	// 		Tags:         convertTagsToSDKTags(tagsToUpdate),
	// 	}
	//
	// 	m.logger.Info("adding resource tags",
	// 		"arn", arn,
	// 		"change", tagsToUpdate)
	// 	if _, err := m.acmClient.AddTagsWithContext(ctx, req); err != nil {
	// 		return err
	// 	}
	// 	m.invalidateResourceTagsCache(arn)
	// 	m.logger.Info("added resource tags",
	// 		"arn", arn)
	// }
	//
	// if len(tagsToRemove) > 0 {
	// 	tagKeys := sets.StringKeySet(tagsToRemove).List()
	// 	req := &elbv2sdk.RemoveTagsInput{
	// 		ResourceArns: []string{arn},
	// 		TagKeys:      tagKeys,
	// 	}
	//
	// 	m.logger.Info("removing resource tags",
	// 		"arn", arn,
	// 		"change", tagKeys)
	// 	if _, err := m.acmClient.RemoveTagsWithContext(ctx, req); err != nil {
	// 		return err
	// 	}
	// 	m.invalidateResourceTagsCache(arn)
	// 	m.logger.Info("removed resource tags",
	// 		"arn", arn)
	// }
	return nil
}

func (m *defaultTaggingManager) ListCertificates(ctx context.Context, tagFilters ...tracking.TagFilter) ([]CertificateWithTags, error) {
	req := &acmsdk.ListCertificatesInput{}                            // no option to add filters directly
	certificates, err := m.acmClient.ListCertificatesAsList(ctx, req) // this will lookup all certs there are
	if err != nil {
		return nil, err
	}

	certARNs := make([]string, 0, len(certificates))
	certByARN := make(map[string]*acmtypes.CertificateSummary, len(certificates))
	for _, cert := range certificates {
		certARN := awssdk.ToString(cert.CertificateArn)
		certARNs = append(certARNs, certARN)
		certByARN[certARN] = &cert
	}

	certificateTagsByARN, err := m.describeCertificateTags(ctx, certARNs)
	if err != nil {
		return nil, err
	}

	var sdkCerts []CertificateWithTags
	for _, filter := range tagFilters {
		for arn, tags := range certificateTagsByARN {
			if filter.Matches(tags) {
				sdkCerts = append(sdkCerts, CertificateWithTags{
					Certificate: certByARN[arn],
					Tags:        certificateTagsByARN[arn],
				})
			}
		}
	}

	return sdkCerts, nil
}

// gets a list of certificate ARNs and returns a map of tags for each certificate
func (m *defaultTaggingManager) describeCertificateTags(ctx context.Context, arns []string) (map[string]map[string]string, error) {
	// TODO: implement a resourcesTagsCache to avoid too many bulk-api calls
	// see m.describeResourceTags for an example of such a caching option
	tagsByARN := make(map[string]map[string]string, len(arns))
	for _, arn := range arns {
		input := &acm.ListTagsForCertificateInput{
			CertificateArn: awssdk.String(arn),
		}
		tagsByARNFromAWS, err := m.acmClient.ListTagsForCertificate(ctx, input)
		if err != nil {
			return nil, err
		}

		tags := convertTagsFromSDKTags(tagsByARNFromAWS.Tags)
		tagsByARN[arn] = tags
	}

	return tagsByARN, nil
}

// func (m *defaultTaggingManager) describeResourceTags(ctx context.Context, arns []string) (map[string]map[string]string, error) {
// 	m.resourceTagsCacheMutex.Lock()
// 	defer m.resourceTagsCacheMutex.Unlock()
//
// 	tagsByARN := make(map[string]map[string]string, len(arns))
// 	var arnsWithoutTagsCache []string
// 	for _, arn := range arns {
// 		if rawTagsCacheItem, exists := m.resourceTagsCache.Get(arn); exists {
// 			tagsCacheItem := rawTagsCacheItem.(map[string]string)
// 			tagsByARN[arn] = tagsCacheItem
// 		} else {
// 			arnsWithoutTagsCache = append(arnsWithoutTagsCache, arn)
// 		}
// 	}
// 	tagsByARNFromAWS, err := m.describeResourceTagsFromAWS(ctx, arnsWithoutTagsCache)
// 	if err != nil {
// 		return nil, err
// 	}
// 	for arn, tags := range tagsByARNFromAWS {
// 		m.resourceTagsCache.Set(arn, tags, m.resourceTagsCacheTTL)
// 		tagsByARN[arn] = tags
// 	}
// 	return tagsByARN, nil
// }

// describeResourceTagsFromAWS describes tags for elbv2 resources.
// returns tags indexed by resource ARN.
func (m *defaultTaggingManager) describeResourceTagsFromAWS(ctx context.Context, arns []string) (map[string]map[string]string, error) {
	tagsByARN := make(map[string]map[string]string, len(arns))
	// arnsChunks := algorithm.ChunkStrings(arns, m.describeTagsChunkSize)
	// for _, arnsChunk := range arnsChunks {
	// 	req := &elbv2sdk.DescribeTagsInput{
	// 		ResourceArns: arnsChunk,
	// 	}
	// 	resp, err := m.elbv2Client.DescribeTagsWithContext(ctx, req)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	for _, tagDescription := range resp.TagDescriptions {
	// 		tagsByARN[awssdk.ToString(tagDescription.ResourceArn)] = convertSDKTagsToTags(tagDescription.Tags)
	// 	}
	// }
	return tagsByARN, nil
}

func (m *defaultTaggingManager) invalidateResourceTagsCache(arn string) {
	m.resourceTagsCacheMutex.Lock()
	defer m.resourceTagsCacheMutex.Unlock()

	m.resourceTagsCache.Delete(arn)
}

// convert tagFilters to RGTTagFilters
func convertTagFiltersToRGTTagFilters(tagFilter tracking.TagFilter) []rgttypes.TagFilter {
	var RGTTagFilters []rgttypes.TagFilter
	for k, v := range tagFilter {
		RGTTagFilters = append(RGTTagFilters, rgttypes.TagFilter{
			Key:    awssdk.String(k),
			Values: v,
		})
	}
	return RGTTagFilters
}

// convert tags into AWS SDK tag presentation.
func convertTagsToSDKTags(tags map[string]string) []acmtypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	sdkTags := make([]acmtypes.Tag, 0, len(tags))

	for _, key := range sets.StringKeySet(tags).List() {
		sdkTags = append(sdkTags, acmtypes.Tag{
			Key:   awssdk.String(key),
			Value: awssdk.String(tags[key]),
		})
	}
	return sdkTags
}

// convert AWS SDK tag presentation into tags.
func convertTagsFromSDKTags(sdkTags []acmtypes.Tag) map[string]string {
	if len(sdkTags) == 0 {
		return nil
	}
	tags := make(map[string]string, len(sdkTags))

	for _, tag := range sdkTags {
		tags[awssdk.ToString(tag.Key)] = awssdk.ToString(tag.Value)
	}
	return tags
}
