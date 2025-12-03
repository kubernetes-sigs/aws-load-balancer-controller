package aga

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
)

func TestIsGlobalAcceleratorControllerEnabled(t *testing.T) {
	tests := []struct {
		name         string
		featureGates config.FeatureGates
		region       string
		want         bool
	}{
		{
			name: "Feature gate disabled",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Disable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "us-west-2",
			want:   false,
		},
		{
			name: "Feature gate enabled, standard region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "us-west-2",
			want:   true,
		},
		{
			name: "Feature gate enabled, China region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "cn-north-1",
			want:   false,
		},
		{
			name: "Feature gate enabled, GovCloud region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "us-gov-west-1",
			want:   false,
		},
		{
			name: "Feature gate enabled, ISO region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "us-iso-east-1",
			want:   false,
		},
		{
			name: "Feature gate enabled, ISO-E region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "eu-isoe-west-1",
			want:   false,
		},
		{
			name: "Feature gate enabled, upper case region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "US-WEST-2",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGlobalAcceleratorControllerEnabled(tt.featureGates, tt.region); got != tt.want {
				t.Errorf("IsGlobalAcceleratorControllerEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsPortInRanges(t *testing.T) {
	tests := []struct {
		name       string
		port       int32
		portRanges []agamodel.PortRange
		expected   bool
	}{
		{
			name: "port within single range",
			port: 85,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
			},
			expected: true,
		},
		{
			name: "port at lower boundary",
			port: 80,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
			},
			expected: true,
		},
		{
			name: "port at upper boundary",
			port: 100,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
			},
			expected: true,
		},
		{
			name: "port below range",
			port: 79,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
			},
			expected: false,
		},
		{
			name: "port above range",
			port: 101,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
			},
			expected: false,
		},
		{
			name: "port within one of multiple ranges",
			port: 443,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
				{
					FromPort: 443,
					ToPort:   443,
				},
				{
					FromPort: 8080,
					ToPort:   8090,
				},
			},
			expected: true,
		},
		{
			name: "port not within any of multiple ranges",
			port: 8000,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
				{
					FromPort: 443,
					ToPort:   443,
				},
				{
					FromPort: 8080,
					ToPort:   8090,
				},
			},
			expected: false,
		},
		{
			name:       "empty port ranges",
			port:       80,
			portRanges: []agamodel.PortRange{},
			expected:   false,
		},
		{
			name:       "nil port ranges",
			port:       80,
			portRanges: nil,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPortInRanges(tt.port, tt.portRanges)
			assert.Equal(t, tt.expected, result)
		})
	}
}
