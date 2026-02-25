package ingress

import (
	networking "k8s.io/api/networking/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"

	"github.com/go-logr/logr"
)

const (
	// annotationValueTrue is the expected value for enabling skip-on-cert-error
	annotationValueTrue = "true"
	// annotationValueFalse is the expected value for disabling skip-on-cert-error
	annotationValueFalse = "false"
)

// CertErrorSkipInfo contains information about an Ingress that should be skipped
// due to a certificate error when the skip-on-cert-error annotation is enabled.
type CertErrorSkipInfo struct {
	// Ingress is the Ingress that should be skipped
	Ingress *networking.Ingress
	// Error is the original certificate discovery error
	Error error
	// FailedHosts is the list of TLS hosts that failed certificate discovery
	FailedHosts []string
}

// ShouldSkipOnCertError checks if the skip-on-cert-error annotation is enabled for an Ingress.
// It returns true only when the annotation value is exactly "true".
// It returns false for absent annotation, "false" value, or invalid values.
// A warning is logged for invalid annotation values (not "true" or "false").
func ShouldSkipOnCertError(ing *networking.Ingress, annotationParser annotations.Parser, logger logr.Logger) bool {
	var rawValue string
	exists := annotationParser.ParseStringAnnotation(annotations.IngressSuffixSkipOnCertError, &rawValue, ing.Annotations)

	if !exists {
		return false
	}

	switch rawValue {
	case annotationValueTrue:
		return true
	case annotationValueFalse:
		return false
	default:
		// Invalid annotation value - log warning and return false (safe default)
		logger.Info("Invalid value for skip-on-cert-error annotation, treating as disabled",
			"ingress", k8s.NamespacedName(ing),
			"value", rawValue,
			"validValues", []string{annotationValueTrue, annotationValueFalse})
		return false
	}
}

// GetIngressTLSHosts extracts all TLS hosts from an Ingress resource.
// This includes hosts from both the rules and the TLS configuration.
func GetIngressTLSHosts(ing *networking.Ingress) []string {
	hostsSet := make(map[string]struct{})
	for _, r := range ing.Spec.Rules {
		if len(r.Host) != 0 {
			hostsSet[r.Host] = struct{}{}
		}
	}
	for _, t := range ing.Spec.TLS {
		for _, host := range t.Hosts {
			hostsSet[host] = struct{}{}
		}
	}

	hosts := make([]string, 0, len(hostsSet))
	for host := range hostsSet {
		hosts = append(hosts, host)
	}
	return hosts
}
