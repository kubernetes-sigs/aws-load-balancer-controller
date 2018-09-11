package albacm

import "github.com/aws/aws-sdk-go/service/acm"

type Dummy struct {
	ACMWithStatus

	Result    []*acm.CertificateSummary
	ErrResult error
}

func (m *Dummy) ListCertificates(*acm.ListCertificatesInput) (*acm.ListCertificatesOutput, error) {
	if m.ErrResult != nil {
		return nil, m.ErrResult
	}
	return &acm.ListCertificatesOutput{CertificateSummaryList: m.Result}, nil
}

func NewDummy() ACMWithStatus {
	return &Dummy{}
}
