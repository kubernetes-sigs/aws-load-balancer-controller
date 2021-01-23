package core

import (
	"context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gracefuldrain"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathValidatePod = "/validate-v1-pod"
)

// NewPodValidator returns a mutator for Pod.
func NewPodValidator(podGracefulDrain *gracefuldrain.PodGracefulDrain, logger logr.Logger) *podValidator {
	return &podValidator{
		podGracefulDrain,
		logger,
	}
}

var _ webhook.Validator = &podValidator{}

type podValidator struct {
	podGracefulDrain *gracefuldrain.PodGracefulDrain
	logger           logr.Logger
}

func (v *podValidator) Prototype(_ admission.Request) (runtime.Object, error) {
	return &corev1.Pod{}, nil
}

func (v *podValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	return nil
}

func (v *podValidator) ValidateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) error {
	return nil
}

func (v *podValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	pod := obj.(*corev1.Pod)
	// It is somewhat awkward to modify the pod in the validating webhook.
	// However, podGracefulDrain doesn't modify the request on the fly, it listens and schedule delayed deletion (with some side-effects)
	// So the mutating webhook would be awkward too.
	// Here's the rationale: It just adds delay on the pod deletion. The pod will be deleted 'eventually' anyway.
	if err := v.podGracefulDrain.InterceptPodDeletion(ctx, pod); err != nil {
		return err
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-v1-pod,mutating=false,failurePolicy=fail,groups="",resources=pods,verbs=delete,versions=v1,name=vpod.elbv2.k8s.aws,sideEffects=NoneOnDryRun,webhookVersions=v1beta1

func (v *podValidator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathValidatePod, webhook.ValidatingWebhookForValidator(v))
}
