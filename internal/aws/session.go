package aws

import (
	"fmt"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	"github.com/prometheus/client_golang/prometheus"
)

// NewSession returns an AWS session based off of the provided AWS config
func NewSession(awsconfig *aws.Config, AWSDebug bool, mc metric.Collector) *session.Session {
	session, err := session.NewSession(awsconfig)
	if err != nil {
		mc.IncAPIErrorCount(prometheus.Labels{"service": "AWS", "request": "NewSession"})
		glog.ErrorDepth(4, fmt.Sprintf("Failed to create AWS session: %s", err.Error()))
		return nil
	}

	session.Handlers.Retry.PushFront(func(r *request.Request) {
		mc.IncAPIRetryCount(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name})
	})

	session.Handlers.Send.PushFront(func(r *request.Request) {
		mc.IncAPIRequestCount(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name})
		if AWSDebug {
			glog.InfoDepth(4, fmt.Sprintf("Request: %s/%s, Payload: %s", r.ClientInfo.ServiceName, r.Operation.Name, log.Prettify(r.Params)))
		}
	})

	notFoundRegex := regexp.MustCompile("^[A-za-z]+NotFound")
	session.Handlers.Complete.PushFront(func(r *request.Request) {
		if r.Error != nil {
			if value, ok := r.Context().Value("report-not-found-error").(bool); !notFoundRegex.MatchString(r.Error.Error()) && !ok || value {
				mc.IncAPIErrorCount(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name})
				if AWSDebug {
					glog.ErrorDepth(4, fmt.Sprintf("Failed request: %s/%s, Payload: %s, Error: %s", r.ClientInfo.ServiceName, r.Operation.Name, log.Prettify(r.Params), r.Error))
				}
			}
		} else {
			if AWSDebug {
				glog.InfoDepth(4, fmt.Sprintf("Response: %s/%s, Body: %s", r.ClientInfo.ServiceName, r.Operation.Name, log.Prettify(r.Data)))
			}
		}
	})
	return session
}
