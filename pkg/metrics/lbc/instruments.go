package lbc

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricSubsystem = "awslbc"
)

// These metrics are exported to be used in unit test validation.
const (
	// MetricPodReadinessGateReady tracks the time to flip a readiness gate to true
	MetricPodReadinessGateReady = "readiness_gate_ready_seconds"
)

const (
	labelNamespace = "namespace"
	labelName      = "name"
)

type instruments struct {
	podReadinessFlipSeconds *prometheus.HistogramVec
}

// newInstruments allocates and register new metrics to registerer
func newInstruments(registerer prometheus.Registerer) *instruments {
	podReadinessFlipSeconds := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: metricSubsystem,
		Name:      MetricPodReadinessGateReady,
		Help:      "Latency from pod getting added to the load balancer until the readiness gate is flipped to healthy.",
		Buckets:   []float64{10, 30, 60, 120, 180, 240, 300, 360, 420, 480, 540, 600},
	}, []string{labelNamespace, labelName})

	registerer.MustRegister(podReadinessFlipSeconds)
	return &instruments{
		podReadinessFlipSeconds: podReadinessFlipSeconds,
	}
}
