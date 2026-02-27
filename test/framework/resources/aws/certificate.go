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
)

// CertificateManager is responsible for Certificate resources.
type CertificateManager interface {
	FindCertificateByHostnames(ctx context.Context, hosts []string) (string, error)
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

func (c *defaultCertificateManager) FindCertificateByHostnames(ctx context.Context, hosts []string) (string, error) {
	req := &acm.ListCertificatesInput{}
	certs, err := c.acmClient.ListCertificatesAsList(ctx, req)
	if err != nil {
		return "", err
	}

	// we return the first certificate that matches
	// ignoring the fact that hypothetically this AWS account could contain another certificate containing the exact same hostnames
	for _, cert := range certs {
		certHosts := sets.NewString(cert.SubjectAlternativeNameSummaries...)
		hosts := sets.NewString(hosts...)
		if certHosts.Equal(hosts) {
			return awssdk.ToString(cert.CertificateArn), nil
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
