package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
)

type ACM interface {
	// wrapper to ListCertificatesPagesWithContext API, which aggregates paged results into list.
	ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]types.CertificateSummary, error)
	DescribeCertificateWithContext(ctx context.Context, req *acm.DescribeCertificateInput) (*acm.DescribeCertificateOutput, error)
}

// NewACM constructs new ACM implementation.
func NewACM(cfg aws.Config) ACM {
	return &acmClient{
		acmClient: acm.NewFromConfig(cfg),
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
