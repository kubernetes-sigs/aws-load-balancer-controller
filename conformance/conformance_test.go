package conformance

import (
	"testing"
	"time"

	"sigs.k8s.io/gateway-api/conformance"
	"sigs.k8s.io/gateway-api/conformance/utils/suite"
)

func TestConformance(t *testing.T) {
	options := conformance.DefaultOptions(t)

	// Configure skip tests and supported features
	options.SkipTests = []string{
		"GatewayNameMaximumLength",                  // This test is in v1.7, we will need to address it for v1.7 conformance.
		"GatewayInvalidTLSConfiguration",            // We don't use secrets for TLS
		"GatewaySecretInvalidReferenceGrant",        // We don't use secrets
		"GatewaySecretMissingReferenceGrant",        // We don't use secrets
		"GatewaySecretReferenceGrantAllInNamespace", // We don't use secrets
		"GatewaySecretReferenceGrantSpecific",       // We don't use secrets
		"GatewayWithAttachedRoutes",                 // We don't use secrets
		"HTTPRouteHTTPSListener",                    // We don't use secrets
		"HTTPRouteRequestHeaderModifier",            // We don't support native request header modifier. https://docs.aws.amazon.com/elasticloadbalancing/latest/application/header-modification.html
		"HTTPRouteBackendRequestHeaderModifier",     // We don't support native request header modifier. https://docs.aws.amazon.com/elasticloadbalancing/latest/application/header-modification.html
		"HTTPRouteServiceTypes",                     // We don't support other backends other than Services.
		"HTTPRouteHostnameIntersection",             // Works aside from one test which expects ALB to strip off port from value in host header, ALB does not support that. [We theoretically could support it, but it would change how rules are constructed]
		"ListenerSetReferenceGrant",                 // We don't use secrets
		"GatewayInvalidParametersRef",               // We don't use parameter references
	}
	options.SupportedFeatures = suite.ParseSupportedFeaturesSlice("Gateway,HTTPRoute,ReferenceGrant,HTTPRoutePortRedirect,HTTPRouteMethodMatching,HTTPRouteParentRefPort,HTTPRouteDestinationPortMatching,ListenerSet")

	// Configure timeout config
	options.TimeoutConfig.GatewayStatusMustHaveListeners = 8 * time.Minute // we need to wait for LB to be provisioned before updating gateway listener status
	options.TimeoutConfig.GatewayListenersMustHaveConditions = 8 * time.Minute
	options.TimeoutConfig.NamespacesMustBeReady = 8 * time.Minute
	options.TimeoutConfig.DefaultTestTimeout = 8 * time.Minute

	conformance.RunConformanceWithOptions(t, options)
}
