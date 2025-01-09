package aws

import (
	"context"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/smithy-go"
	smithymiddleware "github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"strconv"
	"time"
)

const (
	sdkMiddlewareCollectAPICallMetric    = "collectAPICallMetric"
	sdkMiddlewareCollectAPIRequestMetric = "collectAPIRequestMetric"
)

type Collector struct {
	instruments *instruments
}

func NewCollector(registerer prometheus.Registerer) *Collector {
	instruments := newInstruments(registerer)
	return &Collector{
		instruments: instruments,
	}
}

/*
WithSDKMetricCollector is a function that collects prometheus metrics for the AWS SDK Go v2 API calls ad requests
*/
func WithSDKMetricCollector(c *Collector, apiOptions []func(*smithymiddleware.Stack) error) []func(*smithymiddleware.Stack) error {
	apiOptions = append(apiOptions, func(stack *smithymiddleware.Stack) error {
		return WithSDKCallMetricCollector(c)(stack)
	}, func(stack *smithymiddleware.Stack) error {
		return WithSDKRequestMetricCollector(c)(stack)
	})
	return apiOptions
}

/*
WithSDKCallMetricCollector is a middleware for the AWS SDK Go v2 that collects and reports metrics on API calls.
The call metrics are collected after the call is completed
*/
func WithSDKCallMetricCollector(c *Collector) func(stack *smithymiddleware.Stack) error {
	return func(stack *smithymiddleware.Stack) error {
		return stack.Initialize.Add(smithymiddleware.InitializeMiddlewareFunc(sdkMiddlewareCollectAPICallMetric, func(
			ctx context.Context, input smithymiddleware.InitializeInput, next smithymiddleware.InitializeHandler,
		) (
			output smithymiddleware.InitializeOutput, metadata smithymiddleware.Metadata, err error,
		) {
			start := time.Now()
			out, metadata, err := next.HandleInitialize(ctx, input)
			resp, ok := awsmiddleware.GetRawResponse(metadata).(*smithyhttp.Response)
			if !ok {
				// No raw response to wrap with.
				return out, metadata, err
			}
			service := awsmiddleware.GetServiceID(ctx)
			operation := operationForRequest(ctx)
			statusCode := strconv.Itoa(resp.StatusCode)
			errorCode := errorCodeForRequest(err)
			retryCount := getRetryMetricsForRequest(metadata)
			duration := time.Since(start)
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
			}).Observe(retryCount)
			return out, metadata, err
		}), smithymiddleware.After)
	}
}

/*
WithSDKRequestMetricCollector is a middleware for the AWS SDK Go v2 that collects and reports metrics on API requests.
The request metrics are collected after each retry attempts
*/
func WithSDKRequestMetricCollector(c *Collector) func(stack *smithymiddleware.Stack) error {
	return func(stack *smithymiddleware.Stack) error {
		return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc(sdkMiddlewareCollectAPIRequestMetric, func(
			ctx context.Context, input smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler,
		) (
			output smithymiddleware.FinalizeOutput, metadata smithymiddleware.Metadata, err error,
		) {
			start := time.Now()
			out, metadata, err := next.HandleFinalize(ctx, input)
			resp, ok := awsmiddleware.GetRawResponse(metadata).(*smithyhttp.Response)
			if !ok {
				// No raw response to wrap with.
				return out, metadata, err
			}
			service := awsmiddleware.GetServiceID(ctx)
			operation := operationForRequest(ctx)
			statusCode := strconv.Itoa(resp.StatusCode)
			errorCode := errorCodeForRequest(err)
			c.instruments.apiRequestsTotal.With(map[string]string{
				labelService:    service,
				labelOperation:  operation,
				labelStatusCode: statusCode,
				labelErrorCode:  errorCode,
			}).Inc()

			requestDuration, ok := awsmiddleware.GetResponseAt(metadata)
			if ok {
				c.instruments.apiRequestDurationSecond.With(map[string]string{
					labelService:   service,
					labelOperation: operation,
				}).Observe(requestDuration.Sub(start).Seconds())
			}
			return out, metadata, err
		}), smithymiddleware.After)
	}
}

func getRetryMetricsForRequest(metadata smithymiddleware.Metadata) float64 {
	retries := float64(0)
	attemptResults, ok := retry.GetAttemptResults(metadata)
	if ok {
		for _, result := range attemptResults.Results {
			if result.Retried {
				retries++
			}
		}
	}
	return retries
}

// errorCodeForRequest returns the error code for response.
func errorCodeForRequest(err error) string {
	errCode := ""
	if err == nil {
		return errCode
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode()
	}
	return "internal"
}

// operationForRequest returns the operation for request.
func operationForRequest(ctx context.Context) string {
	if awsmiddleware.GetOperationName(ctx) != "" {
		return awsmiddleware.GetOperationName(ctx)
	}
	return "?"
}
