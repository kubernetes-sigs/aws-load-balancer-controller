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
		"GatewayInvalidTLSConfiguration",
		"GatewaySecretInvalidReferenceGrant",
		"GatewaySecretMissingReferenceGrant",
		"GatewaySecretReferenceGrantAllInNamespace",
		"GatewaySecretReferenceGrantSpecific",
		"GatewayWithAttachedRoutes",
		"HTTPRouteBackendRequestHeaderModifier",
		"HTTPRouteHTTPSListener",
		"HTTPRouteRequestHeaderModifier",
		"HTTPRouteHostnameIntersection",
		"HTTPRouteServiceTypes",
	}
	options.SupportedFeatures = suite.ParseSupportedFeatures("Gateway,HTTPRoute,ReferenceGrant,HTTPRoutePortRedirect,HTTPRouteMethodMatching,HTTPRouteParentRefPort,HTTPRouteDestinationPortMatching")

	// Configure timeout config
	options.TimeoutConfig.GatewayStatusMustHaveListeners = 8 * time.Minute // we need to wait for LB to be provisioned before updating gateway listener status
	options.TimeoutConfig.GatewayListenersMustHaveConditions = 8 * time.Minute
	options.TimeoutConfig.NamespacesMustBeReady = 8 * time.Minute
	options.TimeoutConfig.DefaultTestTimeout = 8 * time.Minute

	conformance.RunConformanceWithOptions(t, options)
}
