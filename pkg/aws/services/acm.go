package services

import (
	"context"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
)

type ACM interface {
	// wrapper to ListCertificatesPagesWithContext API, which aggregates paged results into list.
	ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]types.CertificateSummary, error)
	DescribeCertificateWithContext(ctx context.Context, req *acm.DescribeCertificateInput) (*acm.DescribeCertificateOutput, error)
	ListTagsForCertificate(ctx context.Context, input *acm.ListTagsForCertificateInput) (*acm.ListTagsForCertificateOutput, error)
	RequestCertificateWithContext(ctx context.Context, input *acm.RequestCertificateInput) (*acm.RequestCertificateOutput, error)
	DeleteCertificateWithContext(ctx context.Context, input *acm.DeleteCertificateInput) (*acm.DeleteCertificateOutput, error)
	WaitForCertificateIssuedWithContext(ctx context.Context, arn string, waitTime time.Duration) error
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

func (c *acmClient) ListTagsForCertificate(ctx context.Context, input *acm.ListTagsForCertificateInput) (*acm.ListTagsForCertificateOutput, error) {
	client, err := c.awsClientsProvider.GetACMClient(ctx, "ListTagsForCertificate")
	if err != nil {
		return &acm.ListTagsForCertificateOutput{}, err
	}

	resp, err := client.ListTagsForCertificate(ctx, input)
	if err != nil {
		return &acm.ListTagsForCertificateOutput{}, err
	}

	return resp, nil
}

func (c *acmClient) DescribeCertificateWithContext(ctx context.Context, input *acm.DescribeCertificateInput) (*acm.DescribeCertificateOutput, error) {
	client, err := c.awsClientsProvider.GetACMClient(ctx, "DescribeCertificate")
	if err != nil {
		return nil, err
	}
	return client.DescribeCertificate(ctx, input)
}

func (c *acmClient) RequestCertificateWithContext(ctx context.Context, req *acm.RequestCertificateInput) (*acm.RequestCertificateOutput, error) {
	client, err := c.awsClientsProvider.GetACMClient(ctx, "RequestCertificate")
	if err != nil {
		return nil, err
	}

	resp, err := client.RequestCertificate(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *acmClient) WaitForCertificateIssuedWithContext(ctx context.Context, arn string, waitTime time.Duration) error {
	client, err := c.awsClientsProvider.GetACMClient(ctx, "WaitForCertificate")
	if err != nil {
		return err
	}

	waiter := acm.NewCertificateValidatedWaiter(client)
	req := &acm.DescribeCertificateInput{
		CertificateArn: awssdk.String(arn),
	}
	err = waiter.Wait(ctx, req, waitTime)
	if err != nil {
		return err
	}

	return nil
}

func (c *acmClient) DeleteCertificateWithContext(ctx context.Context, req *acm.DeleteCertificateInput) (*acm.DeleteCertificateOutput, error) {
	client, err := c.awsClientsProvider.GetACMClient(ctx, "DeleteCertificate")
	if err != nil {
		return nil, err
	}

	resp, err := client.DeleteCertificate(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
