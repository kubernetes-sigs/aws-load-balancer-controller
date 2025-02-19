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
	managedIngressCount     prometheus.Gauge
	managedServiceCount     prometheus.Gauge
	managedTGBCount         prometheus.Gauge
	managedALBCount         prometheus.Gauge
	managedNLBCount         prometheus.Gauge
}

// newInstruments allocates and register new metrics to registerer
func newInstruments(registerer prometheus.Registerer) *instruments {
	podReadinessFlipSeconds := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: metricSubsystem,
		Name:      MetricPodReadinessGateReady,
		Help:      "Latency from pod getting added to the load balancer until the readiness gate is flipped to healthy.",
		Buckets:   []float64{10, 30, 60, 120, 180, 240, 300, 360, 420, 480, 540, 600},
	}, []string{labelNamespace, labelName})
	managedIngressCount := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lb_controller_managed_ingress_count",
		Help: "Number of ingresses managed by the AWS Load Balancer Controller.",
	})
	managedServiceCount := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lb_controller_managed_service_count",
		Help: "Number of service type Load Balancers (NLBs) managed by the AWS Load Balancer Controller.",
	})
	managedTGBCount := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lb_controller_managed_targetgroupbinding_count",
		Help: "Number of targetgroupbindings managed by the AWS Load Balancer Controller.",
	})
	managedALBCount := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lb_controller_managed_albs_total",
		Help: "Current number of ALBs managed by the controller",
	})
	managedNLBCount := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lb_controller_managed_nlbs_total",
		Help: "Current number of NLBs managed by the controller",
	})
	registerer.MustRegister(
		podReadinessFlipSeconds,
		managedIngressCount,
		managedServiceCount,
		managedTGBCount,
		managedALBCount,
		managedNLBCount,
	)
	return &instruments{
		podReadinessFlipSeconds: podReadinessFlipSeconds,
		managedIngressCount:     managedIngressCount,
		managedServiceCount:     managedServiceCount,
		managedTGBCount:         managedTGBCount,
		managedALBCount:         managedALBCount,
		managedNLBCount:         managedNLBCount,
	}
}
