package albsession

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	albprom "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/prometheus"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

// NewSession returns an AWS session based off of the provided AWS config
func NewSession(awsconfig *aws.Config, AWSDebug bool) *session.Session {
	session, err := session.NewSession(awsconfig)
	if err != nil {
		albprom.AWSErrorCount.With(prometheus.Labels{"service": "AWS", "request": "NewSession"}).Add(float64(1))
		glog.ErrorDepth(4, fmt.Sprintf("Failed to create AWS session: %s", err.Error()))
		return nil
	}

	session.Handlers.Retry.PushFront(func(r *request.Request) {
		albprom.AWSRetry.With(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name}).Add(float64(1))
	})

	session.Handlers.Send.PushFront(func(r *request.Request) {
		albprom.AWSRequest.With(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name}).Add(float64(1))
		if AWSDebug {
			glog.InfoDepth(4, fmt.Sprintf("Request: %s/%s, Payload: %s", r.ClientInfo.ServiceName, r.Operation.Name, log.Prettify(r.Params)))
		}
	})

	session.Handlers.Complete.PushFront(func(r *request.Request) {
		if r.Error != nil {
			albprom.AWSErrorCount.With(
				prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name}).Add(float64(1))
			if AWSDebug {
				glog.ErrorDepth(4, fmt.Sprintf("Failed request: %s/%s, Payload: %s, Error: %s", r.ClientInfo.ServiceName, r.Operation.Name, log.Prettify(r.Params), r.Error))
			}
		}
	})
	return session
}
