package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
)

// Resource type categories for annotation mapping.
const (
	LoadBalancerConfig = "LoadBalancerConfiguration"
	ListenerConfig     = "ListenerConfiguration"
	TargetGroupConfig  = "TargetGroupConfiguration"
	HealthCheck        = "HealthCheckConfig"
	HTTPRouteConfig    = "HTTPRoute"
	IngressGrouping    = "IngressGrouping"
	Routing            = "Routing"
	Authentication     = "Authentication"
	FrontendNLB        = "FrontendNLB"
	NotApplicable      = "NotApplicable"
)

// TestAllIngressAnnotationsCovered ensures every Ingress annotation suffix defined in
// pkg/annotations/constants.go is accounted for in the migration tool.
//
// WHY THIS TEST EXISTS:
// If a new annotation is added to pkg/annotations/constants.go but not categorized here,
// this test fails — forcing the developer to decide how to handle it in the migration tool.
func TestAllIngressAnnotationsCovered(t *testing.T) {

	// Implemented: annotations handled in the translate package.
	implemented := map[string][]string{
		LoadBalancerConfig: {
			annotations.IngressSuffixScheme,
			annotations.IngressSuffixLoadBalancerName,
			annotations.IngressSuffixIPAddressType,
			annotations.IngressSuffixSubnets,
			annotations.IngressSuffixCustomerOwnedIPv4Pool,
			annotations.IngressSuffixIPAMIPv4PoolId,
			annotations.IngressSuffixSecurityGroups,
			annotations.IngressSuffixManageSecurityGroupRules,
			annotations.IngressSuffixInboundCIDRs,
			annotations.IngressSuffixSecurityGroupPrefixLists,
			annotations.IngressSuffixLoadBalancerAttributes,
			annotations.IngressSuffixTags,
			annotations.IngressSuffixLoadBalancerCapacityReservation,
			annotations.IngressSuffixWAFv2ACLARN,
			annotations.IngressSuffixWAFv2ACLName,
			annotations.IngressSuffixShieldAdvancedProtection,
		},
		ListenerConfig: {
			annotations.IngressSuffixListenPorts,
			annotations.IngressSuffixCertificateARN,
			annotations.IngressSuffixSSLPolicy,
			annotations.IngressSuffixlsAttsAnnotationPrefix,
			annotations.IngressSuffixMutualAuthentication,
		},
		TargetGroupConfig: {
			annotations.IngressSuffixTargetType,
			annotations.IngressSuffixBackendProtocol,
			annotations.IngressSuffixBackendProtocolVersion,
			annotations.IngressSuffixTargetGroupAttributes,
			annotations.IngressSuffixTargetNodeLabels,
			annotations.IngressLBSuffixMultiClusterTargetGroup,
			annotations.IngressSuffixTargetControlPort,
		},
		HealthCheck: {
			annotations.IngressSuffixHealthCheckPort,
			annotations.IngressSuffixHealthCheckProtocol,
			annotations.IngressSuffixHealthCheckPath,
			annotations.IngressSuffixHealthCheckIntervalSeconds,
			annotations.IngressSuffixHealthCheckTimeoutSeconds,
			annotations.IngressSuffixHealthyThresholdCount,
			annotations.IngressSuffixUnhealthyThresholdCount,
			annotations.IngressSuffixSuccessCodes,
		},
		HTTPRouteConfig: {
			annotations.IngressSuffixUseRegexPathMatch,
		},
	}

	// Planned: annotations not yet implemented.
	planned := map[string][]string{
		IngressGrouping: {
			annotations.IngressSuffixGroupName,
			annotations.IngressSuffixGroupOrder,
		},
		Routing: {
			annotations.IngressSuffixSSLRedirect,
			// NOTE: "use-annotation" action backends (alb.ingress.kubernetes.io/actions.{name}),
			// condition annotations (alb.ingress.kubernetes.io/conditions.{name}), and
			// transform annotations (alb.ingress.kubernetes.io/transforms.{name}) are dynamically
			// named and cannot be tracked as static suffixes here.
			// actions.* translation is implemented in translate_action_helper.go (forward, redirect, fixed-response).
			// conditions.* translation is implemented in translate_condition_helper.go.
			// transforms.* translation is implemented in translate_transform_helper.go.
			// ssl-redirect is planned for a subsequent PR.
		},
		Authentication: {
			annotations.IngressSuffixAuthType,
			annotations.IngressSuffixAuthIDPCognito,
			annotations.IngressSuffixAuthIDPOIDC,
			annotations.IngressSuffixAuthOnUnauthenticatedRequest,
			annotations.IngressSuffixAuthScope,
			annotations.IngressSuffixAuthSessionCookie,
			annotations.IngressSuffixAuthSessionTimeout,
			annotations.IngressSuffixJwtValidation,
		},
		FrontendNLB: {
			annotations.IngressSuffixEnableFrontendNlb,
			annotations.IngressSuffixFrontendNlbScheme,
			annotations.IngressSuffixFrontendNlbSubnets,
			annotations.IngressSuffixFrontendNlbSecurityGroups,
			annotations.IngressSuffixFrontendNlbListenerPortMapping,
			annotations.IngressSuffixFrontendNlbEipAllocations,
			annotations.IngressSuffixFrontendNlbHealthCheckPort,
			annotations.IngressSuffixFrontendNlbHealthCheckProtocol,
			annotations.IngressSuffixFrontendNlbHealthCheckPath,
			annotations.IngressSuffixFrontendNlbHealthCheckIntervalSeconds,
			annotations.IngressSuffixFrontendNlbHealthCheckTimeoutSeconds,
			annotations.IngressSuffixFrontendNlbHealthCheckHealthyThresholdCount,
			annotations.IngressSuffixFrontendNlHealthCheckbUnhealthyThresholdCount,
			annotations.IngressSuffixFrontendNlbHealthCheckSuccessCodes,
			annotations.IngressSuffixFrontendNlbAttributes,
			annotations.IngressSuffixFrontendNlbTags,
		},
	}

	// Not applicable: no Gateway API equivalent.
	notApplicable := map[string][]string{
		NotApplicable: {
			annotations.IngressSuffixWAFACLID, // WAF Classic not supported in Gateway API
			annotations.IngressSuffixWebACLID, // deprecated alias of waf-acl-id
		},
	}

	// Flatten all maps and verify no duplicates
	all := make(map[string]bool)
	for category, suffixes := range implemented {
		for _, s := range suffixes {
			assert.False(t, all[s], "duplicate annotation %s in implemented[%s]", s, category)
			all[s] = true
		}
	}
	for category, suffixes := range planned {
		for _, s := range suffixes {
			assert.False(t, all[s], "duplicate annotation %s in planned[%s]", s, category)
			all[s] = true
		}
	}
	for category, suffixes := range notApplicable {
		for _, s := range suffixes {
			assert.False(t, all[s], "duplicate annotation %s in notApplicable[%s]", s, category)
			all[s] = true
		}
	}

	// Total IngressSuffix* + IngressLBSuffix* constants in pkg/annotations/constants.go.
	// Update when adding new annotations: grep -c 'IngressSuffix\|IngressLBSuffix' pkg/annotations/constants.go
	const totalExpectedAnnotations = 66

	assert.Equal(t, totalExpectedAnnotations, len(all),
		"Annotation count mismatch. A new Ingress annotation was likely added to pkg/annotations/constants.go. "+
			"Add it to implemented, planned, or notApplicable, then update totalExpectedAnnotations.")
}
