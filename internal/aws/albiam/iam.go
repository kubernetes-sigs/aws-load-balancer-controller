package albiam

import (
	"time"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/prometheus/client_golang/prometheus"
)

// IAMsvc is a pointer to the awsutil IAM service
var IAMsvc *IAM

// IAM is our extension to AWS's IAM.iam
type IAM struct {
	iamiface.IAMAPI
	mc metric.Collector
}

// NewIAM returns an IAM based off of the provided aws.Config
func NewIAM(awsSession *session.Session, mc metric.Collector) {
	IAMsvc = &IAM{
		IAMAPI: iam.New(awsSession),
		mc:     mc,
	}
	if IAMsvc.mc == nil {
		// prevent nil pointer panic
		IAMsvc.mc = metric.DummyCollector{}
	}
}

// Status validates IAM connectivity
func (i *IAM) Status() func() error {
	return func() error {
		in := &iam.ListServerCertificatesInput{}
		in.SetMaxItems(1)

		start := time.Now()
		if _, err := i.ListServerCertificates(in); err != nil {
			return err
		}
		i.mc.ObserveAPIRequest(prometheus.Labels{"operation": "ListServerCertificates"}, start)
		return nil
	}
}
