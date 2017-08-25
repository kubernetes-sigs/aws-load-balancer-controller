package acm

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/acm/acmiface"
)

// ACMsvc is a pointer to the awsutil ACM service
var ACMsvc *ACM

// ACM is our extension to AWS's ACM.acm
type ACM struct {
	acmiface.ACMAPI
}

// NewACM sets ACMsvc based off of the provided AWS session
func NewACM(awsSession *session.Session) {
	ACMsvc = &ACM{
		acm.New(awsSession),
	}
}

// CertExists checks whether the provided ARN existing in AWS.
func (a *ACM) CertExists(arn *string) bool {
	if _, err := a.DescribeCertificate(&acm.DescribeCertificateInput{CertificateArn: arn}); err != nil {
		return false
	}
	return true
}
