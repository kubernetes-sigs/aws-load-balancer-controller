package networking

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathValidateNetworkingIngress = "/validate-networking-v1-ingress"
)

// NewIngressValidator returns a validator for Ingress API.
func NewIngressValidator(client client.Client, ingConfig config.IngressConfig, logger logr.Logger) *ingressValidator {
	return &ingressValidator{
		annotationParser:                   annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress),
		classAnnotationMatcher:             ingress.NewDefaultClassAnnotationMatcher(ingConfig.IngressClass),
		classLoader:                        ingress.NewDefaultClassLoader(client, false),
		disableIngressClassAnnotation:      ingConfig.DisableIngressClassAnnotation,
		disableIngressGroupAnnotation:      ingConfig.DisableIngressGroupNameAnnotation,
		manageIngressesWithoutIngressClass: ingConfig.IngressClass == "",
		logger:                             logger,
	}
}

var _ webhook.Validator = &ingressValidator{}

type ingressValidator struct {
	annotationParser              annotations.Parser
	classAnnotationMatcher        ingress.ClassAnnotationMatcher
	classLoader                   ingress.ClassLoader
	disableIngressClassAnnotation bool
	disableIngressGroupAnnotation bool
	// manageIngressesWithoutIngressClass specifies whether ingresses without "kubernetes.io/ingress.class" annotation
	// and "spec.ingressClassName" should be managed or not.
	manageIngressesWithoutIngressClass bool
	logger                             logr.Logger
}

func (v *ingressValidator) Prototype(req admission.Request) (runtime.Object, error) {
	return &networking.Ingress{}, nil
}

func (v *ingressValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	ing := obj.(*networking.Ingress)
	if skip, err := v.checkIngressClass(ctx, ing); skip || err != nil {
		return err
	}
	if err := v.checkIngressClassAnnotationUsage(ing, nil); err != nil {
		return err
	}
	if err := v.checkGroupNameAnnotationUsage(ing, nil); err != nil {
		return err
	}
	if err := v.checkIngressClassUsage(ctx, ing, nil); err != nil {
		return err
	}
	if err := v.checkIngressAnnotationConditions(ing); err != nil {
		return err
	}
	return nil
}

func (v *ingressValidator) ValidateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) error {
	ing := obj.(*networking.Ingress)
	oldIng := oldObj.(*networking.Ingress)
	if skip, err := v.checkIngressClass(ctx, ing); skip || err != nil {
		return err
	}
	if err := v.checkIngressClassAnnotationUsage(ing, oldIng); err != nil {
		return err
	}
	if err := v.checkGroupNameAnnotationUsage(ing, oldIng); err != nil {
		return err
	}
	if err := v.checkIngressClassUsage(ctx, ing, oldIng); err != nil {
		return err
	}
	if err := v.checkIngressAnnotationConditions(ing); err != nil {
		return err
	}
	return nil
}

func (v *ingressValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

// checkIngressClass checks to see if this ingress is handled by this controller.
func (v *ingressValidator) checkIngressClass(ctx context.Context, ing *networking.Ingress) (bool, error) {
	if ingClassAnnotation, exists := ing.Annotations[annotations.IngressClass]; exists {
		return !v.classAnnotationMatcher.Matches(ingClassAnnotation), nil
	}
	classConfiguration, err := v.classLoader.Load(ctx, ing)
	if err != nil {
		return false, err
	}
	if classConfiguration.IngClass != nil {
		return classConfiguration.IngClass.Spec.Controller != ingress.IngressClassControllerALB, nil
	}
	return !v.manageIngressesWithoutIngressClass, nil
}

// checkIngressClassAnnotationUsage checks the usage of kubernetes.io/ingress.class annotation.
// kubernetes.io/ingress.class annotation cannot be set to the ingress class for this controller once disabled,
// so that we enforce users to use spec.ingressClassName in Ingress and IngressClass resource instead.
func (v *ingressValidator) checkIngressClassAnnotationUsage(ing *networking.Ingress, oldIng *networking.Ingress) error {
	if !v.disableIngressClassAnnotation {
		return nil
	}
	usedInNewIng := false
	usedInOldIng := false
	if ingClassAnnotation, exists := ing.Annotations[annotations.IngressClass]; exists {
		if v.classAnnotationMatcher.Matches(ingClassAnnotation) {
			usedInNewIng = true
		}
	}
	if oldIng != nil {
		if ingClassAnnotation, exists := oldIng.Annotations[annotations.IngressClass]; exists {
			if v.classAnnotationMatcher.Matches(ingClassAnnotation) {
				usedInOldIng = true
			}
		}
	}
	if !usedInOldIng && usedInNewIng {
		return errors.Errorf("new usage of `%s` annotation is forbidden", annotations.IngressClass)
	}
	return nil
}

// checkGroupNameAnnotationUsage checks the usage of "group.name" annotation.
// "group.name" annotation cannot be set once disabled,
// so that we enforce users to use spec.group in IngressClassParams resource instead.
func (v *ingressValidator) checkGroupNameAnnotationUsage(ing *networking.Ingress, oldIng *networking.Ingress) error {
	if !v.disableIngressGroupAnnotation {
		return nil
	}
	usedInNewIng := false
	usedInOldIng := false
	newGroupName := ""
	oldGroupName := ""
	if exists := v.annotationParser.ParseStringAnnotation(annotations.IngressSuffixGroupName, &newGroupName, ing.Annotations); exists {
		usedInNewIng = true
	}
	if oldIng != nil {
		if exists := v.annotationParser.ParseStringAnnotation(annotations.IngressSuffixGroupName, &oldGroupName, oldIng.Annotations); exists {
			usedInOldIng = true
		}
	}

	if usedInNewIng {
		if !usedInOldIng || (newGroupName != oldGroupName) {
			return errors.Errorf("new usage of `%s/%s` annotation is forbidden", annotations.AnnotationPrefixIngress, annotations.IngressSuffixGroupName)
		}
	}
	return nil
}

// checkIngressClassUsage checks the usage of "ingressClassName" field.
// if ingressClassName is mutated, it must refer to a existing & valid IngressClass.
func (v *ingressValidator) checkIngressClassUsage(ctx context.Context, ing *networking.Ingress, oldIng *networking.Ingress) error {
	usedInNewIng := false
	usedInOldIng := false
	newIngressClassName := ""
	oldIngressClassName := ""

	if ing.Spec.IngressClassName != nil {
		usedInNewIng = true
		newIngressClassName = awssdk.StringValue(ing.Spec.IngressClassName)
	}
	if oldIng != nil && oldIng.Spec.IngressClassName != nil {
		usedInOldIng = true
		oldIngressClassName = awssdk.StringValue(oldIng.Spec.IngressClassName)
	}

	if usedInNewIng {
		if !usedInOldIng || (newIngressClassName != oldIngressClassName) {
			_, err := v.classLoader.Load(ctx, ing)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// checkGroupNameAnnotationUsage checks the validity of "conditions.${conditions-name}" annotation.
func (v *ingressValidator) checkIngressAnnotationConditions(ing *networking.Ingress) error {
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			var conditions []ingress.RuleCondition
			annotationKey := fmt.Sprintf("conditions.%v", path.Backend.Service.Name)
			_, err := v.annotationParser.ParseJSONAnnotation(annotationKey, &conditions, ing.Annotations)
			if err != nil {
				return err
			}

			for _, condition := range conditions {
				if err := condition.Validate(); err != nil {
					return fmt.Errorf("ignoring Ingress %s/%s since invalid alb.ingress.kubernetes.io/conditions.%s annotation: %w",
						ing.Namespace,
						ing.Name,
						path.Backend.Service.Name,
						err,
					)
				}
			}
		}
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-networking-v1-ingress,mutating=false,failurePolicy=fail,groups=networking.k8s.io,resources=ingresses,verbs=create;update,versions=v1,name=vingress.elbv2.k8s.aws,sideEffects=None,matchPolicy=Equivalent,webhookVersions=v1,admissionReviewVersions=v1beta1

func (v *ingressValidator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathValidateNetworkingIngress, webhook.ValidatingWebhookForValidator(v, mgr.GetScheme()))
}
