package networking

import (
	"context"
	"fmt"
	"strings"

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
	lbAttrsDeletionProtectionEnabled = "deletion_protection.enabled"
	defaultScheme                    = "internal"
)

// NewIngressValidator returns a validator for Ingress API.
func NewIngressValidator(client client.Client, ingConfig config.IngressConfig, logger logr.Logger) *ingressValidator {
	return &ingressValidator{
		annotationParser:              annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress),
		classAnnotationMatcher:        ingress.NewDefaultClassAnnotationMatcher(ingConfig.IngressClass),
		classLoader:                   ingress.NewDefaultClassLoader(client),
		disableIngressClassAnnotation: ingConfig.DisableIngressClassAnnotation,
		disableIngressGroupAnnotation: ingConfig.DisableIngressGroupNameAnnotation,
		logger:                        logger,
	}
}

var _ webhook.Validator = &ingressValidator{}

type ingressValidator struct {
	annotationParser              annotations.Parser
	classAnnotationMatcher        ingress.ClassAnnotationMatcher
	classLoader                   ingress.ClassLoader
	disableIngressClassAnnotation bool
	disableIngressGroupAnnotation bool
	logger                        logr.Logger
}

func (v *ingressValidator) Prototype(req admission.Request) (runtime.Object, error) {
	return &networking.Ingress{}, nil
}

func (v *ingressValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	ing := obj.(*networking.Ingress)
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
	if err := v.validateDeletionProtectionAnnotation(ctx, ing, oldIng); err != nil {
		return err
	}
	return nil
}

func (v *ingressValidator) validateDeletionProtectionAnnotation(ctx context.Context, ing *networking.Ingress, oldIng *networking.Ingress) error {
	// Get the values of the scheme and ingressClass for the old and new Ingress objects
	var rawSchemaOld, rawSchema string
	var controllerPartOld, controllerPart string
	var ingClassConfig ingress.ClassConfiguration
	var err error
	if controller, exists := ing.Annotations[annotations.IngressClass]; exists {
		// Parse the ingress suffix scheme and ingress class from annotations
		_ = v.annotationParser.ParseStringAnnotation(annotations.IngressSuffixScheme, &rawSchema, ing.Annotations)
		controllerPart = controller
	} else if ing.Spec.IngressClassName != nil {
		ingClassConfig, err = v.classLoader.Load(ctx, ing)
		if err != nil {
			return err
		}
		if ingClassConfig.IngClassParams != nil && ingClassConfig.IngClassParams.Spec.Scheme != nil {
			rawSchema = string(*ingClassConfig.IngClassParams.Spec.Scheme)
		} else {
			_ = v.annotationParser.ParseStringAnnotation(annotations.IngressSuffixScheme, &rawSchema, ing.Annotations)
		}
		controller := ingClassConfig.IngClass.Spec.Controller
		controllerPart = strings.Split(controller, "/")[1]
	}
	// Use default scheme if no scheme is specified
	if rawSchema == "" {
		rawSchema = defaultScheme
	}
	if controllerOld, exists := oldIng.Annotations[annotations.IngressClass]; exists {
		// Parse the ingress suffix scheme and ingress class from annotations
		_ = v.annotationParser.ParseStringAnnotation(annotations.IngressSuffixScheme, &rawSchemaOld, oldIng.Annotations)
		controllerPartOld = controllerOld
	} else if oldIng.Spec.IngressClassName != nil {
		oldIngClassConfig, err := v.classLoader.Load(ctx, oldIng)
		if err != nil {
			return err
		}
		if oldIngClassConfig.IngClassParams != nil && oldIngClassConfig.IngClassParams.Spec.Scheme != nil {
			rawSchemaOld = string(*oldIngClassConfig.IngClassParams.Spec.Scheme)
		} else {
			_ = v.annotationParser.ParseStringAnnotation(annotations.IngressSuffixScheme, &rawSchemaOld, oldIng.Annotations)
		}
		controllerOld := oldIngClassConfig.IngClass.Spec.Controller
		controllerPartOld = strings.Split(controllerOld, "/")[1]
	}
	// Use default scheme if no scheme is specified
	if rawSchemaOld == "" {
		rawSchemaOld = defaultScheme
	}
	// Check if the scheme or type of the load balancer changed in the new Ingress object
	if rawSchemaOld != rawSchema || controllerPart != controllerPartOld {
		// Check if the Ingress object had the deletion protection annotation enabled
		enabled, err := v.getDeletionProtectionEnabled(ing, ingClassConfig)
		if err != nil {
			return err
		}
		if enabled == "true" {
			return errors.Errorf("cannot change the scheme or type of ingress %s/%s with deletion protection enabled", ing.Namespace, ing.Name)
		}
	}
	return nil
}

// getDeletionProtectionEnabled extracts the value of the "deletion_protection.enabled" attribute from the "alb.ingress.kubernetes.io/load-balancer-attributes" annotation of the given Ingress object.
// If the annotation or the attribute is not present, it returns an empty string.
func (v *ingressValidator) getDeletionProtectionEnabled(ing *networking.Ingress, ingClassConfig ingress.ClassConfiguration) (string, error) {
	var lbAttributes map[string]string
	_, err := v.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixLoadBalancerAttributes, &lbAttributes, ing.Annotations)
	if err != nil {
		return "", err
	}
	if lbAttributes[lbAttrsDeletionProtectionEnabled] != "" {
		return lbAttributes[lbAttrsDeletionProtectionEnabled], nil
	}
	if ing.Spec.IngressClassName != nil && ingClassConfig.IngClassParams != nil && len(ingClassConfig.IngClassParams.Spec.LoadBalancerAttributes) != 0 {
		for _, attr := range ingClassConfig.IngClassParams.Spec.LoadBalancerAttributes {
			if attr.Key == "deletion_protection.enabled" {
				deletionProtectionEnabled := attr.Value
				return deletionProtectionEnabled, nil
			}
		}
	}
	return "", nil
}

func (v *ingressValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
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
	mgr.GetWebhookServer().Register(apiPathValidateNetworkingIngress, webhook.ValidatingWebhookForValidator(v))
}
