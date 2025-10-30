package conformance

import (
	"testing"
	"time"

	"sigs.k8s.io/gateway-api/conformance"
	"sigs.k8s.io/gateway-api/conformance/utils/suite"
)

func TestConformance(t *testing.T) {
	options := conformance.DefaultOptions(t)

	// TODO: SkipTests, SupportedFeatures,  ExemptFeatures needs to be updated after we conduct all conformance tests
	// Below is only an example for now
	// Configure skip tests, supported features and exempt features
	options.SkipTests = suite.ParseSkipTests("GatewaySecretInvalidReferenceGrant")
	options.SupportedFeatures = suite.ParseSupportedFeatures("Gateway,HTTPRoute")
	options.ExemptFeatures = suite.ParseSupportedFeatures("GatewayStaticAddresses,GatewayHTTPListenerIsolation")

	// Configure timeout config
	options.TimeoutConfig.GatewayStatusMustHaveListeners = 10 * time.Minute // we need to wait for LB to be provisioned before updating gateway listener status
	options.TimeoutConfig.GatewayListenersMustHaveConditions = 10 * time.Minute
	options.TimeoutConfig.NamespacesMustBeReady = 10 * time.Minute
	options.TimeoutConfig.DefaultTestTimeout = 10 * time.Minute

	conformance.RunConformanceWithOptions(t, options)
}
