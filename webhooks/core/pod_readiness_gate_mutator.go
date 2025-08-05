package core

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/inject"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathMutatePodReadinessGate = "/mutate-v1-pod"
)

// NewPodReadinessGateMutator returns a pod readiness gate mutator.
func NewPodReadinessGateMutator(podReadinessGateInjector *inject.PodReadinessGate, metricsCollector lbcmetrics.MetricCollector) *podReadinessGateMutator {
	return &podReadinessGateMutator{
		podReadinessGateInjector: podReadinessGateInjector,
		metricsCollector:         metricsCollector,
	}
}

var _ webhook.Mutator = &podReadinessGateMutator{}

type podReadinessGateMutator struct {
	podReadinessGateInjector *inject.PodReadinessGate
	metricsCollector         lbcmetrics.MetricCollector
}

func (m *podReadinessGateMutator) Prototype(_ admission.Request) (runtime.Object, error) {
	return &corev1.Pod{}, nil
}

func (m *podReadinessGateMutator) MutateCreate(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	pod := obj.(*corev1.Pod)
	if err := m.podReadinessGateInjector.Mutate(ctx, pod); err != nil {
		m.metricsCollector.ObserveWebhookMutationError(apiPathMutatePodReadinessGate, "podReadinessGateInjector")
		return pod, err
	}

	return pod, nil
}

func (m *podReadinessGateMutator) MutateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error) {
	return obj, nil
}

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=ignore,groups="",resources=pods,verbs=create,versions=v1,name=mpod.elbv2.k8s.aws,sideEffects=None,webhookVersions=v1,admissionReviewVersions=v1beta1

func (m *podReadinessGateMutator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathMutatePodReadinessGate, webhook.MutatingWebhookForMutator(m, mgr.GetScheme()))
}
