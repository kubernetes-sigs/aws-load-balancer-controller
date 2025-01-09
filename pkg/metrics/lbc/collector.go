package lbc

import (
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

type MetricCollector interface {
	// ObservePodReadinessGateReady this metric is useful to determine how fast pods are becoming ready in the load balancer.
	// Due to some architectural constraints, we can only emit this metric for pods that are using readiness gates.
	ObservePodReadinessGateReady(namespace string, tgbName string, duration time.Duration)
}

type collector struct {
	instruments *instruments
}

type noOpCollector struct{}

func (n *noOpCollector) ObservePodReadinessGateReady(_ string, _ string, _ time.Duration) {
}

func NewCollector(registerer prometheus.Registerer) MetricCollector {
	if registerer == nil {
		return &noOpCollector{}
	}

	instruments := newInstruments(registerer)
	return &collector{
		instruments: instruments,
	}
}

func (c *collector) ObservePodReadinessGateReady(namespace string, tgbName string, duration time.Duration) {
	c.instruments.podReadinessFlipSeconds.With(prometheus.Labels{
		labelNamespace: namespace,
		labelName:      tgbName,
	}).Observe(duration.Seconds())
}
