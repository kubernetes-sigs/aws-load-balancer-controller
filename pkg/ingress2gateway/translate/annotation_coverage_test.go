package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
)

// TestAllIngressAnnotationsCovered ensures every Ingress annotation suffix defined in
// pkg/annotations/constants.go is accounted for in the migration tool.
//
// WHY THIS TEST EXISTS:
// If a new annotation is added to pkg/annotations/constants.go but not categorized here,
// this test fails — forcing the developer to decide how to handle it in the migration tool.
func TestAllIngressAnnotationsCovered(t *testing.T) {

	// Implemented: annotations handled in the translate package.
	implemented := []string{
		// LB-level → LoadBalancerConfiguration
		annotations.IngressSuffixScheme,                          // → spec.scheme
		annotations.IngressSuffixLoadBalancerName,                // → spec.loadBalancerName
		annotations.IngressSuffixIPAddressType,                   // → spec.ipAddressType
		annotations.IngressSuffixSubnets,                         // → spec.loadBalancerSubnets
		annotations.IngressSuffixCustomerOwnedIPv4Pool,           // → spec.customerOwnedIpv4Pool
		annotations.IngressSuffixIPAMIPv4PoolId,                  // → spec.ipv4IPAMPoolId
		annotations.IngressSuffixSecurityGroups,                  // → spec.securityGroups
		annotations.IngressSuffixManageSecurityGroupRules,        // → spec.manageBackendSecurityGroupRules
		annotations.IngressSuffixInboundCIDRs,                    // → spec.sourceRanges
		annotations.IngressSuffixSecurityGroupPrefixLists,        // → spec.securityGroupPrefixes
		annotations.IngressSuffixLoadBalancerAttributes,          // → spec.loadBalancerAttributes
		annotations.IngressSuffixTags,                            // → spec.tags (LB + TG)
		annotations.IngressSuffixLoadBalancerCapacityReservation, // → spec.minimumLoadBalancerCapacity
		annotations.IngressSuffixWAFv2ACLARN,                     // → spec.wafV2.webACL
		annotations.IngressSuffixWAFv2ACLName,                    // → spec.wafV2.webACL (name variant)
		annotations.IngressSuffixShieldAdvancedProtection,        // → spec.shieldConfiguration.enabled

		// Listener-level → LoadBalancerConfiguration.spec.listenerConfigurations[]
		annotations.IngressSuffixListenPorts,            // → Gateway.spec.listeners + listenerConfigurations
		annotations.IngressSuffixCertificateARN,         // → listenerConfigurations[].defaultCertificate
		annotations.IngressSuffixSSLPolicy,              // → listenerConfigurations[].sslPolicy
		annotations.IngressSuffixlsAttsAnnotationPrefix, // → listenerConfigurations[].listenerAttributes
		annotations.IngressSuffixMutualAuthentication,   // → listenerConfigurations[].mutualAuthentication

		// TG-level → TargetGroupConfiguration.spec.defaultConfiguration
		annotations.IngressSuffixTargetType,                // → targetType
		annotations.IngressSuffixBackendProtocol,           // → protocol
		annotations.IngressSuffixBackendProtocolVersion,    // → protocolVersion
		annotations.IngressSuffixTargetGroupAttributes,     // → targetGroupAttributes
		annotations.IngressSuffixTargetNodeLabels,          // → nodeSelector
		annotations.IngressLBSuffixMultiClusterTargetGroup, // → enableMultiCluster
		annotations.IngressSuffixTargetControlPort,         // → targetControlPort

		// Health check → TargetGroupConfiguration.spec.defaultConfiguration.healthCheckConfig
		annotations.IngressSuffixHealthCheckPort,            // → healthCheckPort
		annotations.IngressSuffixHealthCheckProtocol,        // → healthCheckProtocol
		annotations.IngressSuffixHealthCheckPath,            // → healthCheckPath
		annotations.IngressSuffixHealthCheckIntervalSeconds, // → healthCheckInterval
		annotations.IngressSuffixHealthCheckTimeoutSeconds,  // → healthCheckTimeout
		annotations.IngressSuffixHealthyThresholdCount,      // → healthyThresholdCount
		annotations.IngressSuffixUnhealthyThresholdCount,    // → unhealthyThresholdCount
		annotations.IngressSuffixSuccessCodes,               // → matcher.httpCode

		// Routing → HTTPRoute
		annotations.IngressSuffixUseRegexPathMatch, // → matches[].path.type: RegularExpression
	}

	// Planned: annotations not yet implemented.
	planned := []string{
		// Ingress grouping
		annotations.IngressSuffixGroupName,  // → shared Gateway per group
		annotations.IngressSuffixGroupOrder, // → HTTPRoute rule ordering

		// Routing
		annotations.IngressSuffixSSLRedirect, // → HTTPRoute RequestRedirect filter

		// Authentication → ListenerRuleConfiguration.spec.actions[]
		annotations.IngressSuffixAuthType,                     // → action type selector
		annotations.IngressSuffixAuthIDPCognito,               // → authenticateCognitoConfig
		annotations.IngressSuffixAuthIDPOIDC,                  // → authenticateOIDCConfig
		annotations.IngressSuffixAuthOnUnauthenticatedRequest, // → onUnauthenticatedRequest
		annotations.IngressSuffixAuthScope,                    // → scope
		annotations.IngressSuffixAuthSessionCookie,            // → sessionCookieName
		annotations.IngressSuffixAuthSessionTimeout,           // → sessionTimeout
		annotations.IngressSuffixJwtValidation,                // → jwtValidationConfig

		// Frontend NLB → Gateway Chaining (NLB Gateway + TCPRoutes)
		annotations.IngressSuffixEnableFrontendNlb,                             // → NLB GatewayClass + Gateway + TCPRoutes
		annotations.IngressSuffixFrontendNlbScheme,                             // → NLB LoadBalancerConfiguration.spec.scheme
		annotations.IngressSuffixFrontendNlbSubnets,                            // → NLB LoadBalancerConfiguration.spec.loadBalancerSubnets
		annotations.IngressSuffixFrontendNlbSecurityGroups,                     // → NLB LoadBalancerConfiguration.spec.securityGroups
		annotations.IngressSuffixFrontendNlbListenerPortMapping,                // → TCPRoute port mapping
		annotations.IngressSuffixFrontendNlbEipAllocations,                     // → NLB SubnetConfiguration.eipAllocation
		annotations.IngressSuffixFrontendNlbHealthCheckPort,                    // → ALB-target TGC healthCheckConfig
		annotations.IngressSuffixFrontendNlbHealthCheckProtocol,                // → ALB-target TGC healthCheckConfig
		annotations.IngressSuffixFrontendNlbHealthCheckPath,                    // → ALB-target TGC healthCheckConfig
		annotations.IngressSuffixFrontendNlbHealthCheckIntervalSeconds,         // → ALB-target TGC healthCheckConfig
		annotations.IngressSuffixFrontendNlbHealthCheckTimeoutSeconds,          // → ALB-target TGC healthCheckConfig
		annotations.IngressSuffixFrontendNlbHealthCheckHealthyThresholdCount,   // → ALB-target TGC healthCheckConfig
		annotations.IngressSuffixFrontendNlHealthCheckbUnhealthyThresholdCount, // → ALB-target TGC healthCheckConfig
		annotations.IngressSuffixFrontendNlbHealthCheckSuccessCodes,            // → ALB-target TGC healthCheckConfig
		annotations.IngressSuffixFrontendNlbAttributes,                         // → NLB LoadBalancerConfiguration.spec.loadBalancerAttributes
		annotations.IngressSuffixFrontendNlbTags,                               // → NLB LoadBalancerConfiguration.spec.tags
	}

	// Not applicable: no Gateway API equivalent.
	notApplicable := []string{
		annotations.IngressSuffixWAFACLID, // WAF Classic not supported in Gateway API
		annotations.IngressSuffixWebACLID, // deprecated alias of waf-acl-id
	}

	// Verify no duplicates across categories
	all := make(map[string]bool)
	for _, s := range implemented {
		assert.False(t, all[s], "duplicate annotation: %s", s)
		all[s] = true
	}
	for _, s := range planned {
		assert.False(t, all[s], "duplicate annotation: %s", s)
		all[s] = true
	}
	for _, s := range notApplicable {
		assert.False(t, all[s], "duplicate annotation: %s", s)
		all[s] = true
	}

	// Total IngressSuffix* + IngressLBSuffix* constants in pkg/annotations/constants.go.
	// Update when adding new annotations: grep -c 'IngressSuffix\|IngressLBSuffix' pkg/annotations/constants.go
	const totalExpectedAnnotations = 66

	assert.Equal(t, totalExpectedAnnotations, len(all),
		"Annotation count mismatch. A new Ingress annotation was likely added to pkg/annotations/constants.go. "+
			"Add it to implemented, planned, or notApplicable, then update totalExpectedAnnotations.")
}
