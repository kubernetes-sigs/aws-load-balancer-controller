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
