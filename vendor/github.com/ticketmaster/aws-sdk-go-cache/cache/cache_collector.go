package cache

import (
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/prometheus/client_golang/prometheus"
)

type cacheCollector struct {
	prometheus.Collector

	hitCounter   *prometheus.CounterVec
	flushCounter *prometheus.CounterVec
}

func newCacheCollector(namespace string) *cacheCollector {
	return &cacheCollector{
		hitCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "aws_api_cache_activity",
				Help:      `Cache activity`,
			},
			[]string{"service", "operation", "action"},
		),
		flushCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "aws_api_cache_flushes",
				Help:      `Cache flushes`,
			},
			[]string{"service", "operation"},
		),
	}
}

func (c cacheCollector) Collect(ch chan<- prometheus.Metric) {
	c.hitCounter.Collect(ch)
	c.flushCounter.Collect(ch)
}

// Describe implements prometheus.Collector
func (c cacheCollector) Describe(ch chan<- *prometheus.Desc) {
	c.hitCounter.Describe(ch)
	c.flushCounter.Describe(ch)
}

func (c *cacheCollector) incHit(r *request.Request) {
	c.hitCounter.With(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name, "action": "hit"}).Inc()
}

func (c *cacheCollector) incMiss(r *request.Request) {
	c.hitCounter.With(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name, "action": "miss"}).Inc()
}

func (c *cacheCollector) incFlush(serviceName, operationName string) {
	c.flushCounter.With(prometheus.Labels{"service": serviceName, "operation": operationName}).Inc()
}
