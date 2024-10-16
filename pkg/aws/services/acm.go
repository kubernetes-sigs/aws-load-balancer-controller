package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
)

type ACM interface {
	// wrapper to ListCertificatesPagesWithContext API, which aggregates paged results into list.
	ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]types.CertificateSummary, error)
	DescribeCertificateWithContext(ctx context.Context, req *acm.DescribeCertificateInput) (*acm.DescribeCertificateOutput, error)
}

// NewACM constructs new ACM implementation.
func NewACM(awsClientsProvider provider.AWSClientsProvider) ACM {
	return &acmClient{
		awsClientsProvider: awsClientsProvider,
	}
}

type acmClient struct {
	awsClientsProvider provider.AWSClientsProvider
}

func (c *acmClient) ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]types.CertificateSummary, error) {
	var result []types.CertificateSummary
	client, err := c.awsClientsProvider.GetACMClient(ctx, "ListCertificates")
	if err != nil {
		return nil, err
	}
	paginator := acm.NewListCertificatesPaginator(client, input)
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
	client, err := c.awsClientsProvider.GetACMClient(ctx, "DescribeCertificate")
	if err != nil {
		return nil, err
	}
	return client.DescribeCertificate(ctx, input)
}
