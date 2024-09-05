package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
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

// TODO : WIP Migrate metric collection
//func (c *collector) InjectHandlers(cfg aws.Config) {
//	handlers.CompleteAttempt.PushFrontNamed(request.NamedHandler{
//		Name: sdkHandlerCollectAPIRequestMetric,
//		Fn:   c.collectAPIRequestMetric,
//	})
//	handlers.Complete.PushFrontNamed(request.NamedHandler{
//		Name: sdkHandlerCollectAPICallMetric,
//		Fn:   c.collectAPICallMetric,
//	})
//}

//func (c *collector) CollectAPICallMetricMiddleware() func(*smithymiddleware.Stack) error {
//	return func(stack *smithymiddleware.Stack) error {
//		return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("CollectAPICallMetricMiddleware", func(ctx context.Context, input smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (output smithymiddleware.FinalizeOutput, metadata smithymiddleware.Metadata, err error) {
//			start := time.Now()
//			output, metadata, err = next.HandleFinalize(ctx, input)
//			service := awsmiddleware.GetServiceID(ctx)
//			operation := awsmiddleware.GetOperationName(ctx)
//			response, ok := output.Result.(*smithyhttp.Response)
//			if !ok {
//				return ok
//			}
//			statusCode := response.StatusCode
//			errorCode := response
//			duration := time.Since(r.Time)
//
//		}), smithymiddleware.Before)
//		}
//	}
//}
//
//func (c *collector) collectAPIRequestMetric(r *request.Request) {
//	service := r.ClientInfo.ServiceID
//	operation := r.Operation.Name
//	statusCode := statusCodeForRequest(r)
//	errorCode := errorCodeForRequest(r)
//	duration := time.Since(r.AttemptTime)
//
//	c.instruments.apiRequestsTotal.With(map[string]string{
//		labelService:    service,
//		labelOperation:  operation,
//		labelStatusCode: statusCode,
//		labelErrorCode:  errorCode,
//	}).Inc()
//	c.instruments.apiRequestDurationSecond.With(map[string]string{
//		labelService:   service,
//		labelOperation: operation,
//	}).Observe(duration.Seconds())
//}
//
//func (c *collector) collectAPICallMetric(r *request.Request) {
//	service := r.ClientInfo.ServiceID
//	operation := r.Operation.Name
//	statusCode := statusCodeForRequest(r)
//	errorCode := errorCodeForRequest(r)
//	duration := time.Since(r.Time)
//
//	c.instruments.apiCallsTotal.With(map[string]string{
//		labelService:    service,
//		labelOperation:  operation,
//		labelStatusCode: statusCode,
//		labelErrorCode:  errorCode,
//	}).Inc()
//	c.instruments.apiCallDurationSeconds.With(map[string]string{
//		labelService:   service,
//		labelOperation: operation,
//	}).Observe(duration.Seconds())
//	c.instruments.apiCallRetries.With(map[string]string{
//		labelService:   service,
//		labelOperation: operation,
//	}).Observe(float64(r.RetryCount))
//}
//
//// statusCodeForRequest returns the http status code for request.
//// if there is no http response, returns "0".
//func statusCodeForRequest(r *request.Request) string {
//	if r.HTTPResponse != nil {
//		return strconv.Itoa(r.HTTPResponse.StatusCode)
//	}
//	return "0"
//}
//
//// errorCodeForRequest returns the error code for request.
//// if no error happened, returns "".
//func errorCodeForRequest(r *request.Request) string {
//	if r.Error != nil {
//		if awserr, ok := r.Error.(awserr.Error); ok {
//			return awserr.Code()
//		}
//		return "internal"
//	}
//	return ""
//}
//
//// operationForRequest returns the operation for request.
//func operationForRequest(r *request.Request) string {
//	if r.Operation != nil {
//		return r.Operation.Name
//	}
//	return "?"
//}
