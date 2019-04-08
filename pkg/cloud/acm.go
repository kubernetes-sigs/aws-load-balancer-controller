package cloud

import (
	"context"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/acm/acmiface"
)

// ACM is an wrapper around original ACMAPI with additional convenient APIs.
type ACM interface {
	acmiface.ACMAPI

	ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]*acm.CertificateSummary, error)
}

func NewACM(session *session.Session) ACM {
	return &defaultACM{
		acm.New(session),
	}
}

var _ ACM = (*defaultACM)(nil)

type defaultACM struct {
	acmiface.ACMAPI
}

func (c *defaultACM) ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]*acm.CertificateSummary, error) {
	var result []*acm.CertificateSummary
	if err := c.ListCertificatesPagesWithContext(ctx, input, func(output *acm.ListCertificatesOutput, _ bool) bool {
		result = append(result, output.CertificateSummaryList...)
		return true
	}); err != nil {
		return nil, err
	}
	return result, nil
}
