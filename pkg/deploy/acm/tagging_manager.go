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
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
)

const (
	// cache ttl for tags on certificates.
	defaultResourceTagsCacheTTL = 20 * time.Minute
)

type CertificateWithTags struct {
	Certificate *acmtypes.CertificateSummary
	Tags        map[string]string
}

// abstraction around tagging operations for ACM.
type TaggingManager interface {
	// ListCertificates returns Certificates along with tags
	ListCertificates(ctx context.Context, tagFilters ...tracking.TagFilter) ([]CertificateWithTags, error)
}

// NewDefaultTaggingManager constructs default TaggingManager.
func NewDefaultTaggingManager(acmClient services.ACM, featureGates config.FeatureGates, logger logr.Logger) *defaultTaggingManager {
	return &defaultTaggingManager{
		acmClient:            acmClient,
		featureGates:         featureGates,
		logger:               logger,
		resourceTagsCache:    cache.NewExpiring(),
		resourceTagsCacheTTL: defaultResourceTagsCacheTTL,
	}
}

var _ TaggingManager = &defaultTaggingManager{}

// default implementation for TaggingManager
type defaultTaggingManager struct {
	acmClient    services.ACM
	featureGates config.FeatureGates
	logger       logr.Logger
	// cache for tags on certificates.
	resourceTagsCache      *cache.Expiring
	resourceTagsCacheTTL   time.Duration
	resourceTagsCacheMutex sync.RWMutex
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

	tagsByARNFromAWS, err := m.describeCertificateTagsFromAWS(ctx, arnsWithoutTagsCache)
	if err != nil {
		return nil, err
	}

	for arn, tags := range tagsByARNFromAWS {
		m.resourceTagsCache.Set(arn, tags, m.resourceTagsCacheTTL)
		tagsByARN[arn] = tags
	}

	return tagsByARN, nil
}

func (m *defaultTaggingManager) describeCertificateTagsFromAWS(ctx context.Context, arns []string) (map[string]map[string]string, error) {
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

// convert AWS SDK tag presentation into string map
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
