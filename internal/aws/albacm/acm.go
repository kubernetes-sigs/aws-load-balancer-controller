package albacm

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/acm/acmiface"
)

// ACMsvc is an instance of the current AWS ACM service
var ACMsvc ACMWithStatus

// ACMWithStatus is our extension to AWS's ACM.acm
type ACMWithStatus interface {
	acmiface.ACMAPI

	Status() func() error
}

// ACM is a concrete implementation of ACMWithStatus
type ACM struct {
	*acm.ACM
}

// NewACM sets ACMsvc based off of the provided AWS session
func NewACM(awsSession *session.Session) {
	ACMsvc = &ACM{
		acm.New(awsSession),
	}
}

// Status validates ACM connectivity
func (a *ACM) Status() func() error {
	return func() error {
		in := &acm.ListCertificatesInput{}
		in.SetMaxItems(1)

		if _, err := a.ListCertificates(in); err != nil {
			return err
		}
		return nil
	}
}
