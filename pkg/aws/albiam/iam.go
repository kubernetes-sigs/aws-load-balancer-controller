package albiam

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
)

// IAMsvc is a pointer to the awsutil IAM service
var IAMsvc *IAM

// IAM is our extension to AWS's IAM.iam
type IAM struct {
	iamiface.IAMAPI
}

// NewIAM returns an IAM based off of the provided aws.Config
func NewIAM(awsSession *session.Session) {
	IAMsvc = &IAM{
		iam.New(awsSession),
	}
}

// CertExists checks whether the provided ARN exists in AWS.
func (i *IAM) CertExists(arn *string) bool {
	arnString := *arn
	certificateName := arnString[strings.LastIndex(arnString, "/")+1 : len(arnString)]

	params := &iam.GetServerCertificateInput{ServerCertificateName: aws.String(certificateName)}

	if _, err := i.GetServerCertificate(params); err != nil {
		return false
	}
	return true
}

// Status validates IAM connectivity
func (i *IAM) Status() func() error {
	return func() error {
		in := &iam.ListServerCertificatesInput{}
		in.SetMaxItems(1)

		if _, err := i.ListServerCertificates(in); err != nil {
			return err
		}
		return nil
	}
}
