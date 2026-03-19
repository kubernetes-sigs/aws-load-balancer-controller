package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// JoinStrings joins a string slice with commas.
func JoinStrings(ss []string) string {
	return strings.Join(ss, ",")
}

// GetSectionName generates a valid Gateway API SectionName string from protocol and port.
// The result is lowercase to satisfy the SectionName regex: ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
func GetSectionName(protocol string, port int32) string {
	return fmt.Sprintf("%s-%d", strings.ToLower(protocol), port)
}

// resourceName builds a deterministic, collision-resistant resource name.
// Pattern: {name}-{suffix}-{hash10}
// The hash is SHA-256 of namespace/name/suffix, truncated to 10 hex chars.
// This mirrors the controller's naming pattern (e.g., "k8s-demo-echogate-ddb87892de").
// Name is truncated to 8 chars and suffix to 8 chars; trailing hyphens from
// truncation are trimmed to produce valid K8s names.
func resourceName(namespace, name, suffix string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(namespace))
	_, _ = h.Write([]byte("/"))
	_, _ = h.Write([]byte(name))
	_, _ = h.Write([]byte("/"))
	_, _ = h.Write([]byte(suffix))
	hash := hex.EncodeToString(h.Sum(nil))[:10]
	truncName := strings.TrimRight(truncate(name, 8), "-")
	truncSuffix := strings.TrimRight(truncate(suffix, 8), "-")
	return fmt.Sprintf("%s-%s-%s", truncName, truncSuffix, hash)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// GetGatewayName returns the Gateway resource name derived from the Ingress.
func GetGatewayName(namespace, ingressName string) string {
	return resourceName(namespace, ingressName, "gateway")
}

// GetLBConfigName returns the LoadBalancerConfiguration resource name.
func GetLBConfigName(namespace, ingressName string) string {
	return resourceName(namespace, ingressName, "lb-config")
}

// GetHTTPRouteName returns the HTTPRoute resource name.
func GetHTTPRouteName(namespace, ingressName string) string {
	return resourceName(namespace, ingressName, "route")
}

// GetDefaultHTTPRouteName returns the HTTPRoute name for the default backend catch-all route.
func GetDefaultHTTPRouteName(namespace, ingressName string) string {
	return resourceName(namespace, ingressName, "default-route")
}

// GetTGConfigName returns the TargetGroupConfiguration resource name.
func GetTGConfigName(namespace, serviceName string) string {
	return resourceName(namespace, serviceName, "tg-config")
}
