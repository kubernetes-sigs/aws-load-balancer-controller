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
	ImportCertificate(context.Context, *acm.ImportCertificateInput) (string, error)
	DeleteCertificate(context.Context, string) error
	ListTagsForCertificate(context.Context, string) (map[string]string, error)
	AddTagsToCertificate(context.Context, string, map[string]string) error

	// StatusACM validates ACM connectivity
	StatusACM() func() error

	// ACMAvailable whether ACM service is available
	ACMAvailable() bool
}

func (c *Cloud) ImportCertificate(ctx context.Context, input *acm.ImportCertificateInput) (string, error) {
	resp, err := c.acm.ImportCertificateWithContext(ctx, input)
	if err != nil {
		return "", err
	}
	return aws.StringValue(resp.CertificateArn), nil
}

func (c *Cloud) DeleteCertificate(ctx context.Context, certArn string) error {
	_, err := c.acm.DeleteCertificateWithContext(ctx, &acm.DeleteCertificateInput{
		CertificateArn: aws.String(certArn),
	})
	return err
}

func (c *Cloud) ListTagsForCertificate(ctx context.Context, certArn string) (map[string]string, error) {
	resp, err := c.acm.ListTagsForCertificateWithContext(ctx, &acm.ListTagsForCertificateInput{
		CertificateArn: aws.String(certArn),
	})
	if err != nil {
		return nil, err
	}
	tags := make(map[string]string, len(resp.Tags))
	for _, tag := range resp.Tags {
		tags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
	}
	return tags, nil
}

func (c *Cloud) AddTagsToCertificate(ctx context.Context, certArn string, tags map[string]string) error {
	acmTags := make([]*acm.Tag, 0, len(tags))
	for k, v := range tags {
		acmTags = append(acmTags, &acm.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	_, err := c.acm.AddTagsToCertificateWithContext(ctx, &acm.AddTagsToCertificateInput{
		CertificateArn: aws.String(certArn),
		Tags:           acmTags,
	})
	return err
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
	_, err := resolver.EndpointFor(acm.EndpointsID, c.region)
	return err == nil
}
