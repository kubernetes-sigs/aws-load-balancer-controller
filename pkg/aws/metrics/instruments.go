package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricSubsystemAWS = "aws"

	metricAPICallsTotal          = "api_calls_total"
	metricAPICallDurationSeconds = "api_call_duration_seconds"
	metricAPICallRetries         = "api_call_retries"

	metricAPIRequestsTotal          = "api_requests_total"
	metricAPIRequestDurationSeconds = "api_request_duration_seconds"
)

const (
	labelService    = "service"
	labelOperation  = "operation"
	labelStatusCode = "status_code"
	labelErrorCode  = "error_code"
)

type instruments struct {
	apiCallsTotal            *prometheus.CounterVec
	apiCallDurationSeconds   *prometheus.HistogramVec
	apiCallRetries           *prometheus.HistogramVec
	apiRequestsTotal         *prometheus.CounterVec
	apiRequestDurationSecond *prometheus.HistogramVec
}

// newInstruments allocates and register new metrics to registerer
func newInstruments(registerer prometheus.Registerer) (*instruments, error) {
	apiCallsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: metricSubsystemAWS,
		Name:      metricAPICallsTotal,
		Help:      "Total number of SDK API calls from the customer's code to AWS services",
	}, []string{labelService, labelOperation, labelStatusCode, labelErrorCode})
	apiCallDurationSeconds := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: metricSubsystemAWS,
		Name:      metricAPICallDurationSeconds,
		Help:      "Perceived latency from when your code makes an SDK call, includes retries",
	}, []string{labelService, labelOperation})
	apiCallRetries := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: metricSubsystemAWS,
		Name:      metricAPICallRetries,
		Help:      "Number of times the SDK retried requests to AWS services for SDK API calls",
		Buckets:   []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	}, []string{labelService, labelOperation})

	apiRequestsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: metricSubsystemAWS,
		Name:      metricAPIRequestsTotal,
		Help:      "Total number of HTTP requests that the SDK made",
	}, []string{labelService, labelOperation, labelStatusCode, labelErrorCode})
	apiRequestDurationSecond := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: metricSubsystemAWS,
		Name:      metricAPIRequestDurationSeconds,
		Help:      "Latency of an individual HTTP request to the service endpoint",
	}, []string{labelService, labelOperation})

	if err := registerer.Register(apiCallsTotal); err != nil {
		return nil, err
	}
	if err := registerer.Register(apiCallDurationSeconds); err != nil {
		return nil, err
	}
	if err := registerer.Register(apiCallRetries); err != nil {
		return nil, err
	}
	if err := registerer.Register(apiRequestsTotal); err != nil {
		return nil, err
	}
	if err := registerer.Register(apiRequestDurationSecond); err != nil {
		return nil, err
	}
	return &instruments{
		apiCallsTotal:            apiCallsTotal,
		apiCallDurationSeconds:   apiCallDurationSeconds,
		apiCallRetries:           apiCallRetries,
		apiRequestsTotal:         apiRequestsTotal,
		apiRequestDurationSecond: apiRequestDurationSecond,
	}, nil
}
