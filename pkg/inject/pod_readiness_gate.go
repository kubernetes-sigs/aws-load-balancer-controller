package inject

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2api "sigs.k8s.io/aws-alb-ingress-controller/apis/elbv2/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

const (
	v1ReadiessGatePrefix = "target-health.alb.ingress.k8s.aws"
	readinessGatePrefix  = "elbv2.k8s.aws"
)

// NewPodReadinessGate constructs new PodReadinessGate
func NewPodReadinessGate(config Config, k8sClient client.Client, logger logr.Logger) *PodReadinessGate {
	return &PodReadinessGate{
		config:    config,
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// PodReadinessGate is a pod mutator that adds readiness gates to pods matching the target group bindings
type PodReadinessGate struct {
	config    Config
	k8sClient client.Client
	logger    logr.Logger
}

// matchLabels returns true if the service selector matches the pod labels
func (m *PodReadinessGate) matchLabels(selector map[string]string, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for key, selectorVal := range selector {
		if labelsVal, ok := labels[key]; !ok || labelsVal != selectorVal {
			return false
		}
	}
	return true
}

// removeReadinessGates removes existing v1/v2 readiness gates. v1 readiness gates are removed for maintaining
// backwards compatibility
func (m *PodReadinessGate) removeReadinessGates(_ context.Context, pod *corev1.Pod) {
	var modifiedReadinessGates []corev1.PodReadinessGate
	for _, item := range pod.Spec.ReadinessGates {
		if !strings.HasPrefix(string(item.ConditionType), v1ReadiessGatePrefix) &&
			!strings.HasPrefix(string(item.ConditionType), readinessGatePrefix) {
			modifiedReadinessGates = append(modifiedReadinessGates, item)
		}
	}
	pod.Spec.ReadinessGates = modifiedReadinessGates
}

// Mutate adds the readiness gates to the pod if there are target group bindings on the same namespace as the pod
// and referring to existing services matching the pod labels
func (m *PodReadinessGate) Mutate(ctx context.Context, pod *corev1.Pod) error {
	if !m.config.EnablePodReadinessGateInject {
		return nil
	}
	m.removeReadinessGates(ctx, pod)

	// see https://github.com/kubernetes/kubernetes/issues/88282 and https://github.com/kubernetes/kubernetes/issues/76680
	req := webhook.ContextGetAdmissionRequest(ctx)
	tgbList := &elbv2api.TargetGroupBindingList{}
	if err := m.k8sClient.List(ctx, tgbList, client.InNamespace(req.Namespace)); err != nil {
		m.logger.V(1).Info("unable to list TargetGroupBindings", "namespace", req.Namespace)
		return client.IgnoreNotFound(err)
	}

	var newReadinessGates []corev1.PodReadinessGate
	for _, tg := range tgbList.Items {
		svcRef := tg.Spec.ServiceRef
		svc := &corev1.Service{}
		if err := m.k8sClient.Get(ctx, types.NamespacedName{Name: svcRef.Name, Namespace: req.Namespace}, svc); err != nil {
			// If the service is not found, ignore
			if client.IgnoreNotFound(err) == nil {
				m.logger.Info("Unable to lookup service", "name", svcRef, "namespace", req.Namespace)
				continue
			}
			return err
		}
		if m.matchLabels(svc.Spec.Selector, pod.Labels) {
			readinessGate := corev1.PodReadinessGate{
				ConditionType: corev1.PodConditionType(fmt.Sprintf("%s/%s", readinessGatePrefix, tg.Name)),
			}
			newReadinessGates = append(newReadinessGates, readinessGate)
			m.logger.V(1).Info("Adding readiness gate", "readinessGate", readinessGate.String())
		}
	}
	m.logger.V(1).Info("Appending readiness gates to", "pod", pod.Name, "gates", newReadinessGates)
	pod.Spec.ReadinessGates = append(pod.Spec.ReadinessGates, newReadinessGates...)
	return nil
}
