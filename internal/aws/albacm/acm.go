package albacm

import (
	"time"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/acm/acmiface"
	"github.com/prometheus/client_golang/prometheus"
)

// ACMsvc is a pointer to the awsutil ACM service
var ACMsvc *ACM

// ACM is our extension to AWS's ACM.acm
type ACM struct {
	acmiface.ACMAPI
	mc metric.Collector
}

// NewACM sets ACMsvc based off of the provided AWS session
func NewACM(awsSession *session.Session, mc metric.Collector) {
	ACMsvc = &ACM{
		ACMAPI: acm.New(awsSession),
		mc:     mc,
	}
	if ACMsvc.mc == nil {
		// prevent nil pointer panic
		ACMsvc.mc = metric.DummyCollector{}
	}
}

// Status validates ACM connectivity
func (a *ACM) Status() func() error {
	return func() error {
		in := &acm.ListCertificatesInput{}
		in.SetMaxItems(1)

		start := time.Now()
		if _, err := a.ListCertificates(in); err != nil {
			return err
		}
		a.mc.ObserveAPIRequest(prometheus.Labels{"operation": "ListCertificates"}, start)
		return nil
	}
}
