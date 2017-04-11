package awsutil

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/acm/acmiface"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

// ACM is our extension to AWS's ACM.acm
type ACM struct {
	Svc acmiface.ACMAPI
}

// NewACM returns an ACM based off of the provided aws.Config
func NewACM(awsconfig *aws.Config) *ACM {
	awsSession, err := session.NewSession(awsconfig)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ACM", "request": "NewSession"}).Add(float64(1))
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	awsSession.Handlers.Send.PushFront(func(r *request.Request) {
		AWSRequest.With(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name}).Add(float64(1))
		if AWSDebug {
			glog.Infof("Request: %s/%s, Payload: %s", r.ClientInfo.ServiceName, r.Operation, r.Params)
		}
	})

	elbClient := ACM{
		acm.New(awsSession),
	}
	return &elbClient
}

// CertExists checks whether the provided ARN existing in AWS.
func (a *ACM) CertExists(arn *string) bool {
	if _, err := a.Svc.DescribeCertificate(&acm.DescribeCertificateInput{CertificateArn: arn}); err != nil {
		return false
	}
	return true
}
