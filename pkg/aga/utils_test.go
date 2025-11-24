package aga

import (
	"testing"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
)

func TestIsAGAControllerEnabled(t *testing.T) {
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
				fg.Disable(config.AGAController)
				return fg
			}(),
			region: "us-west-2",
			want:   false,
		},
		{
			name: "Feature gate enabled, standard region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.AGAController)
				return fg
			}(),
			region: "us-west-2",
			want:   true,
		},
		{
			name: "Feature gate enabled, China region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.AGAController)
				return fg
			}(),
			region: "cn-north-1",
			want:   false,
		},
		{
			name: "Feature gate enabled, GovCloud region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.AGAController)
				return fg
			}(),
			region: "us-gov-west-1",
			want:   false,
		},
		{
			name: "Feature gate enabled, ISO region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.AGAController)
				return fg
			}(),
			region: "us-iso-east-1",
			want:   false,
		},
		{
			name: "Feature gate enabled, ISO-E region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.AGAController)
				return fg
			}(),
			region: "eu-isoe-west-1",
			want:   false,
		},
		{
			name: "Feature gate enabled, upper case region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.AGAController)
				return fg
			}(),
			region: "US-WEST-2",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAGAControllerEnabled(tt.featureGates, tt.region); got != tt.want {
				t.Errorf("IsAGAControllerEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
