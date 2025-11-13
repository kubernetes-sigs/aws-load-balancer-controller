package inject

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/targetgroupbinding"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

// NewPodReadinessGate constructs new PodReadinessGate
func NewPodReadinessGate(config Config, k8sClient client.Client, logger logr.Logger) *PodReadinessGate {
	return &PodReadinessGate{
		config:    config,
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// PodReadinessGate is a pod mutator that adds targetHealth readiness gates to pods matching the target group bindings
type PodReadinessGate struct {
	config    Config
	k8sClient client.Client
	logger    logr.Logger
}

// Mutate adds the targetHealth readiness gates to the pod if there are target group bindings on the same namespace as the pod
// and referring to existing services matching the pod labels
func (m *PodReadinessGate) Mutate(ctx context.Context, pod *corev1.Pod) error {
	if !m.config.EnablePodReadinessGateInject {
		return nil
	}

	// see https://github.com/kubernetes/kubernetes/issues/88282 and https://github.com/kubernetes/kubernetes/issues/76680
	req := webhook.ContextGetAdmissionRequest(ctx)
	targetHealthCondTypes, err := m.computeTargetHealthReadinessGateConditionTypes(ctx, req.Namespace, pod)
	if err != nil {
		return err
	}

	if len(targetHealthCondTypes) > 0 {
		// legacy readiness gates are removed for maintaining backwards compatibility.
		m.removeLegacyTargetHealthReadinessGates(ctx, pod)
	}

	for _, condType := range targetHealthCondTypes {
		if !k8s.IsPodHasReadinessGate(pod, condType) {
			pod.Spec.ReadinessGates = append(pod.Spec.ReadinessGates, corev1.PodReadinessGate{
				ConditionType: condType,
			})
		}
	}
	return nil
}

// computeTargetHealthReadinessGateConditionTypes computes the desired condition types for targetHealth readiness gate.
func (m *PodReadinessGate) computeTargetHealthReadinessGateConditionTypes(ctx context.Context, namespace string, pod *corev1.Pod) ([]corev1.PodConditionType, error) {
	tgbList := &elbv2api.TargetGroupBindingList{}
	if err := m.k8sClient.List(ctx, tgbList, client.InNamespace(namespace)); err != nil {
		m.logger.V(1).Info("unable to list TargetGroupBindings", "namespace", namespace)
		return nil, errors.Wrap(err, "unable to determine targetHealth readinessGates")
	}
	var targetHealthCondTypes []corev1.PodConditionType
	for _, tgb := range tgbList.Items {
		if tgb.Spec.TargetType == nil || (*tgb.Spec.TargetType) != elbv2api.TargetTypeIP {
			continue
		}

		svcKey := types.NamespacedName{Namespace: tgb.Namespace, Name: tgb.Spec.ServiceRef.Name}
		svc := &corev1.Service{}
		if err := m.k8sClient.Get(ctx, svcKey, svc); err != nil {
			// If the service is not found, ignore
			if apierrors.IsNotFound(err) {
				m.logger.Info("unable to lookup service", "service", svcKey)
				continue
			}
			return nil, errors.Wrap(err, "unable to determine targetHealth readinessGates")
		}
		var svcSelector labels.Selector
		if len(svc.Spec.Selector) == 0 {
			svcSelector = labels.Nothing()
		} else {
			svcSelector = labels.SelectorFromSet(svc.Spec.Selector)
		}
		if svcSelector.Matches(labels.Set(pod.Labels)) {
			targetHealthCondType := targetgroupbinding.BuildTargetHealthPodConditionType(&tgb)
			targetHealthCondTypes = append(targetHealthCondTypes, targetHealthCondType)
		}
	}
	return targetHealthCondTypes, nil
}

// removeLegacyTargetHealthReadinessGates removes existing legacy targetHealth readiness gates.
func (m *PodReadinessGate) removeLegacyTargetHealthReadinessGates(_ context.Context, pod *corev1.Pod) {
	var modifiedReadinessGates []corev1.PodReadinessGate
	for _, item := range pod.Spec.ReadinessGates {
		if !strings.HasPrefix(string(item.ConditionType), targetgroupbinding.TargetHealthPodConditionTypePrefixLegacy) {
			modifiedReadinessGates = append(modifiedReadinessGates, item)
		}
	}
	pod.Spec.ReadinessGates = modifiedReadinessGates
}
