package awsutil

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
)

// ACM is our extension to AWS's ACM.acm
type IAM struct {
	Svc iamiface.IAMAPI
}

// NewIAM returns an IAM based off of the provided aws.Config
func NewIAM(awsSession *session.Session) *IAM {
	iamClient := IAM{
		iam.New(awsSession),
	}
	return &iamClient
}

// CertExists checks whether the provided ARN existing in AWS.
func (i *IAM) CertExists(arn *string) bool {
	arn_string := *arn
	certificate_name := arn_string[strings.LastIndex(arn_string, "/")+1 : len(arn_string)]

	params := &iam.GetServerCertificateInput{ServerCertificateName: aws.String(certificate_name)}

	if _, err := i.Svc.GetServerCertificate(params); err != nil {
		return false
	}
	return true
}
