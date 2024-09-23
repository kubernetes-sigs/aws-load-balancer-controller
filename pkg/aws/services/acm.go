package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
)

type ACM interface {
	// wrapper to ListCertificatesPagesWithContext API, which aggregates paged results into list.
	ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]types.CertificateSummary, error)
	DescribeCertificateWithContext(ctx context.Context, req *acm.DescribeCertificateInput) (*acm.DescribeCertificateOutput, error)
}

// NewACM constructs new ACM implementation.
func NewACM(cfg aws.Config, endpointsResolver *endpoints.Resolver) ACM {
	customEndpoint := endpointsResolver.EndpointFor(acm.ServiceID)
	return &acmClient{
		acmClient: acm.NewFromConfig(cfg, func(o *acm.Options) {
			if customEndpoint != nil {
				o.BaseEndpoint = customEndpoint
			}
		}),
	}
}

type acmClient struct {
	acmClient *acm.Client
}

func (c *acmClient) ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]types.CertificateSummary, error) {
	var result []types.CertificateSummary
	paginator := acm.NewListCertificatesPaginator(c.acmClient, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.CertificateSummaryList...)
	}
	return result, nil
}

func (c *acmClient) DescribeCertificateWithContext(ctx context.Context, input *acm.DescribeCertificateInput) (*acm.DescribeCertificateOutput, error) {
	return c.acmClient.DescribeCertificate(ctx, input)
}
