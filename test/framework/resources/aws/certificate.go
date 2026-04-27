package aws

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
)

// CertificateManager is responsible for Certificate resources.
type CertificateManager interface {
	FindCertificateByHostnames(ctx context.Context, hosts []string, tagFilters ...tracking.TagFilter) (string, error)
	GetCertificateDetail(ctx context.Context, certARN string) (*types.CertificateDetail, error)
}

// NewDefaultCertificateManager constructs new defaultLoadBalancerManager.
func NewDefaultCertificateManager(acmClient services.ACM, logger logr.Logger) *defaultCertificateManager {
	return &defaultCertificateManager{
		acmClient: acmClient,
		logger:    logger,
	}
}

var _ CertificateManager = &defaultCertificateManager{}

// default implementation for LoadBalancerManager
type defaultCertificateManager struct {
	acmClient services.ACM
	logger    logr.Logger
}

func (c *defaultCertificateManager) FindCertificateByHostnames(ctx context.Context, hosts []string, tagFilters ...tracking.TagFilter) (string, error) {
	req := &acm.ListCertificatesInput{}
	certs, err := c.acmClient.ListCertificatesAsList(ctx, req)
	if err != nil {
		return "", err
	}

	// we return the first certificate that matches
	for _, cert := range certs {
		certHosts := sets.NewString(cert.SubjectAlternativeNameSummaries...)
		hosts := sets.NewString(hosts...) // first requirement, only get tags if hostnames match
		if certHosts.Equal(hosts) {
			tags, err := c.acmClient.ListTagsForCertificate(ctx, &acm.ListTagsForCertificateInput{CertificateArn: cert.CertificateArn})
			if err != nil {
				return "", err
			}
			tagsMap := convertTagsFromSDKTags(tags.Tags)
			for _, filter := range tagFilters {
				if filter.Matches(tagsMap) { // second requirement, matching the tagFilters provided
					return awssdk.ToString(cert.CertificateArn), nil
				}
			}

		}
	}
	return "", errors.Errorf("couldn't find certificate with matching hostnames: %v", hosts)
}

func (c *defaultCertificateManager) GetCertificateDetail(ctx context.Context, certARN string) (*types.CertificateDetail, error) {
	req := &acm.DescribeCertificateInput{CertificateArn: awssdk.String(certARN)}
	desc, err := c.acmClient.DescribeCertificateWithContext(ctx, req)
	if err != nil {
		return &types.CertificateDetail{}, err
	}

	return desc.Certificate, nil
}

// convert AWS SDK tag presentation into string map
func convertTagsFromSDKTags(sdkTags []types.Tag) map[string]string {
	if len(sdkTags) == 0 {
		return nil
	}
	tags := make(map[string]string, len(sdkTags))

	for _, tag := range sdkTags {
		tags[awssdk.ToString(tag.Key)] = awssdk.ToString(tag.Value)
	}
	return tags
}
