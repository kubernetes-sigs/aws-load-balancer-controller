package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
)

// IAMAPI is our wrapper IAM API interface
type IAMAPI interface {
	// StatusIAM validates IAM  connectivity
	StatusIAM() func() error
}

// Status validates IAM connectivity
func (c *Cloud) StatusIAM() func() error {
	return func() error {
		in := &iam.ListServerCertificatesInput{MaxItems: aws.Int64(1)}

		if _, err := c.iam.ListServerCertificatesWithContext(context.TODO(), in); err != nil {
			return fmt.Errorf("[iam.ListServerCertificatesWithContext]: %v", err)
		}
		return nil
	}
}
