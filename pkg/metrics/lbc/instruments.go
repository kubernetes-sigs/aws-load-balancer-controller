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
	// MetricControllerReconcileErrors tracks the total number of controller errors by error type.
	MetricControllerReconcileErrors = "controller_reconcile_errors_total"
	// MetricControllerReconcileStageDuration tracks latencies of different reconcile stages.
	MetricControllerReconcileStageDuration = "controller_reconcile_stage_duration"
	// MetricWebhookValidationFailure tracks the total number of validation errors by error type.
	MetricWebhookValidationFailure = "webhook_validation_failure_total"
	// MetricWebhookMutationFailure tracks the total number of mutation errors by error type.
	MetricWebhookMutationFailure = "webhook_mutation_failure_total"
	// MetricControllerCacheObjectCount tracks the total number of object in the controller runtime cache.
	MetricControllerCacheObjectCount = "controller_cache_object_total"
	// MetricTopTalker tracks what resources are causing the most reconciles.
	MetricControllerTopTalkers = "controller_top_talkers"
	// MetricQuicTargetMissingServerId tracks the total number of QUIC targets attempted to be registered without a generated server id.
	MetricQuicTargetMissingServerId = "quic_target_missing_server_id"
	// MetricIngressCertErrorSkipped tracks the total number of Ingresses skipped due to certificate errors
	MetricIngressCertErrorSkipped = "ingress_cert_error_skipped_total"
)

const (
	labelNamespace      = "namespace"
	labelName           = "name"
	labelController     = "controller"
	labelErrorCategory  = "error_category"
	labelReconcileStage = "reconcile_stage"
	labelWebhookName    = "webhook_name"
	LabelResource       = "resource"
	labelIngressName    = "ingress_name"
	labelGroupName      = "group_name"
)

type instruments struct {
	podReadinessFlipSeconds       *prometheus.HistogramVec
	quicTargetsMissingServerId    *prometheus.CounterVec
	controllerReconcileErrors     *prometheus.CounterVec
	controllerReconcileLatency    *prometheus.HistogramVec
	webhookValidationFailure      *prometheus.CounterVec
	webhookMutationFailure        *prometheus.CounterVec
	controllerCacheObjectCount    *prometheus.GaugeVec
	controllerReconcileTopTalkers *prometheus.GaugeVec
	ingressCertErrorSkipped       *prometheus.CounterVec
}

// newInstruments allocates and register new metrics to registerer
func newInstruments(registerer prometheus.Registerer) *instruments {
	podReadinessFlipSeconds := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: metricSubsystem,
		Name:      MetricPodReadinessGateReady,
		Help:      "Latency from pod getting added to the load balancer until the readiness gate is flipped to healthy.",
		Buckets:   []float64{10, 30, 60, 120, 180, 240, 300, 360, 420, 480, 540, 600},
	}, []string{labelNamespace, labelName})

	controllerReconcileErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: metricSubsystem,
		Name:      MetricControllerReconcileErrors,
		Help:      "Counts the number of reconcile error, categorized by error type.",
	}, []string{labelController, labelErrorCategory})

	controllerQuicTargetMissingServerId := prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: metricSubsystem,
		Name:      MetricQuicTargetMissingServerId,
		Help:      "tracks the total number of QUIC targets attempted to be registered without a generated server id.",
	}, []string{labelNamespace, labelName})

	controllerReconcileStageDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: metricSubsystem,
		Name:      MetricControllerReconcileStageDuration,
		Help:      "latencies of different reconcile stages.",
		Buckets:   prometheus.DefBuckets,
	}, []string{labelController, labelReconcileStage})

	webhookValidationFailure := prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: metricSubsystem,
		Name:      MetricWebhookValidationFailure,
		Help:      "Counts the number of webhook validation failure, categorized by error type.",
	}, []string{labelWebhookName, labelErrorCategory})

	webhookMutationFailure := prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: metricSubsystem,
		Name:      MetricWebhookMutationFailure,
		Help:      "Counts the number of webhook mutation failure, categorized by error type.",
	}, []string{labelWebhookName, labelErrorCategory})

	controllerCacheObjectCount := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricSubsystem,
		Name:      MetricControllerCacheObjectCount,
		Help:      "Counts the number of objects in the controller cache.",
	}, []string{LabelResource})

	controllerReconcileTopTalkers := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricSubsystem,
		Name:      MetricControllerTopTalkers,
		Help:      "Counts the number of reconciliations triggered per resource",
	}, []string{labelController, labelNamespace, labelName})

	ingressCertErrorSkipped := prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: metricSubsystem,
		Name:      MetricIngressCertErrorSkipped,
		Help:      "Counts the total number of Ingresses skipped due to certificate errors.",
	}, []string{labelNamespace, labelIngressName, labelGroupName})

	registerer.MustRegister(podReadinessFlipSeconds, controllerReconcileErrors, controllerReconcileStageDuration, webhookValidationFailure, webhookMutationFailure, controllerCacheObjectCount, controllerReconcileTopTalkers, ingressCertErrorSkipped)
	return &instruments{
		podReadinessFlipSeconds:       podReadinessFlipSeconds,
		controllerReconcileErrors:     controllerReconcileErrors,
		controllerReconcileLatency:    controllerReconcileStageDuration,
		webhookValidationFailure:      webhookValidationFailure,
		webhookMutationFailure:        webhookMutationFailure,
		controllerCacheObjectCount:    controllerCacheObjectCount,
		controllerReconcileTopTalkers: controllerReconcileTopTalkers,
		quicTargetsMissingServerId:    controllerQuicTargetMissingServerId,
		ingressCertErrorSkipped:       ingressCertErrorSkipped,
	}
}
