package aws

import (
	"fmt"

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
		in := &acm.ListCertificatesInput{}
		in.SetMaxItems(1)

		if _, err := c.acm.ListCertificates(in); err != nil {
			return fmt.Errorf("[acm.ListCertificates]: %v", err)
		}
		return nil
	}
}
