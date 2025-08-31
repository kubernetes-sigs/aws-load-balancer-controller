package networking

import (
	"context"
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathValidateNetworkingIngress = "/validate-networking-v1-ingress"
)

// NewIngressValidator returns a validator for Ingress API.
func NewIngressValidator(client client.Client, ingConfig config.IngressConfig, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector) *ingressValidator {
	return &ingressValidator{
		annotationParser:                   annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress),
		classAnnotationMatcher:             ingress.NewDefaultClassAnnotationMatcher(ingConfig.IngressClass),
		classLoader:                        ingress.NewDefaultClassLoader(client, false),
		disableIngressClassAnnotation:      ingConfig.DisableIngressClassAnnotation,
		disableIngressGroupAnnotation:      ingConfig.DisableIngressGroupNameAnnotation,
		manageIngressesWithoutIngressClass: ingConfig.IngressClass == "",
		logger:                             logger,
		metricsCollector:                   metricsCollector,
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
	metricsCollector                   lbcmetrics.MetricCollector
}

func (v *ingressValidator) Prototype(req admission.Request) (runtime.Object, error) {
	return &networking.Ingress{}, nil
}

func (v *ingressValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	ing := obj.(*networking.Ingress)
	if skip, err := v.checkIngressClass(ctx, ing); skip || err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateNetworkingIngress, "checkIngressClass")
		return err
	}
	if err := v.checkIngressClassAnnotationUsage(ing, nil); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateNetworkingIngress, "checkIngressClassAnnotationUsage")
		return err
	}
	if err := v.checkGroupNameAnnotationUsage(ing, nil); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateNetworkingIngress, "checkGroupNameAnnotationUsage")
		return err
	}
	if err := v.checkIngressClassUsage(ctx, ing, nil); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateNetworkingIngress, "checkIngressClassUsage")
		return err
	}
	if err := v.checkIngressAnnotationConditions(ing); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateNetworkingIngress, "checkIngressAnnotationConditions")
		return err
	}
	if err := v.checkFrontendNlbTagsAnnotation(ing); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateNetworkingIngress, "checkFrontendNlbTagsAnnotation")
		return err
	}
	return nil
}

func (v *ingressValidator) ValidateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) error {
	ing := obj.(*networking.Ingress)
	oldIng := oldObj.(*networking.Ingress)
	if skip, err := v.checkIngressClass(ctx, ing); skip || err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateNetworkingIngress, "checkIngressClass")
		return err
	}
	if err := v.checkIngressClassAnnotationUsage(ing, oldIng); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateNetworkingIngress, "checkIngressClassAnnotationUsage")
		return err
	}
	if err := v.checkGroupNameAnnotationUsage(ing, oldIng); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateNetworkingIngress, "checkGroupNameAnnotationUsage")
		return err
	}
	if err := v.checkIngressClassUsage(ctx, ing, oldIng); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateNetworkingIngress, "checkIngressClassUsage")
		return err
	}
	if err := v.checkIngressAnnotationConditions(ing); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateNetworkingIngress, "checkIngressAnnotationConditions")
		return err
	}
	if err := v.checkFrontendNlbTagsAnnotation(ing); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateNetworkingIngress, "checkFrontendNlbTagsAnnotation")
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
		newIngressClassName = awssdk.ToString(ing.Spec.IngressClassName)
	}
	if oldIng != nil && oldIng.Spec.IngressClassName != nil {
		usedInOldIng = true
		oldIngressClassName = awssdk.ToString(oldIng.Spec.IngressClassName)
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

// checkFrontendNlbTagsAnnotation validates the frontend NLB tags annotation format and constraints
func (v *ingressValidator) checkFrontendNlbTagsAnnotation(ing *networking.Ingress) error {
	annotationKey := fmt.Sprintf("%s/%s", annotations.AnnotationPrefixIngress, annotations.IngressSuffixFrontendNlbTags)
	annotationValue, exists := ing.Annotations[annotationKey]

	// If annotation doesn't exist, no validation needed
	if !exists {
		return nil
	}

	// If annotation is empty, no validation needed
	if strings.TrimSpace(annotationValue) == "" {
		return nil
	}

	// Parse the annotation value manually to provide better error messages
	tags := make(map[string]string)
	tagPairs := strings.Split(annotationValue, ",")

	for _, tagPair := range tagPairs {
		tagPair = strings.TrimSpace(tagPair)
		if tagPair == "" {
			continue // Skip empty tag pairs
		}

		// Check for proper key=value format
		parts := strings.SplitN(tagPair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid frontend NLB tags format: tag '%s' must be in 'key=value' format", tagPair)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Validate key and value are not empty
		if key == "" {
			return fmt.Errorf("invalid frontend NLB tags format: tag key cannot be empty")
		}
		if value == "" {
			return fmt.Errorf("invalid frontend NLB tags format: tag value cannot be empty")
		}

		// Check for duplicate keys
		if _, exists := tags[key]; exists {
			return fmt.Errorf("invalid frontend NLB tags: duplicate tag key '%s'", key)
		}

		tags[key] = value
	}

	// Validate tag count limit (AWS limit is 50 tags)
	const maxTagCount = 50
	if len(tags) > maxTagCount {
		return fmt.Errorf("invalid frontend NLB tags: number of tags (%d) exceeds maximum allowed (%d)", len(tags), maxTagCount)
	}

	// Validate tag key and value constraints
	const maxTagKeyLength = 128
	const maxTagValueLength = 256

	for key, value := range tags {
		// Validate tag key length
		if len(key) > maxTagKeyLength {
			return fmt.Errorf("invalid frontend NLB tags: tag key exceeds maximum length of %d characters", maxTagKeyLength)
		}

		// Validate tag value length
		if len(value) > maxTagValueLength {
			return fmt.Errorf("invalid frontend NLB tags: tag value exceeds maximum length of %d characters", maxTagValueLength)
		}

		// Check for AWS reserved tag keys (aws:* pattern)
		if strings.HasPrefix(strings.ToLower(key), "aws:") {
			return fmt.Errorf("invalid frontend NLB tags: tag key '%s' is reserved (aws:* pattern)", key)
		}
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-networking-v1-ingress,mutating=false,failurePolicy=fail,groups=networking.k8s.io,resources=ingresses,verbs=create;update,versions=v1,name=vingress.elbv2.k8s.aws,sideEffects=None,matchPolicy=Equivalent,webhookVersions=v1,admissionReviewVersions=v1beta1

func (v *ingressValidator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathValidateNetworkingIngress, webhook.ValidatingWebhookForValidator(v, mgr.GetScheme()))
}
