package networking

import (
	"context"

	"github.com/go-logr/logr"
	networking "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathValidateNetworkingIngressClass = "/validate-networking-v1-ingressclass"
)

// NewIngressClassValidator returns a validator for IngressClass API.
func NewIngressClassValidator(client client.Client, logger logr.Logger) *ingressClassValidator {
	return &ingressClassValidator{
		client: client,
		logger: logger,
	}
}

var _ webhook.Validator = &ingressClassValidator{}

type ingressClassValidator struct {
	client client.Client
	logger logr.Logger
}

func (v *ingressClassValidator) Prototype(req admission.Request) (runtime.Object, error) {
	return &networking.IngressClass{}, nil
}

func (v *ingressClassValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	ingClass := obj.(*networking.IngressClass)
	return v.validate(ctx, ingClass)
}

func (v *ingressClassValidator) ValidateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) error {
	ingClass := obj.(*networking.IngressClass)
	return v.validate(ctx, ingClass)
}

func (v *ingressClassValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

// checkIngressClass checks to see if this ingress is handled by this controller.
func (v *ingressClassValidator) validate(ctx context.Context, ingClass *networking.IngressClass) error {
	if ingClass.Spec.Controller != ingress.IngressClassControllerALB {
		return nil
	}

	if ingClass.Spec.Parameters != nil {
		fld := field.NewPath("spec", "parameters")
		allErrs := field.ErrorList{}
		if ingClass.Spec.Parameters.APIGroup == nil {
			allErrs = append(allErrs, field.Required(fld.Child("apiGroup"), "must be \"elbv2.k8s.aws\""))
		} else if (*ingClass.Spec.Parameters.APIGroup) != elbv2api.GroupVersion.Group {
			allErrs = append(allErrs, field.Forbidden(fld.Child("apiGroup"), "must be \"elbv2.k8s.aws\""))
		}
		if ingClass.Spec.Parameters.Kind == "" {
			allErrs = append(allErrs, field.Required(fld.Child("kind"), "must be \"IngressClassParams\""))
		} else if ingClass.Spec.Parameters.Kind != "IngressClassParams" {
			allErrs = append(allErrs, field.Forbidden(fld.Child("kind"), "must be \"IngressClassParams\""))
		}

		if len(allErrs) == 0 {
			ingClassParamsKey := types.NamespacedName{Name: ingClass.Spec.Parameters.Name}
			ingClassParams := &elbv2api.IngressClassParams{}
			if err := v.client.Get(ctx, ingClassParamsKey, ingClassParams); err != nil {
				if apierrors.IsNotFound(err) {
					return field.NotFound(fld.Child("name"), ingClass.Spec.Parameters.Name)
				}
				return err
			}
		}

		return allErrs.ToAggregate()
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-networking-v1-ingressclass,mutating=false,failurePolicy=fail,groups=networking.k8s.io,resources=ingressclasses,verbs=create;update,versions=v1,name=vingressclass.elbv2.k8s.aws,sideEffects=None,matchPolicy=Equivalent,webhookVersions=v1,admissionReviewVersions=v1beta1

func (v *ingressClassValidator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathValidateNetworkingIngressClass, webhook.ValidatingWebhookForValidator(v))
}
