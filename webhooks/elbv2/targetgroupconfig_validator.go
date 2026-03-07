package elbv2

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathValidateGatewayTargetGroupConfiguration = "/validate-gateway-k8s-aws-v1beta1-targetgroupconfiguration"
	gatewayKind                                    = "Gateway"
)

// NewTargetGroupConfigurationValidator returns a validator for the TargetGroupConfiguration CRD.
func NewTargetGroupConfigurationValidator(k8sClient client.Client, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector) *targetGroupConfigurationValidator {
	return &targetGroupConfigurationValidator{
		k8sClient:        k8sClient,
		logger:           logger,
		metricsCollector: metricsCollector,
	}
}

var _ webhook.Validator = &targetGroupConfigurationValidator{}

type targetGroupConfigurationValidator struct {
	k8sClient        client.Client
	logger           logr.Logger
	metricsCollector lbcmetrics.MetricCollector
}

func (v *targetGroupConfigurationValidator) Prototype(_ admission.Request) (runtime.Object, error) {
	return &elbv2gw.TargetGroupConfiguration{}, nil
}

func (v *targetGroupConfigurationValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	tgc := obj.(*elbv2gw.TargetGroupConfiguration)
	return v.checkGatewayTGCUniqueness(ctx, tgc)
}

func (v *targetGroupConfigurationValidator) ValidateUpdate(ctx context.Context, obj runtime.Object, _ runtime.Object) error {
	tgc := obj.(*elbv2gw.TargetGroupConfiguration)
	return v.checkGatewayTGCUniqueness(ctx, tgc)
}

func (v *targetGroupConfigurationValidator) ValidateDelete(_ context.Context, _ runtime.Object) error {
	return nil
}

// checkGatewayTGCUniqueness ensures at most one TargetGroupConfiguration with
// targetReference.kind=Gateway exists per Gateway in the same namespace.
func (v *targetGroupConfigurationValidator) checkGatewayTGCUniqueness(ctx context.Context, tgc *elbv2gw.TargetGroupConfiguration) error {
	if tgc.Spec.TargetReference.Kind == nil || *tgc.Spec.TargetReference.Kind != gatewayKind {
		return nil
	}

	tgcList := &elbv2gw.TargetGroupConfigurationList{}
	if err := v.k8sClient.List(ctx, tgcList, client.InNamespace(tgc.Namespace)); err != nil {
		return fmt.Errorf("Unable to list TargetGroupConfigurations: %w", err)
	}

	for _, existing := range tgcList.Items {
		if existing.Name == tgc.Name {
			continue
		}
		if existing.Spec.TargetReference.Kind != nil &&
			*existing.Spec.TargetReference.Kind == gatewayKind &&
			existing.Spec.TargetReference.Name == tgc.Spec.TargetReference.Name {
			if v.metricsCollector != nil {
				v.metricsCollector.ObserveWebhookValidationError(apiPathValidateGatewayTargetGroupConfiguration, "checkGatewayTGCUniqueness")
			}
			return fmt.Errorf("TargetGroupConfiguration referencing Gateway %q already exists in namespace %q (%s), only one is allowed per Gateway",
				tgc.Spec.TargetReference.Name, tgc.Namespace, existing.Name)
		}
	}
	return nil
}

// +kubebuilder:webhook:path=/validate-gateway-k8s-aws-v1beta1-targetgroupconfiguration,mutating=false,failurePolicy=fail,groups=gateway.k8s.aws,resources=targetgroupconfigurations,verbs=create;update,versions=v1beta1,name=vtargetgroupconfiguration.gateway.k8s.aws,sideEffects=None,matchPolicy=Equivalent,webhookVersions=v1,admissionReviewVersions=v1beta1

func (v *targetGroupConfigurationValidator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathValidateGatewayTargetGroupConfiguration, webhook.ValidatingWebhookForValidator(v, mgr.GetScheme()))
}
