package albiam

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"fmt"
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

// Status validates IAM connectivity
func (i *IAM) Status() func() error {
	return func() error {
		in := &iam.ListServerCertificatesInput{}
		in.SetMaxItems(1)

		if _, err := i.ListServerCertificates(in); err != nil {
			return fmt.Errorf("[iam.ListServerCertificates]: %v", err)
		}
		return nil
	}
}
