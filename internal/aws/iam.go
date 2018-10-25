package aws

import (
	"fmt"

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
		in := &iam.ListServerCertificatesInput{}
		in.SetMaxItems(1)

		if _, err := c.iam.ListServerCertificates(in); err != nil {
			return fmt.Errorf("[iam.ListServerCertificates]: %v", err)
		}
		return nil
	}
}
