package shared_utils

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
)

type mockFeatureGates struct {
	enabled bool
}

func (m *mockFeatureGates) Enabled(feature config.Feature) bool {
	if feature == config.AGAController {
		return m.enabled
	}
	return false
}

func (m *mockFeatureGates) Enable(feature config.Feature)  {}
func (m *mockFeatureGates) Disable(feature config.Feature) {}
func (m *mockFeatureGates) BindFlags(fs *pflag.FlagSet)    {}

func Test_IsAGAControllerEnabled(t *testing.T) {
	tests := []struct {
		name         string
		featureGate  bool
		region       string
		expectResult bool
	}{
		{
			name:         "feature gate disabled",
			featureGate:  false,
			region:       "us-west-2",
			expectResult: false,
		},
		{
			name:         "feature gate enabled, standard region",
			featureGate:  true,
			region:       "us-west-2",
			expectResult: true,
		},
		{
			name:         "feature gate enabled, eu region",
			featureGate:  true,
			region:       "eu-west-1",
			expectResult: true,
		},
		{
			name:         "feature gate enabled, China region",
			featureGate:  true,
			region:       "cn-north-1",
			expectResult: false,
		},
		{
			name:         "feature gate enabled, GovCloud region",
			featureGate:  true,
			region:       "us-gov-west-1",
			expectResult: false,
		},
		{
			name:         "feature gate enabled, ap region",
			featureGate:  true,
			region:       "ap-southeast-1",
			expectResult: true,
		},
		{
			name:         "feature gate enabled, iso region",
			featureGate:  true,
			region:       "us-isof-east-1",
			expectResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFG := &mockFeatureGates{enabled: tt.featureGate}
			result := IsAGAControllerEnabled(mockFG, tt.region)
			assert.Equal(t, tt.expectResult, result)
		})
	}
}
