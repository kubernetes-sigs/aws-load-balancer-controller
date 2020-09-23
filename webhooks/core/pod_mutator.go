package core

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	podinjector "sigs.k8s.io/aws-load-balancer-controller/pkg/inject"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathMutatePod = "/mutate-v1-pod"
)

// NewPodMutator returns a mutator for Pod.
func NewPodMutator(readiness *podinjector.PodReadinessGate) *podMutator {
	return &podMutator{
		readiness: readiness,
	}
}

var _ webhook.Mutator = &podMutator{}

type podMutator struct {
	readiness *podinjector.PodReadinessGate
}

func (m *podMutator) Prototype(req admission.Request) (runtime.Object, error) {
	return &corev1.Pod{}, nil
}

func (m *podMutator) MutateCreate(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	pod := obj.(*corev1.Pod)
	err := m.readiness.Mutate(ctx, pod)
	return pod, err
}

func (m *podMutator) MutateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error) {
	return obj, nil
}

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create,versions=v1alpha1,name=mpod.elbv2.k8s.aws
func (m *podMutator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathMutatePod, webhook.MutatingWebhookForMutator(m))
}
