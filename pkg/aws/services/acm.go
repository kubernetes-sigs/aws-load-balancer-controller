package services

import (
	"context"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/acm/acmiface"
)

type ACM interface {
	acmiface.ACMAPI

	// wrapper to ListCertificatesPagesWithContext API, which aggregates paged results into list.
	ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]*acm.CertificateSummary, error)
}

// NewACM constructs new ACM implementation.
func NewACM(session *session.Session) *defaultACM {
	return &defaultACM{
		ACMAPI: acm.New(session),
	}
}

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
