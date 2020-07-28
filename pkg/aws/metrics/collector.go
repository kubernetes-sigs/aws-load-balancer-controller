package metrics

import (
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/prometheus/client_golang/prometheus"
	"strconv"
	"time"
)

const (
	sdkHandlerCollectAPICallMetric    = "collectAPICallMetric"
	sdkHandlerCollectAPIRequestMetric = "collectAPIRequestMetric"
)

type collector struct {
	instruments *instruments
}

func NewCollector(registerer prometheus.Registerer) (*collector, error) {
	instruments, err := newInstruments(registerer)
	if err != nil {
		return nil, err
	}
	return &collector{
		instruments: instruments,
	}, nil
}

func (c *collector) InjectHandlers(handlers *request.Handlers) {
	handlers.CompleteAttempt.PushFrontNamed(request.NamedHandler{
		Name: sdkHandlerCollectAPIRequestMetric,
		Fn:   c.collectAPIRequestMetric,
	})
	handlers.Complete.PushFrontNamed(request.NamedHandler{
		Name: sdkHandlerCollectAPICallMetric,
		Fn:   c.collectAPICallMetric,
	})
}

func (c *collector) collectAPIRequestMetric(r *request.Request) {
	service := r.ClientInfo.ServiceID
	operation := r.Operation.Name
	statusCode := statusCodeForRequest(r)
	errorCode := errorCodeForRequest(r)
	duration := time.Since(r.AttemptTime)

	c.instruments.apiRequestsTotal.With(map[string]string{
		labelService:    service,
		labelOperation:  operation,
		labelStatusCode: statusCode,
		labelErrorCode:  errorCode,
	}).Inc()
	c.instruments.apiRequestDurationSecond.With(map[string]string{
		labelService:   service,
		labelOperation: operation,
	}).Observe(duration.Seconds())
}

func (c *collector) collectAPICallMetric(r *request.Request) {
	service := r.ClientInfo.ServiceID
	operation := r.Operation.Name
	statusCode := statusCodeForRequest(r)
	errorCode := errorCodeForRequest(r)
	duration := time.Since(r.Time)

	c.instruments.apiCallsTotal.With(map[string]string{
		labelService:    service,
		labelOperation:  operation,
		labelStatusCode: statusCode,
		labelErrorCode:  errorCode,
	}).Inc()
	c.instruments.apiCallDurationSeconds.With(map[string]string{
		labelService:   service,
		labelOperation: operation,
	}).Observe(duration.Seconds())
	c.instruments.apiCallRetries.With(map[string]string{
		labelService:   service,
		labelOperation: operation,
	}).Observe(float64(r.RetryCount))
}

// statusCodeForRequest returns the http status code for request.
// if there is no http response, returns "0".
func statusCodeForRequest(r *request.Request) string {
	if r.HTTPResponse != nil {
		return strconv.Itoa(r.HTTPResponse.StatusCode)
	}
	return "0"
}

// errorCodeForRequest returns the error code for request.
// if no error happened, returns "".
func errorCodeForRequest(r *request.Request) string {
	if r.Error != nil {
		if awserr, ok := r.Error.(awserr.Error); ok {
			return awserr.Code()
		}
		return "internal"
	}
	return ""
}

// operationForRequest returns the operation for request.
func operationForRequest(r *request.Request) string {
	if r.Operation != nil {
		return r.Operation.Name
	}
	return "?"
}
