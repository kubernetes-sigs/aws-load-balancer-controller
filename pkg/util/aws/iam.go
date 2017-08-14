package aws

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
)

// IAM is our extension to AWS's IAM.iam
type IAM struct {
	iamiface.IAMAPI
}

// NewIAM returns an IAM based off of the provided aws.Config
func NewIAM(awsSession *session.Session) *IAM {
	iamClient := IAM{
		iam.New(awsSession),
	}
	return &iamClient
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
