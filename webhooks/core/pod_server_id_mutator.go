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
	apiPathMutatePodServerID = "/mutate-v1-pod-server-id"
)

// NewPodServerIDMutator returns a pod server id mutator.
func NewPodServerIDMutator(serverIDInjector inject.QUICServerIDInjector, metricsCollector lbcmetrics.MetricCollector) *ServerIDMutator {
	return &ServerIDMutator{
		serverIDInjector: serverIDInjector,
		metricsCollector: metricsCollector,
	}
}

var _ webhook.Mutator = &podReadinessGateMutator{}

type ServerIDMutator struct {
	serverIDInjector inject.QUICServerIDInjector
	metricsCollector lbcmetrics.MetricCollector
}

func (m *ServerIDMutator) Prototype(_ admission.Request) (runtime.Object, error) {
	return &corev1.Pod{}, nil
}

func (m *ServerIDMutator) MutateCreate(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	pod := obj.(*corev1.Pod)
	if err := m.serverIDInjector.Mutate(ctx, pod); err != nil {
		m.metricsCollector.ObserveWebhookMutationError(apiPathMutatePodServerID, "serverIDInjector")
		return pod, err
	}
	return pod, nil
}

func (m *ServerIDMutator) MutateUpdate(_ context.Context, obj runtime.Object, _ runtime.Object) (runtime.Object, error) {
	return obj, nil
}

// +kubebuilder:webhook:path=/mutate-v1-pod-server-id,mutating=true,failurePolicy=Fail,groups="",resources=pods,verbs=create,versions=v1,name=quicid.elbv2.k8s.aws,sideEffects=None,webhookVersions=v1,admissionReviewVersions=v1beta1

func (m *ServerIDMutator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathMutatePodServerID, webhook.MutatingWebhookForMutator(m, mgr.GetScheme()))
}
