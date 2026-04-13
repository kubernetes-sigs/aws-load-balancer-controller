package translate

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/intstr"
	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
)

// ingressAnnotationParser is a package-level parser for Ingress annotations.
// All annotation helpers below delegate to this parser for consistent prefix handling.
var ingressAnnotationParser = annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress)

// getString returns the annotation value for the given suffix, or empty string if not present.
func getString(annos map[string]string, suffix string) string {
	var v string
	ingressAnnotationParser.ParseStringAnnotation(suffix, &v, annos)
	return v
}

// getBool parses a boolean annotation. Returns nil if not present.
func getBool(annos map[string]string, suffix string) *bool {
	var v bool
	exists, err := ingressAnnotationParser.ParseBoolAnnotation(suffix, &v, annos)
	if !exists || err != nil {
		return nil
	}
	return &v
}

// getInt32 parses an int32 annotation. Returns nil if not present.
func getInt32(annos map[string]string, suffix string) *int32 {
	var v int32
	exists, err := ingressAnnotationParser.ParseInt32Annotation(suffix, &v, annos)
	if !exists || err != nil {
		return nil
	}
	return &v
}

// getInt64 parses an int64 annotation. Returns nil if not present.
func getInt64(annos map[string]string, suffix string) *int64 {
	var v int64
	exists, err := ingressAnnotationParser.ParseInt64Annotation(suffix, &v, annos)
	if !exists || err != nil {
		return nil
	}
	return &v
}

// getStringSlice parses a comma-separated string list annotation.
func getStringSlice(annos map[string]string, suffix string) []string {
	var v []string
	if !ingressAnnotationParser.ParseStringSliceAnnotation(suffix, &v, annos) {
		return nil
	}
	return v
}

// getStringMap parses a stringMap annotation (k1=v1,k2=v2).
func getStringMap(annos map[string]string, suffix string) map[string]string {
	var v map[string]string
	exists, err := ingressAnnotationParser.ParseStringMapAnnotation(suffix, &v, annos)
	if !exists || err != nil {
		return nil
	}
	return v
}

// listenPortEntry represents a single entry in the listen-ports JSON annotation.
type listenPortEntry struct {
	Protocol string
	Port     int32
}

// parseListenPorts parses the listen-ports JSON annotation.
// Format: '[{"HTTP": 80}, {"HTTPS": 443}]'
func parseListenPorts(annos map[string]string) ([]listenPortEntry, error) {
	var raw []map[string]int32
	exists, err := ingressAnnotationParser.ParseJSONAnnotation(annotations.IngressSuffixListenPorts, &raw, annos)
	if err != nil {
		return nil, fmt.Errorf("failed to parse listen-ports annotation: %w", err)
	}
	if !exists {
		return nil, nil
	}
	var result []listenPortEntry
	for _, entry := range raw {
		for proto, port := range entry {
			result = append(result, listenPortEntry{
				Protocol: strings.ToUpper(proto),
				Port:     port,
			})
		}
	}
	return result, nil
}

// parseConditionAnnotation parses the alb.ingress.kubernetes.io/conditions.<svcName> JSON annotation.
func parseConditionAnnotation(annos map[string]string, svcName string) ([]ingress.RuleCondition, error) {
	var conditions []ingress.RuleCondition
	annotationSuffix := fmt.Sprintf("conditions.%s", svcName)
	exists, err := ingressAnnotationParser.ParseJSONAnnotation(annotationSuffix, &conditions, annos)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	return conditions, nil
}

// parseTransformAnnotation parses the alb.ingress.kubernetes.io/transforms.<svcName> JSON annotation.
func parseTransformAnnotation(annos map[string]string, svcName string) ([]ingress.Transform, error) {
	var transforms []ingress.Transform
	annotationSuffix := fmt.Sprintf("transforms.%s", svcName)
	exists, err := ingressAnnotationParser.ParseJSONAnnotation(annotationSuffix, &transforms, annos)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	return transforms, nil
}

// parseJwtValidationAnnotation parses the alb.ingress.kubernetes.io/jwt-validation JSON annotation.
func parseJwtValidationAnnotation(annos map[string]string) (*ingress.JwtValidationConfig, error) {
	var cfg ingress.JwtValidationConfig
	exists, err := ingressAnnotationParser.ParseJSONAnnotation(annotations.IngressSuffixJwtValidation, &cfg, annos)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	return &cfg, nil
}

// parseActionAnnotation parses the alb.ingress.kubernetes.io/actions.<svcName> JSON annotation.
func parseActionAnnotation(annos map[string]string, svcName string) (*ingress.Action, error) {
	var a ingress.Action
	annotationSuffix := fmt.Sprintf("actions.%s", svcName)
	exists, err := ingressAnnotationParser.ParseJSONAnnotation(annotationSuffix, &a, annos)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("missing action annotation %q for service %q", annotationSuffix, svcName)
	}
	// Normalize simplified forward schema: targetGroupARN/targetGroupName at top level
	// gets wrapped into forwardConfig for uniform handling.
	// Matches normalizeSimplifiedSchemaForwardAction in pkg/ingress/enhanced_backend_builder.go.
	if a.Type == ingress.ActionTypeForward && (a.TargetGroupARN != nil || a.TargetGroupName != nil) {
		a = ingress.Action{
			Type: ingress.ActionTypeForward,
			ForwardConfig: &ingress.ForwardActionConfig{
				TargetGroups: []ingress.TargetGroupTuple{
					{
						TargetGroupARN:  a.TargetGroupARN,
						TargetGroupName: a.TargetGroupName,
					},
				},
			},
		}
	}
	// Normalize servicePort to int type for backwards compatibility with old AWSALBIngressController.
	// Matches normalizeServicePortForBackwardsCompatibility in pkg/ingress/enhanced_backend_builder.go.
	if a.Type == ingress.ActionTypeForward && a.ForwardConfig != nil {
		for _, tgt := range a.ForwardConfig.TargetGroups {
			if tgt.ServicePort != nil {
				normalizedPort := intstr.Parse(tgt.ServicePort.String())
				*tgt.ServicePort = normalizedPort
			}
		}
	}
	return &a, nil
}
