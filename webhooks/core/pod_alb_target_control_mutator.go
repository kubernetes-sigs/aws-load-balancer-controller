package core

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/inject/albtargetcontrol"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathMutateAlbTargetControlNamespace = "/mutate-alb-target-control-namespace-v1-pod"
	apiPathMutateAlbTargetControlObject    = "/mutate-alb-target-control-object-v1-pod"
)

// NewAlbTargetControlAgentMutator returns a mutator for ALB target control agent sidecar injection.
func NewALBTargetControlAgentMutator(albTargetControlAgentInjector albtargetcontrol.ALBTargetControlAgentInjector, metricsCollector lbcmetrics.MetricCollector) *ALBTargetControlAgentMutator {
	return &ALBTargetControlAgentMutator{
		albTargetControlAgentInjector: albTargetControlAgentInjector,
		metricsCollector:              metricsCollector,
		logger:                        ctrl.Log.WithName("alb-target-control-agent-mutator"),
	}
}

var _ webhook.Mutator = &ALBTargetControlAgentMutator{}

type ALBTargetControlAgentMutator struct {
	albTargetControlAgentInjector albtargetcontrol.ALBTargetControlAgentInjector
	metricsCollector              lbcmetrics.MetricCollector
	logger                        logr.Logger
}

func (m *ALBTargetControlAgentMutator) Prototype(_ admission.Request) (runtime.Object, error) {
	return &corev1.Pod{}, nil
}

func (m *ALBTargetControlAgentMutator) MutateCreate(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	pod := obj.(*corev1.Pod)

	if m.albTargetControlAgentInjector != nil {
		m.logger.Info("Attempting ALB target control agent sidecar injection", "pod", pod.Name, "namespace", pod.Namespace)
		if err := m.albTargetControlAgentInjector.Mutate(ctx, pod); err != nil {
			m.logger.Error(err, "Failed to inject ALB target control agent sidecar", "pod", pod.Name, "namespace", pod.Namespace)
			m.metricsCollector.ObserveWebhookMutationError(apiPathMutateAlbTargetControlNamespace, "albTargetControlAgentInjector")
			return pod, err
		}
		m.logger.Info("ALB target control agent sidecar injection completed", "pod", pod.Name, "namespace", pod.Namespace)
	}

	return pod, nil
}

func (m *ALBTargetControlAgentMutator) MutateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error) {
	return obj, nil
}

// +kubebuilder:webhook:path=/mutate-alb-target-control-namespace-v1-pod,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create,versions=v1,name=alb-target-control.namespace.elbv2.k8s.aws,sideEffects=None,webhookVersions=v1,admissionReviewVersions=v1beta1
// +kubebuilder:webhook:path=/mutate-alb-target-control-object-v1-pod,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create,versions=v1,name=alb-target-control.object.elbv2.k8s.aws,sideEffects=None,webhookVersions=v1,admissionReviewVersions=v1beta1

func (m *ALBTargetControlAgentMutator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathMutateAlbTargetControlNamespace, webhook.MutatingWebhookForMutator(m, mgr.GetScheme()))
	mgr.GetWebhookServer().Register(apiPathMutateAlbTargetControlObject, webhook.MutatingWebhookForMutator(m, mgr.GetScheme()))
}
