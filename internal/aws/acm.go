package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws/endpoints"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
)

// ACMAPI is our wrapper ACM API interface
type ACMAPI interface {
	// StatusACM validates ACM connectivity
	StatusACM() func() error

	// ACMAvailable whether ACM service is available
	ACMAvailable() bool

	// ListCertificates returns a list of certificate objects from ACM
	ListCertificates(ctx context.Context, input *acm.ListCertificatesInput) ([]*acm.CertificateSummary, error)

	// DescribeCertificate is an wrapper around acm.DescribeCertificate
	DescribeCertificate(ctx context.Context, certArn string) (*acm.CertificateDetail, error)
}

// Status validates ACM connectivity
func (c *Cloud) StatusACM() func() error {
	return func() error {
		in := &acm.ListCertificatesInput{
			MaxItems: aws.Int64(1),
		}

		if _, err := c.acm.ListCertificatesWithContext(context.TODO(), in); err != nil {
			return fmt.Errorf("[acm.ListCertificatesWithContext]: %v", err)
		}
		return nil
	}
}

func (c *Cloud) ACMAvailable() bool {
	resolver := endpoints.DefaultResolver()
	_, err := resolver.EndpointFor(acm.EndpointsID, c.region, endpoints.StrictMatchingOption)
	return err == nil
}

// ListCertificates returns a list of certificates from ACM
// Apply a filter to the query using the status parameter
func (c *Cloud) ListCertificates(ctx context.Context, input *acm.ListCertificatesInput) ([]*acm.CertificateSummary, error) {
	var certSummaries []*acm.CertificateSummary
	if err := c.acm.ListCertificatesPagesWithContext(ctx, input, func(output *acm.ListCertificatesOutput, _ bool) bool {
		certSummaries = append(certSummaries, output.CertificateSummaryList...)
		return true
	}); err != nil {
		return nil, err
	}

	return certSummaries, nil
}

func (c *Cloud) DescribeCertificate(ctx context.Context, certArn string) (*acm.CertificateDetail, error) {
	resp, err := c.acm.DescribeCertificateWithContext(ctx, &acm.DescribeCertificateInput{
		CertificateArn: aws.String(certArn),
	})
	if err != nil {
		return nil, err
	}
	return resp.Certificate, nil
}
