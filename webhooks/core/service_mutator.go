package core

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathMutateService = "/mutate-v1-service"
)

// NewServiceMutator returns a mutator for Service.
func NewServiceMutator(lbClass string, logger logr.Logger) *serviceMutator {
	return &serviceMutator{
		logger:            logger,
		loadBalancerClass: lbClass,
	}
}

var _ webhook.Mutator = &serviceMutator{}

type serviceMutator struct {
	logger            logr.Logger
	loadBalancerClass string
}

func (m *serviceMutator) Prototype(_ admission.Request) (runtime.Object, error) {
	return &corev1.Service{}, nil
}

func (m *serviceMutator) MutateCreate(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	svc := obj.(*corev1.Service)
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return svc, nil
	}

	if svc.Spec.LoadBalancerClass != nil && *svc.Spec.LoadBalancerClass != "" {
		m.logger.Info("service already has loadBalancerClass, skipping", "service", svc.Name, "loadBalancerClass", *svc.Spec.LoadBalancerClass)
		return svc, nil
	}

	svc.Spec.LoadBalancerClass = &m.loadBalancerClass
	m.logger.Info("setting service loadBalancerClass", "service", svc.Name, "loadBalancerClass", m.loadBalancerClass)

	return svc, nil
}

func (m *serviceMutator) MutateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error) {
	return obj, nil
}

// +kubebuilder:webhook:path=/mutate-v1-service,mutating=true,failurePolicy=fail,groups="",resources=services,verbs=create,versions=v1,name=mservice.elbv2.k8s.aws,sideEffects=None,webhookVersions=v1,admissionReviewVersions=v1beta1

func (m *serviceMutator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathMutateService, webhook.MutatingWebhookForMutator(m))
}
