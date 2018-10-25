package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
)

// ACMAPI is our wrapper ACM API interface
type ACMAPI interface {
	// StatusACM validates ACM connectivity
	StatusACM() func() error
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
