/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package collectors

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// AWSAPIController defines base metrics about the AWS API client
type AWSAPIController struct {
	prometheus.Collector

	awsAPIRequest        *prometheus.CounterVec
	awsAPIRequestLatency *prometheus.HistogramVec
	awsAPIError          *prometheus.CounterVec
	awsAPIRetry          *prometheus.CounterVec
}

// NewAWSAPIController creates a new prometheus collector for the
// AWS API client
func NewAWSAPIController() *AWSAPIController {
	return &AWSAPIController{
		awsAPIRequest: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: PrometheusNamespace,
				Name:      "aws_api_requests",
				Help:      `Cumulative number of requests made to the AWS API`,
			},
			[]string{"service", "operation"},
		),
		awsAPIRequestLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: PrometheusNamespace,
			Name:      "aws_api_requests_latency_seconds",
			Help:      `Total latency distributions of requests made to the AWS API (in seconds)`,

			// lowest bucket start of upper bound 0.0001 sec (0.1 ms) with factor 2
			// highest bucket start of 0.0001 sec * 2^17 == 13.1072 sec
			Buckets: prometheus.ExponentialBuckets(0.0001, 2, 18),
		},
			[]string{"operation"},
		),
		awsAPIError: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: PrometheusNamespace,
				Name:      "aws_api_errors",
				Help:      `Cumulative number of errors from the AWS API`,
			},
			[]string{"service", "operation"},
		),
		awsAPIRetry: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: PrometheusNamespace,
				Name:      "aws_api_retries",
				Help:      `Cumulative number of retries to the AWS API`,
			},
			[]string{"service", "operation"},
		),
	}
}

// IncAPIRequestCount increment the reconcile counter
func (a *AWSAPIController) IncAPIRequestCount(l prometheus.Labels) {
	a.awsAPIRequest.With(l).Inc()
}

// ObserveAPIRequest tracks request latency
func (a *AWSAPIController) ObserveAPIRequest(l prometheus.Labels, start time.Time) {
	a.awsAPIRequestLatency.With(l).Observe(time.Since(start).Seconds())
}

// IncAPIErrorCount increment the reconcile counter
func (a *AWSAPIController) IncAPIErrorCount(l prometheus.Labels) {
	a.awsAPIError.With(l).Inc()
}

// IncAPIRetryCount increment the reconcile counter
func (a *AWSAPIController) IncAPIRetryCount(l prometheus.Labels) {
	a.awsAPIRetry.With(l).Inc()
}

// Describe implements prometheus.Collector
func (a AWSAPIController) Describe(ch chan<- *prometheus.Desc) {
	a.awsAPIRequest.Describe(ch)
	a.awsAPIRequestLatency.Describe(ch)
	a.awsAPIError.Describe(ch)
	a.awsAPIRetry.Describe(ch)
}

// Collect implements the prometheus.Collector interface.
func (a AWSAPIController) Collect(ch chan<- prometheus.Metric) {
	a.awsAPIRequest.Collect(ch)
	a.awsAPIRequestLatency.Collect(ch)
	a.awsAPIError.Collect(ch)
	a.awsAPIRetry.Collect(ch)
}
