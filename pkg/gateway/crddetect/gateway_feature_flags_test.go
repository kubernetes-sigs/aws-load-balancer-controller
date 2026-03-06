package crddetect

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
)

func TestApplyGatewayFeatureFlags_StandardMissing(t *testing.T) {
	fg := config.NewFeatureGates()
	assert.True(t, fg.Enabled(config.ALBGatewayAPI), "precondition: ALBGatewayAPI should default to true")
	assert.True(t, fg.Enabled(config.NLBGatewayAPI), "precondition: NLBGatewayAPI should default to true")

	standardResult := k8s.CRDGroupResult{
		GroupVersion: "gateway.networking.k8s.io/v1",
		AllPresent:   false,
		MissingKinds: []string{"HTTPRoute"},
	}
	experimentalResult := k8s.CRDGroupResult{
		GroupVersion: "gateway.networking.k8s.io/v1alpha2",
		AllPresent:   true,
	}

	ApplyGatewayFeatureFlags(standardResult, experimentalResult, fg, logr.Discard())

	assert.False(t, fg.Enabled(config.ALBGatewayAPI), "ALBGatewayAPI should be disabled when standard CRDs are missing")
	assert.False(t, fg.Enabled(config.NLBGatewayAPI), "NLBGatewayAPI should be disabled when standard CRDs are missing")
}

func TestApplyGatewayFeatureFlags_ExperimentalMissing_StandardPresent(t *testing.T) {
	fg := config.NewFeatureGates()

	standardResult := k8s.CRDGroupResult{
		GroupVersion: "gateway.networking.k8s.io/v1",
		AllPresent:   true,
	}
	experimentalResult := k8s.CRDGroupResult{
		GroupVersion: "gateway.networking.k8s.io/v1alpha2",
		AllPresent:   false,
		MissingKinds: []string{"TCPRoute"},
	}

	ApplyGatewayFeatureFlags(standardResult, experimentalResult, fg, logr.Discard())

	assert.True(t, fg.Enabled(config.ALBGatewayAPI), "ALBGatewayAPI should remain enabled when standard CRDs are present")
	assert.False(t, fg.Enabled(config.NLBGatewayAPI), "NLBGatewayAPI should be disabled when experimental CRDs are missing")
}

func TestApplyGatewayFeatureFlags_BothPresent(t *testing.T) {
	fg := config.NewFeatureGates()

	standardResult := k8s.CRDGroupResult{
		GroupVersion: "gateway.networking.k8s.io/v1",
		AllPresent:   true,
	}
	experimentalResult := k8s.CRDGroupResult{
		GroupVersion: "gateway.networking.k8s.io/v1alpha2",
		AllPresent:   true,
	}

	ApplyGatewayFeatureFlags(standardResult, experimentalResult, fg, logr.Discard())

	assert.True(t, fg.Enabled(config.ALBGatewayAPI), "ALBGatewayAPI should remain enabled")
	assert.True(t, fg.Enabled(config.NLBGatewayAPI), "NLBGatewayAPI should remain enabled")
}

func TestApplyGatewayFeatureFlags_BothMissing(t *testing.T) {
	fg := config.NewFeatureGates()

	standardResult := k8s.CRDGroupResult{
		GroupVersion: "gateway.networking.k8s.io/v1",
		AllPresent:   false,
		MissingKinds: []string{"Gateway", "GatewayClass"},
	}
	experimentalResult := k8s.CRDGroupResult{
		GroupVersion: "gateway.networking.k8s.io/v1alpha2",
		AllPresent:   false,
		MissingKinds: []string{"TCPRoute", "UDPRoute"},
	}

	ApplyGatewayFeatureFlags(standardResult, experimentalResult, fg, logr.Discard())

	assert.False(t, fg.Enabled(config.ALBGatewayAPI), "ALBGatewayAPI should be disabled")
	assert.False(t, fg.Enabled(config.NLBGatewayAPI), "NLBGatewayAPI should be disabled")
}

func TestApplyGatewayFeatureFlags_ExplicitlyEnabledStillDisabledWhenCRDsMissing(t *testing.T) {
	fg := config.NewFeatureGates()
	// Simulate explicit --feature-gates ALBGatewayAPI=true,NLBGatewayAPI=true
	fg.Enable(config.ALBGatewayAPI)
	fg.Enable(config.NLBGatewayAPI)

	standardResult := k8s.CRDGroupResult{
		GroupVersion: "gateway.networking.k8s.io/v1",
		AllPresent:   false,
		MissingKinds: []string{"GRPCRoute"},
	}
	experimentalResult := k8s.CRDGroupResult{
		GroupVersion: "gateway.networking.k8s.io/v1alpha2",
		AllPresent:   true,
	}

	ApplyGatewayFeatureFlags(standardResult, experimentalResult, fg, logr.Discard())

	assert.False(t, fg.Enabled(config.ALBGatewayAPI), "ALBGatewayAPI should be force-disabled even when explicitly enabled")
	assert.False(t, fg.Enabled(config.NLBGatewayAPI), "NLBGatewayAPI should be force-disabled even when explicitly enabled")
}
